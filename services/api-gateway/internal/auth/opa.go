// Package auth wires the AuthZ decider used by api-gateway.
//
// In production a sidecar runs OPA with the rules in /deploy/opa; this file
// exposes both a stub (allow-all for dev) and an HTTP-OPA adapter so the
// production wiring works without code changes when ENABLE_OPA=true.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aegisvision/pkg/platform/middleware"
)

// Decision matches the JSON shape OPA returns for /v1/data/<package>/allow.
type opaResponse struct {
	Result bool `json:"result"`
}

// OPAClient queries an OPA sidecar over HTTP.
type OPAClient struct {
	Endpoint   string // e.g. http://127.0.0.1:8181/v1/data/aegisvision/authz/allow
	HTTPClient *http.Client
}

func NewOPAClient(endpoint string) *OPAClient {
	return &OPAClient{
		Endpoint:   endpoint,
		HTTPClient: &http.Client{Timeout: 250 * time.Millisecond},
	}
}

// Allow implements middleware.AuthZDecider. Network failures surface as
// ErrAuthZUnavailable so the middleware can fail closed and return 503.
func (c *OPAClient) Allow(ctx context.Context, in middleware.AuthZInput) (bool, error) {
	body, err := json.Marshal(map[string]any{"input": in})
	if err != nil {
		return false, fmt.Errorf("opa: marshal input: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("opa: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: %w", middleware.ErrAuthZUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("%w: status %d", middleware.ErrAuthZUnavailable, resp.StatusCode)
	}
	var out opaResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, errors.New("opa: invalid response")
	}
	return out.Result, nil
}

// AllowAll is the dev/test decider. Refuse to use this for any env != dev —
// the constructor refuses to build with that env to make the mistake loud.
func AllowAll(env string) middleware.AuthZDecider {
	if env != "dev" && env != "test" {
		panic("auth.AllowAll must not be used outside dev/test (set AEGIS_OPA_ENDPOINT instead)")
	}
	return middleware.AuthZDeciderFunc(func(context.Context, middleware.AuthZInput) (bool, error) {
		return true, nil
	})
}
