// Package agentclient is a thin HTTP client for agent-service. The
// autonomy-orchestrator opens an agent session per scheduled fire and
// per accepted signal; this is the wire it uses.
package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL    string
	HTTP       *http.Client
	AuthHeader string
}

func New(baseURL string) *Client {
	return &Client{BaseURL: baseURL, HTTP: &http.Client{Timeout: 60 * time.Second}}
}

type StartRequest struct {
	Role     string `json:"role"`
	Goal     string `json:"goal"`
	MaxSteps int    `json:"max_steps"`
}

type StartResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	GateID    string `json:"gate_id,omitempty"`
	Final     string `json:"final,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (c *Client) StartSession(ctx context.Context, tenant string, req StartRequest) (*StartResponse, error) {
	if c.BaseURL == "" {
		return &StartResponse{Status: "completed", SessionID: fmt.Sprintf("dev-sess-%d", time.Now().UnixNano())}, nil
	}
	body, _ := json.Marshal(req)
	hreq, _ := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/agents/sessions", bytes.NewReader(body))
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("X-Aegis-Tenant", tenant)
	if c.AuthHeader != "" {
		hreq.Header.Set("Authorization", c.AuthHeader)
	}
	resp, err := c.HTTP.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("start session: %s: %s", resp.Status, string(b))
	}
	var out StartResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.SessionID == "" {
		return nil, errors.New("session response missing id")
	}
	return &out, nil
}
