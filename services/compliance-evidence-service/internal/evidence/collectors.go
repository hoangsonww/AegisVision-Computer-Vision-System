// Per-source Collectors. Each one knows the upstream service's REST shape
// and projects it onto Records. They're deliberately small + testable.
package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// AuditCollector queries audit-service /v1/audit and emits CC6.1/CC7.3
// records.
type AuditCollector struct {
	BaseURL string
	HTTP    *http.Client
}

func NewAuditCollector(baseURL string) *AuditCollector {
	return &AuditCollector{BaseURL: baseURL, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (a *AuditCollector) Name() string         { return "audit-service" }
func (a *AuditCollector) Controls() []Control  { return []Control{CC6_1, CC7_3} }

type auditEnvelope struct {
	Items []struct {
		ID         string    `json:"id"`
		TenantID   string    `json:"tenant_id"`
		At         time.Time `json:"at"`
		Service    string    `json:"service"`
		Method     string    `json:"method"`
		Path       string    `json:"path"`
		Status     int       `json:"status"`
		Outcome    string    `json:"outcome"`
		UserAgent  string    `json:"user_agent"`
		Subject    string    `json:"actor_subject"`
	} `json:"items"`
}

func (a *AuditCollector) Collect(ctx context.Context, req Request) ([]Record, error) {
	if a.BaseURL == "" {
		return nil, nil
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(req.Limit))
	if req.Control != "" {
		q.Set("control", string(req.Control))
	}
	u := fmt.Sprintf("%s/v1/audit?%s", a.BaseURL, q.Encode())
	hreq, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	hreq.Header.Set("X-Aegis-Tenant", req.TenantID)
	resp, err := a.HTTP.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("audit: %s: %s", resp.Status, string(b))
	}
	var body auditEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(body.Items))
	for _, it := range body.Items {
		if !inRange(it.At, req.From, req.To) {
			continue
		}
		ctl := CC7_3 // default — security-event detection
		if it.Outcome == "denied" || it.Outcome == "unauthorized" {
			ctl = CC6_1
		}
		out = append(out, Record{
			Control: ctl, TenantID: it.TenantID, OccurredAt: it.At,
			Source: "audit-service",
			Summary: fmt.Sprintf("%s %s %s → %d", it.Service, it.Method, it.Path, it.Status),
			Reference: it.ID, Outcome: it.Outcome, ActorSubject: it.Subject,
		})
	}
	return out, nil
}

// GateCollector queries policy-gate-service for CC6.6 (consequential-action approval).
type GateCollector struct {
	BaseURL string
	HTTP    *http.Client
}

func NewGateCollector(baseURL string) *GateCollector {
	return &GateCollector{BaseURL: baseURL, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (g *GateCollector) Name() string        { return "policy-gate-service" }
func (g *GateCollector) Controls() []Control { return []Control{CC6_6} }

type gateEnvelope struct {
	Items []struct {
		Request struct {
			ID            string    `json:"id"`
			SessionID     string    `json:"session_id"`
			TenantID      string    `json:"tenant_id"`
			ToolName      string    `json:"tool_name"`
			ActionSummary string    `json:"action_summary"`
			BlastRadius   string    `json:"blast_radius"`
			RequestedAt   time.Time `json:"requested_at"`
		} `json:"request"`
		Decision       string    `json:"decision"`
		DecidedBy      string    `json:"decided_by"`
		DecisionReason string    `json:"decision_reason"`
		DecidedAt      time.Time `json:"decided_at"`
	} `json:"items"`
}

func (g *GateCollector) Collect(ctx context.Context, req Request) ([]Record, error) {
	if g.BaseURL == "" {
		return nil, nil
	}
	hreq, _ := http.NewRequestWithContext(ctx, "GET", g.BaseURL+"/v1/gates?all=true", nil)
	hreq.Header.Set("X-Aegis-Tenant", req.TenantID)
	resp, err := g.HTTP.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gate: %s", resp.Status)
	}
	var body gateEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(body.Items))
	for _, it := range body.Items {
		ts := it.DecidedAt
		if ts.IsZero() {
			ts = it.Request.RequestedAt
		}
		if !inRange(ts, req.From, req.To) {
			continue
		}
		out = append(out, Record{
			Control: CC6_6, TenantID: it.Request.TenantID, OccurredAt: ts,
			Source: "policy-gate-service",
			Summary: fmt.Sprintf("tool=%s blast=%s decision=%s reason=%s",
				it.Request.ToolName, it.Request.BlastRadius, it.Decision, it.DecisionReason),
			Reference: it.Request.ID, Outcome: it.Decision, ActorSubject: it.DecidedBy,
		})
	}
	return out, nil
}

