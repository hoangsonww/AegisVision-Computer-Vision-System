// Package jwks is a lightweight JWKS fetch + cache for the auth-proxy.
//
// Two responsibilities:
//
//  1. Fetch a JWKS document from an OIDC issuer and parse it into a
//     map of `kid → *rsa.PublicKey | *ecdsa.PublicKey`.
//  2. Refresh on a timer + on-miss (so a key rotation that adds a new
//     kid is picked up without restart).
//
// The proxy is the request-authentication front-door; this package is
// deliberately conservative:
//
//   - HMAC algorithms are NOT supported (a shared HMAC secret with every
//     verifier defeats OIDC). Only RS*, PS*, and ES* keys are returned.
//   - We do not honor weak / unknown key types (anything not in the
//     above set is silently dropped from the keyset).
//   - A failed refresh keeps the previous keyset live — a network blip
//     against the IdP must not de-auth every in-flight request.
//
// The pkg/platform/middleware.Authn middleware already wraps a JWKS
// fetcher; this package is the implementation we hand it.
package jwks

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// Key is the public-key half of one JWK plus the algorithm name. The
// upstream JWT verifier dispatches on Alg.
type Key struct {
	KID string
	Alg string // "RS256" / "RS384" / "RS512" / "PS256" / "ES256" / "ES384" / ...
	Pub any    // *rsa.PublicKey or *ecdsa.PublicKey
}

// Cache is goroutine-safe.
type Cache struct {
	URL        string
	HTTPClient *http.Client
	// Refresh interval; defaults to 1h.
	Refresh time.Duration
	// Minimum interval between forced refreshes triggered by a missing kid.
	MinRefresh time.Duration

	mu     sync.RWMutex
	keys   map[string]Key
	last   time.Time
	loaded bool
}

// New constructs a cache. Call Start to begin background refresh.
func New(url string) *Cache {
	return &Cache{
		URL:        url,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		Refresh:    1 * time.Hour,
		MinRefresh: 30 * time.Second,
		keys:       map[string]Key{},
	}
}

// Start begins the periodic refresh loop. Returns after the first fetch
// completes (so callers can fail fast on bad config).
func (c *Cache) Start(ctx context.Context) error {
	if err := c.refresh(ctx); err != nil {
		return err
	}
	go c.loop(ctx)
	return nil
}

func (c *Cache) loop(ctx context.Context) {
	t := time.NewTicker(c.Refresh)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = c.refresh(ctx) // failures are non-fatal; we keep the previous keyset
		}
	}
}

// Get returns the key for the given kid. If unknown and we haven't
// refreshed recently, kicks off a forced refresh to pick up new keys.
// Returns (Key{}, false) if no match after refresh.
func (c *Cache) Get(ctx context.Context, kid string) (Key, bool) {
	c.mu.RLock()
	k, ok := c.keys[kid]
	last := c.last
	c.mu.RUnlock()
	if ok {
		return k, true
	}
	if time.Since(last) < c.MinRefresh {
		return Key{}, false
	}
	if err := c.refresh(ctx); err != nil {
		return Key{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	k, ok = c.keys[kid]
	return k, ok
}

// Snapshot returns a copy of the current keyset — for debugging /
// metrics.
func (c *Cache) Snapshot() []Key {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Key, 0, len(c.keys))
	for _, k := range c.keys {
		out = append(out, k)
	}
	return out
}

// refresh fetches the JWKS document and rebuilds the keyset.
func (c *Cache) refresh(ctx context.Context) error {
	if c.URL == "" {
		return errors.New("jwks: empty URL")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jwks: GET %s: %s: %s", c.URL, resp.Status, string(b))
	}
	var doc struct {
		Keys []jwkRaw `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return err
	}
	out := make(map[string]Key, len(doc.Keys))
	for _, j := range doc.Keys {
		k, err := parseKey(j)
		if err != nil {
			continue // drop unsupported types silently
		}
		out[j.KID] = k
	}
	if len(out) == 0 {
		return errors.New("jwks: no usable keys in document")
	}
	c.mu.Lock()
	c.keys = out
	c.last = time.Now()
	c.loaded = true
	c.mu.Unlock()
	return nil
}

type jwkRaw struct {
	KTY string `json:"kty"`
	KID string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func parseKey(j jwkRaw) (Key, error) {
	// Reject anything not "sig" or empty (some IdPs omit use).
	if j.Use != "" && j.Use != "sig" {
		return Key{}, errors.New("not a signing key")
	}
	// Algorithm whitelist — HMAC explicitly excluded.
	switch j.Alg {
	case "RS256", "RS384", "RS512", "PS256", "PS384", "PS512",
		"ES256", "ES384", "ES512":
	default:
		return Key{}, fmt.Errorf("unsupported alg: %q", j.Alg)
	}
	switch j.KTY {
	case "RSA":
		n, err := b64uintFromBase64URL(j.N)
		if err != nil {
			return Key{}, err
		}
		e, err := b64intFromBase64URL(j.E)
		if err != nil {
			return Key{}, err
		}
		return Key{KID: j.KID, Alg: j.Alg, Pub: &rsa.PublicKey{N: n, E: e}}, nil
	case "EC":
		curve, err := curveFor(j.Crv)
		if err != nil {
			return Key{}, err
		}
		x, err := b64uintFromBase64URL(j.X)
		if err != nil {
			return Key{}, err
		}
		y, err := b64uintFromBase64URL(j.Y)
		if err != nil {
			return Key{}, err
		}
		return Key{KID: j.KID, Alg: j.Alg, Pub: &ecdsa.PublicKey{Curve: curve, X: x, Y: y}}, nil
	}
	return Key{}, fmt.Errorf("unsupported kty: %q", j.KTY)
}

func curveFor(crv string) (elliptic.Curve, error) {
	switch crv {
	case "P-256":
		return elliptic.P256(), nil
	case "P-384":
		return elliptic.P384(), nil
	case "P-521":
		return elliptic.P521(), nil
	}
	return nil, fmt.Errorf("unsupported curve: %q", crv)
}

// b64uintFromBase64URL decodes an unpadded base64url integer.
func b64uintFromBase64URL(s string) (*big.Int, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(b), nil
}

func b64intFromBase64URL(s string) (int, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, err
	}
	switch len(b) {
	case 0:
		return 0, errors.New("empty exponent")
	case 1:
		return int(b[0]), nil
	case 2:
		return int(binary.BigEndian.Uint16(append([]byte{0, 0}, b...)[len(b)-2:])), nil
	case 3:
		buf := make([]byte, 4)
		copy(buf[1:], b)
		return int(binary.BigEndian.Uint32(buf)), nil
	case 4:
		return int(binary.BigEndian.Uint32(b)), nil
	}
	return 0, fmt.Errorf("unexpected exponent length %d", len(b))
}
