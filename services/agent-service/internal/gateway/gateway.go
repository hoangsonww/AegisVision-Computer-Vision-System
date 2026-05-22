// HTTPGateway talks to policy-gate-service to open gates and poll for
// resolution. The wire format mirrors intelligence.GateRequest /
// intelligence.Gate.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aegisvision/pkg/agent"
	"github.com/aegisvision/pkg/intelligence"
)

type HTTPGateway struct {
	BaseURL    string
	HTTP       *http.Client
	TenantID   string
	AuthHeader string
}

func New(baseURL, tenant string) *HTTPGateway {
	return &HTTPGateway{
		BaseURL:  baseURL,
		HTTP:     &http.Client{Timeout: 15 * time.Second},
		TenantID: tenant,
	}
}

func (g *HTTPGateway) OpenGate(ctx context.Context, req agent.GateRequest) (string, error) {
	body, _ := json.Marshal(intelligence.GateRequest{
		ID:            newID(),
		SessionID:     req.SessionID,
		TenantID:      req.TenantID,
		ToolName:      req.ToolName,
		ActionSummary: req.ActionSummary,
		ArgumentsJSON: req.ArgumentsJSON,
		BlastRadius:   req.BlastRadius,
		RequestedAt:   time.Now().UTC(),
	})
	hreq, err := http.NewRequestWithContext(ctx, "POST", g.BaseURL+"/v1/gates", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("X-Aegis-Tenant", req.TenantID)
	if g.AuthHeader != "" {
		hreq.Header.Set("Authorization", g.AuthHeader)
	}
	resp, err := g.HTTP.Do(hreq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("open gate: %s: %s", resp.Status, string(b))
	}
	var out intelligence.Gate
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Request.ID, nil
}

func (g *HTTPGateway) GetGate(ctx context.Context, id string) (*agent.Gate, error) {
	hreq, _ := http.NewRequestWithContext(ctx, "GET", g.BaseURL+"/v1/gates/"+id, nil)
	hreq.Header.Set("X-Aegis-Tenant", g.TenantID)
	if g.AuthHeader != "" {
		hreq.Header.Set("Authorization", g.AuthHeader)
	}
	resp, err := g.HTTP.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, errors.New("gate not found")
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get gate: %s: %s", resp.Status, string(b))
	}
	var out intelligence.Gate
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &agent.Gate{
		ID:             out.Request.ID,
		Decision:       string(out.Decision),
		DecidedBy:      out.DecidedBy,
		DecisionReason: out.DecisionReason,
	}, nil
}

func newID() string {
	return fmt.Sprintf("gate-%d", time.Now().UnixNano())
}