// SLOCollector queries slo-watchdog for CC7.2 (SLO monitoring).
type SLOCollector struct {
	BaseURL string
	HTTP    *http.Client
}

func NewSLOCollector(baseURL string) *SLOCollector {
	return &SLOCollector{BaseURL: baseURL, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (s *SLOCollector) Name() string        { return "slo-watchdog" }
func (s *SLOCollector) Controls() []Control { return []Control{CC7_2} }

type sloEnvelope struct {
	Items []struct {
		ID         string    `json:"id"`
		TenantID   string    `json:"tenant_id"`
		TargetID   string    `json:"target_id"`
		BurnRate1h float64   `json:"burn_rate_1h"`
		BurnRate6h float64   `json:"burn_rate_6h"`
		Severity   string    `json:"severity"`
		EmittedAt  time.Time `json:"emitted_at"`
	} `json:"items"`
}

func (s *SLOCollector) Collect(ctx context.Context, req Request) ([]Record, error) {
	if s.BaseURL == "" {
		return nil, nil
	}
	q := url.Values{}
	q.Set("since", req.From.Format(time.RFC3339))
	hreq, _ := http.NewRequestWithContext(ctx, "GET", s.BaseURL+"/v1/slo/signals?"+q.Encode(), nil)
	hreq.Header.Set("X-Aegis-Tenant", req.TenantID)
	resp, err := s.HTTP.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("slo: %s", resp.Status)
	}
	var body sloEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(body.Items))
	for _, it := range body.Items {
		if !inRange(it.EmittedAt, req.From, req.To) {
			continue
		}
		out = append(out, Record{
			Control: CC7_2, TenantID: it.TenantID, OccurredAt: it.EmittedAt,
			Source: "slo-watchdog",
			Summary: fmt.Sprintf("burn 1h=%.2f 6h=%.2f", it.BurnRate1h, it.BurnRate6h),
			Reference: it.ID, Severity: it.Severity,
		})
	}
	return out, nil
}

// DriftCollector queries drift-detection-service for CC7.4 (incident
// response / ML monitoring).
type DriftCollector struct {
	BaseURL string
	HTTP    *http.Client
}

func NewDriftCollector(baseURL string) *DriftCollector {
	return &DriftCollector{BaseURL: baseURL, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (d *DriftCollector) Name() string        { return "drift-detection-service" }
func (d *DriftCollector) Controls() []Control { return []Control{CC7_4} }

type driftEnvelope struct {
	Items []struct {
		ID         string    `json:"id"`
		TenantID   string    `json:"tenant_id"`
		ModelID    string    `json:"model_id"`
		Method     string    `json:"method"`
		Divergence float64   `json:"divergence"`
		Severity   string    `json:"severity"`
		EmittedAt  time.Time `json:"emitted_at"`
	} `json:"items"`
}

func (d *DriftCollector) Collect(ctx context.Context, req Request) ([]Record, error) {
	if d.BaseURL == "" {
		return nil, nil
	}
	q := url.Values{}
	q.Set("since", req.From.Format(time.RFC3339))
	hreq, _ := http.NewRequestWithContext(ctx, "GET", d.BaseURL+"/v1/drift/signals?"+q.Encode(), nil)
	hreq.Header.Set("X-Aegis-Tenant", req.TenantID)
	resp, err := d.HTTP.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("drift: %s", resp.Status)
	}
	var body driftEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(body.Items))
	for _, it := range body.Items {
		if !inRange(it.EmittedAt, req.From, req.To) {
			continue
		}
		out = append(out, Record{
			Control: CC7_4, TenantID: it.TenantID, OccurredAt: it.EmittedAt,
			Source: "drift-detection-service",
			Summary: fmt.Sprintf("model=%s method=%s divergence=%.3f",
				it.ModelID, it.Method, it.Divergence),
			Reference: it.ID, Severity: it.Severity,
		})
	}
	return out, nil
}

func inRange(t, from, to time.Time) bool {
	return !t.Before(from) && !t.After(to)
}
