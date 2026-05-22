// JWT bearer-token authentication with JWKS-cached signature verification.
//
// The gateway authenticates clients with OIDC. We accept compact JWT bearer
// tokens, verify the signature against keys fetched from the issuer's JWKS
// endpoint, and propagate the resolved tenant + roles into the request
// context. Authorization (what the principal can do) is delegated to OPA
// further down the chain (see middleware.AuthZ).
//
// Supported algorithms:
//   - RS256 / RS384 / RS512  (RSA-PKCS#1 v1.5)
//   - ES256 / ES384          (ECDSA)
//
// HS* (HMAC) is deliberately NOT supported — sharing the signing key with
// every verifier defeats the point of OIDC, and we never want to be in a
// position where a curl + a leaked secret can mint platform tokens.
package middleware

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aegisvision/pkg/platform/logging"
	"github.com/aegisvision/pkg/platform/problem"
)

// AuthnConfig configures the JWT bearer middleware.
type AuthnConfig struct {
	// JWKSURL is the issuer's JWKS endpoint (.well-known/jwks.json). Required.
	JWKSURL string
	// Issuer is the expected `iss` claim. Required.
	Issuer string
	// Audience is the expected `aud` claim. If non-empty, tokens must include
	// it (the JWT `aud` may be a string or array of strings).
	Audience string
	// HTTPClient overrides the default JWKS fetcher.
	HTTPClient *http.Client
	// CacheTTL is how long a successfully-fetched JWKS is retained.
	CacheTTL time.Duration
	// RefreshTimeout bounds JWKS fetches.
	RefreshTimeout time.Duration
	// Leeway tolerates small clock skew on exp/nbf claims.
	Leeway time.Duration
	// TenantClaim names the JWT claim that carries the tenant id. Default
	// "tenant_id".
	TenantClaim string
	// RolesClaim names the JWT claim that carries the role array. Default
	// "roles".
	RolesClaim string
}

// Authn returns middleware that requires a valid bearer token, verifies its
// signature against JWKS, and attaches the resolved tenant + identity into
// the request context. A missing or invalid token yields 401 problem+json.
func Authn(cfg AuthnConfig) (func(http.Handler) http.Handler, error) {
	if cfg.JWKSURL == "" {
		return nil, errors.New("authn: JWKSURL is required")
	}
	if cfg.Issuer == "" {
		return nil, errors.New("authn: Issuer is required")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 10 * time.Minute
	}
	if cfg.RefreshTimeout == 0 {
		cfg.RefreshTimeout = 3 * time.Second
	}
	if cfg.Leeway == 0 {
		cfg.Leeway = 30 * time.Second
	}
	if cfg.TenantClaim == "" {
		cfg.TenantClaim = "tenant_id"
	}
	if cfg.RolesClaim == "" {
		cfg.RolesClaim = "roles"
	}
	cache := &jwksCache{cfg: cfg}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			if !strings.HasPrefix(authz, "Bearer ") {
				problem.Unauthorized("bearer token required").Write(w)
				return
			}
			tok := strings.TrimPrefix(authz, "Bearer ")
			claims, err := verifyJWT(r.Context(), tok, cache, cfg)
			if err != nil {
				problem.Unauthorized(err.Error()).Write(w)
				return
			}
			tenant, _ := claims[cfg.TenantClaim].(string)
			if tenant == "" {
				problem.Unauthorized("token missing tenant claim").Write(w)
				return
			}
			ctx := logging.WithTenantID(r.Context(), tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}, nil
}

// --- JWKS cache ---------------------------------------------------------

type jwksKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	X5c []string `json:"x5c"`
}

type jwksDoc struct {
	Keys []jwksKey `json:"keys"`
}

type jwksCache struct {
	cfg     AuthnConfig
	mu      sync.RWMutex
	keys    map[string]any
	fetched time.Time
}

func (c *jwksCache) get(ctx context.Context, kid string) (any, error) {
	c.mu.RLock()
	if k, ok := c.keys[kid]; ok && time.Since(c.fetched) < c.cfg.CacheTTL {
		c.mu.RUnlock()
		return k, nil
	}
	c.mu.RUnlock()
	if err := c.refresh(ctx); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	k, ok := c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("authn: kid %q not found in JWKS", kid)
	}
	return k, nil
}

func (c *jwksCache) refresh(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.RefreshTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.JWKSURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("authn: jwks fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authn: jwks status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var doc jwksDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("authn: jwks decode: %w", err)
	}
	keys := make(map[string]any, len(doc.Keys))
	for _, jk := range doc.Keys {
		k, err := parseJWK(jk)
		if err != nil {
			continue
		}
		keys[jk.Kid] = k
	}
	c.mu.Lock()
	c.keys = keys
	c.fetched = time.Now()
	c.mu.Unlock()
	return nil
}

