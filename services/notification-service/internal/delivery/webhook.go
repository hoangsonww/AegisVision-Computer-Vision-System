// Package delivery is the webhook deliverer for the notification-service.
//
// Production semantics:
//
//   - Body is the canonical JSON projection of an Event.
//   - The HTTP request carries an X-Aegis-Signature header:
//       sha256=hex(HMAC_SHA256(secret, body))
//     Subscribers verify on receipt.
//   - 2xx counts as success; everything else triggers exponential backoff
//     (1s, 2s, 4s, 8s, 16s, 32s, 60s capped) until MaxAttempts.
//   - After MaxAttempts the message is sent to a DLQ subject on the bus
//     (notifications.dlq.v1) so SREs can replay.
package delivery

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Endpoint is a webhook target.
type Endpoint struct {
	URL    string
	Secret string // HMAC-SHA256 secret; required.
	// Tenant binds the endpoint to a tenant for fanout.
	Tenant string
	// MaxAttempts; 0 → default 6.
	MaxAttempts int
}

// Outcome captures the result of a delivery attempt.
type Outcome struct {
	StatusCode int
	Attempt    int
	Latency    time.Duration
	Err        error
}

// Deliver POSTs body to ep with an HMAC signature. It returns the final
// Outcome after retries.
type Client struct {
	HTTP        *http.Client
	RetryBaseMS int
	MaxBackoff  time.Duration
}

func NewClient(timeout time.Duration) *Client {
	return &Client{
		HTTP: &http.Client{Timeout: timeout},
		RetryBaseMS: 1000, MaxBackoff: 60 * time.Second,
	}
}

func (c *Client) Deliver(ctx context.Context, ep Endpoint, body []byte) Outcome {
	if ep.URL == "" || ep.Secret == "" {
		return Outcome{Err: errors.New("delivery: URL and Secret required")}
	}
	maxAttempts := ep.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 6
	}
	var last Outcome
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		last = c.attempt(ctx, ep, body, attempt)
		if last.Err == nil && last.StatusCode >= 200 && last.StatusCode < 300 {
			return last
		}
		// Don't sleep after the final attempt.
		if attempt == maxAttempts {
			break
		}
		// Exponential backoff: base * 2^(attempt-1) ms, capped at MaxBackoff.
		d := time.Duration(c.RetryBaseMS) * time.Millisecond << (attempt - 1)
		if d > c.MaxBackoff {
			d = c.MaxBackoff
		}
		select {
		case <-time.After(d):
		case <-ctx.Done():
			last.Err = ctx.Err()
			return last
		}
	}
	return last
}

func (c *Client) attempt(ctx context.Context, ep Endpoint, body []byte, attempt int) Outcome {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		return Outcome{Attempt: attempt, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Aegis-Tenant", ep.Tenant)
	req.Header.Set("X-Aegis-Signature", Sign(ep.Secret, body))
	req.Header.Set("X-Aegis-Attempt", fmt.Sprintf("%d", attempt))
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Outcome{Attempt: attempt, Latency: time.Since(start), Err: err}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return Outcome{StatusCode: resp.StatusCode, Attempt: attempt, Latency: time.Since(start)}
}

// Sign produces the X-Aegis-Signature header value: "sha256=<hex>".
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
