// Package pagination implements opaque cursor pagination for list endpoints.
//
// We deliberately do not use offset pagination: in a system where a tenant
// can generate hundreds of detections per second, offsets shift between
// pages and clients silently skip or duplicate items. Cursors encode the
// last-seen primary key and a checksum so we can reject tampered cursors at
// the boundary.
package pagination

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Default and maximum page sizes. Servers should clamp client-provided
// page_size to [1, MaxPageSize] and default to DefaultPageSize when missing.
const (
	DefaultPageSize = 50
	MaxPageSize     = 200
)

// ClampPageSize normalizes a user-supplied page size to platform bounds.
func ClampPageSize(n int32) int {
	if n <= 0 {
		return DefaultPageSize
	}
	if n > MaxPageSize {
		return MaxPageSize
	}
	return int(n)
}

// Cursor is the structured payload signed and base64-encoded into the opaque
// page_token. ResourceKind binds a cursor to the list endpoint that issued it
// so an attacker can't move a pipelines cursor to the streams endpoint.
type Cursor struct {
	ResourceKind string    `json:"k"`
	LastID       string    `json:"i"`
	IssuedAt     time.Time `json:"t"`
}

// Encoder signs and base64-encodes cursors. Construct one per service from a
// rotation-aware key (Vault transit, JWKS, etc.). For tests, NewEncoder("dev")
// is fine.
type Encoder struct {
	key []byte
}

// NewEncoder constructs an encoder bound to the given signing key. The key
// must be at least 16 bytes in production; we don't enforce that here to
// keep test ergonomics simple.
func NewEncoder(signingKey string) *Encoder {
	return &Encoder{key: []byte(signingKey)}
}

// Encode produces an opaque, signed cursor string. Returns "" for the
// terminal cursor — list responses should set next_page_token = "" when no
// more results exist.
//
// Wire format: `base64url(body) + "." + base64url(sig)`. We base64-encode
// each part independently so the separator is always unambiguous, even when
// the HMAC signature happens to contain a 0x2E byte.
func (e *Encoder) Encode(kind, lastID string) string {
	if lastID == "" {
		return ""
	}
	c := Cursor{ResourceKind: kind, LastID: lastID, IssuedAt: time.Now().UTC()}
	body, _ := json.Marshal(c)
	mac := hmac.New(sha256.New, e.key)
	mac.Write(body)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// ErrInvalidCursor is returned for malformed or wrong-resource cursors.
var ErrInvalidCursor = errors.New("invalid cursor")

// Decode verifies a cursor, enforces its bound resource kind, and returns the
// decoded structure. Returns ErrInvalidCursor on any failure so handlers can
// translate it to a 400.
func (e *Encoder) Decode(kind, token string) (Cursor, error) {
	if token == "" {
		return Cursor{}, nil
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return Cursor{}, ErrInvalidCursor
	}
	// Strict mode rejects base64 strings whose final-byte unused bits are
	// non-zero. Without this, a tampered last byte can decode to the same
	// signature bytes and slip past the HMAC check.
	body, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	sig, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	mac := hmac.New(sha256.New, e.key)
	mac.Write(body)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return Cursor{}, ErrInvalidCursor
	}
	var c Cursor
	if err := json.Unmarshal(body, &c); err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	if c.ResourceKind != kind {
		return Cursor{}, fmt.Errorf("%w: kind mismatch", ErrInvalidCursor)
	}
	return c, nil
}