func parseJWK(k jwksKey) (any, error) {
	if len(k.X5c) > 0 {
		raw, err := base64.StdEncoding.DecodeString(k.X5c[0])
		if err == nil {
			// Try DER, then PEM.
			if cert, e := x509.ParseCertificate(raw); e == nil {
				return cert.PublicKey, nil
			}
			if blk, _ := pem.Decode(raw); blk != nil {
				if cert, e := x509.ParseCertificate(blk.Bytes); e == nil {
					return cert.PublicKey, nil
				}
			}
		}
	}
	switch k.Kty {
	case "RSA":
		nb, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
		eb, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
		n := new(big.Int).SetBytes(nb)
		e := 0
		for _, b := range eb {
			e = e<<8 | int(b)
		}
		return &rsa.PublicKey{N: n, E: e}, nil
	case "EC":
		xb, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, err
		}
		yb, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, err
		}
		var curve elliptic.Curve
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		default:
			return nil, fmt.Errorf("authn: unsupported curve %q", k.Crv)
		}
		return &ecdsa.PublicKey{
			Curve: curve,
			X:     new(big.Int).SetBytes(xb),
			Y:     new(big.Int).SetBytes(yb),
		}, nil
	}
	return nil, fmt.Errorf("authn: unsupported kty %q", k.Kty)
}

// --- JWT verify ---------------------------------------------------------

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

func verifyJWT(ctx context.Context, tok string, cache *jwksCache, cfg AuthnConfig) (map[string]any, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, errors.New("token: malformed")
	}
	hdrJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("token: invalid header encoding")
	}
	var hdr jwtHeader
	if err := json.Unmarshal(hdrJSON, &hdr); err != nil {
		return nil, errors.New("token: invalid header")
	}
	if hdr.Kid == "" {
		return nil, errors.New("token: missing kid")
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("token: invalid payload encoding")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("token: invalid signature encoding")
	}

	key, err := cache.get(ctx, hdr.Kid)
	if err != nil {
		return nil, err
	}
	signed := []byte(parts[0] + "." + parts[1])
	if err := verifySignature(hdr.Alg, key, signed, sig); err != nil {
		return nil, err
	}

	var claims map[string]any
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, errors.New("token: invalid claims")
	}
	if iss, _ := claims["iss"].(string); iss != cfg.Issuer {
		return nil, fmt.Errorf("token: issuer mismatch (got %q)", iss)
	}
	if cfg.Audience != "" {
		if !audienceMatches(claims["aud"], cfg.Audience) {
			return nil, errors.New("token: audience mismatch")
		}
	}
	now := time.Now()
	if exp, ok := claims["exp"].(float64); ok {
		if now.After(time.Unix(int64(exp), 0).Add(cfg.Leeway)) {
			return nil, errors.New("token: expired")
		}
	}
	if nbf, ok := claims["nbf"].(float64); ok {
		if now.Add(cfg.Leeway).Before(time.Unix(int64(nbf), 0)) {
			return nil, errors.New("token: not yet valid")
		}
	}
	return claims, nil
}

func verifySignature(alg string, key any, signed, sig []byte) error {
	switch alg {
	case "RS256", "RS384", "RS512":
		pk, ok := key.(*rsa.PublicKey)
		if !ok {
			return errors.New("token: alg/key mismatch (want RSA)")
		}
		h, hashID := newHash(alg)
		h.Write(signed)
		if err := rsa.VerifyPKCS1v15(pk, hashID, h.Sum(nil), sig); err != nil {
			return errors.New("token: signature invalid")
		}
		return nil
	case "ES256", "ES384":
		pk, ok := key.(*ecdsa.PublicKey)
		if !ok {
			return errors.New("token: alg/key mismatch (want ECDSA)")
		}
		var byteSize int
		switch alg {
		case "ES256":
			byteSize = 32
		case "ES384":
			byteSize = 48
		}
		if len(sig) != 2*byteSize {
			return errors.New("token: signature length")
		}
		r := new(big.Int).SetBytes(sig[:byteSize])
		s := new(big.Int).SetBytes(sig[byteSize:])
		h, _ := newHash(alg)
		h.Write(signed)
		if !ecdsa.Verify(pk, h.Sum(nil), r, s) {
			return errors.New("token: signature invalid")
		}
		return nil
	}
	return fmt.Errorf("token: unsupported alg %q", alg)
}

func newHash(alg string) (hash.Hash, crypto.Hash) {
	switch alg {
	case "RS256", "ES256":
		return sha256.New(), crypto.SHA256
	case "RS384", "ES384":
		return sha512.New384(), crypto.SHA384
	case "RS512":
		return sha512.New(), crypto.SHA512
	}
	return sha256.New(), crypto.SHA256
}

func audienceMatches(aud any, want string) bool {
	switch v := aud.(type) {
	case string:
		return v == want
	case []any:
		for _, a := range v {
			if s, _ := a.(string); s == want {
				return true
			}
		}
	}
	return false
}
