// Tool catalog: the capabilities the agent loop can exercise.
//
// Each tool's RiskTier is the load-bearing policy contract — anything tier
// >= 3 forces the agent to open a gate (see ADR-0014/0017). Tools that
// MUTATE production state are tier 3 by default; reversible ones (e.g.
// pausing a draft pipeline) are tier 2.
//
// Tools call downstream services over HTTP. In dev/test, the URL is empty
// and the tool returns a synthetic response so the agent loop still flows.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/aegisvision/pkg/agent"
	"github.com/aegisvision/pkg/llm"
)

// Endpoints lists the downstream service base URLs the tools dispatch to.
// Empty values force the synthetic-response path.
type Endpoints struct {
	EventService     string
	PipelineService  string
	ModelRegistry    string
	MeteringService  string
	KnowledgeService string
	HTTP             *http.Client
}

func (e *Endpoints) httpc() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// Register installs the default Phase 4 tool catalog onto reg.
func Register(reg *agent.Registry, ep *Endpoints) {
	reg.Register(&searchDetections{ep: ep})
	reg.Register(&queryPipeline{ep: ep})
	reg.Register(&queryKnowledge{ep: ep})
	reg.Register(&proposeDAGChange{ep: ep})
	reg.Register(&proposeModelSwap{ep: ep})
	reg.Register(&pauseDraftPipeline{ep: ep})
	reg.Register(&promoteModel{ep: ep})
	reg.Register(&overrideRetention{ep: ep})
}

// --- Tier 0: read-only ---

type searchDetections struct{ ep *Endpoints }

func (t *searchDetections) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "search_detections",
		Description: "Search recent detections by predicate (class, time range, tenant scope is implicit).",
		ParametersSchemaJSON: `{"type":"object","required":["query"],"properties":{
            "query":{"type":"string"},
            "time_from":{"type":"string","description":"ISO timestamp"},
            "time_to":{"type":"string","description":"ISO timestamp"},
            "limit":{"type":"integer","minimum":1,"maximum":200}
        }}`,
		RiskTier: llm.RiskTierRead,
	}
}

func (t *searchDetections) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		Query    string `json:"query"`
		TimeFrom string `json:"time_from"`
		TimeTo   string `json:"time_to"`
		Limit    int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &p)
	if p.Limit == 0 {
		p.Limit = 50
	}
	if t.ep.EventService == "" {
		return map[string]any{
			"hits":  []any{},
			"note":  "synthetic (no AEGIS_EVENT_SERVICE_URL configured)",
			"query": p.Query,
		}, nil
	}
	q := url.Values{}
	q.Set("q", p.Query)
	if p.TimeFrom != "" {
		q.Set("from", p.TimeFrom)
	}
	if p.TimeTo != "" {
		q.Set("to", p.TimeTo)
	}
	q.Set("limit", fmt.Sprintf("%d", p.Limit))
	return getJSON(ctx, t.ep, t.ep.EventService+"/v1/events?"+q.Encode())
}

type queryPipeline struct{ ep *Endpoints }

func (t *queryPipeline) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "get_pipeline",
		Description: "Fetch a pipeline declaration + current status.",
		ParametersSchemaJSON: `{"type":"object","required":["pipeline_id"],"properties":{"pipeline_id":{"type":"string"}}}`,
		RiskTier:    llm.RiskTierRead,
	}
}

func (t *queryPipeline) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		PipelineID string `json:"pipeline_id"`
	}
	_ = json.Unmarshal(args, &p)
	if p.PipelineID == "" {
		return nil, fmt.Errorf("pipeline_id required")
	}
	if t.ep.PipelineService == "" {
		return map[string]any{"id": p.PipelineID, "status": "unknown", "note": "synthetic"}, nil
	}
	return getJSON(ctx, t.ep, t.ep.PipelineService+"/v1/pipelines/"+url.PathEscape(p.PipelineID))
}

type queryKnowledge struct{ ep *Endpoints }

func (t *queryKnowledge) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "query_knowledge",
		Description: "Search the platform knowledge corpus (ADRs / runbooks / incidents). Returns up to 5 cited snippets.",
		ParametersSchemaJSON: `{"type":"object","required":["query"],"properties":{
            "query":{"type":"string"},
            "kinds":{"type":"array","items":{"type":"string"}},
            "limit":{"type":"integer","minimum":1,"maximum":10}
        }}`,
		RiskTier: llm.RiskTierRead,
	}
}

func (t *queryKnowledge) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		Query string   `json:"query"`
		Kinds []string `json:"kinds"`
		Limit int      `json:"limit"`
	}
	_ = json.Unmarshal(args, &p)
	if p.Query == "" {
		return nil, fmt.Errorf("query required")
	}
	if p.Limit == 0 {
		p.Limit = 5
	}
	if t.ep.KnowledgeService == "" {
		return map[string]any{"hits": []any{}, "note": "synthetic"}, nil
	}
	q := url.Values{}
	q.Set("q", p.Query)
	q.Set("limit", fmt.Sprintf("%d", p.Limit))
	for _, k := range p.Kinds {
		q.Add("kind", k)
	}
	return getJSON(ctx, t.ep, t.ep.KnowledgeService+"/v1/knowledge/search?"+q.Encode())
}

// --- Tier 1: advise ---

type proposeDAGChange struct{ ep *Endpoints }

func (t *proposeDAGChange) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "propose_dag_change",
		Description: "Propose a DAG change as a draft (no deploy). Returns the diff + a draft id the user can review.",
		ParametersSchemaJSON: `{"type":"object","required":["pipeline_id","new_dag_json"],"properties":{
            "pipeline_id":{"type":"string"},
            "new_dag_json":{"type":"string"},
            "rationale":{"type":"string"}
        }}`,
		RiskTier: llm.RiskTierAdvise,
	}
}

func (t *proposeDAGChange) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p map[string]any
	_ = json.Unmarshal(args, &p)
	return map[string]any{"draft_id": fmt.Sprintf("draft-%d", time.Now().UnixNano()), "input": p, "deploy": false}, nil
}

type proposeModelSwap struct{ ep *Endpoints }

func (t *proposeModelSwap) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "propose_model_swap",
		Description: "Propose swapping the current detection model for an alternative. Returns a recommendation; no canary started.",
		ParametersSchemaJSON: `{"type":"object","required":["pipeline_id","candidate_model_id"],"properties":{
            "pipeline_id":{"type":"string"},
            "candidate_model_id":{"type":"string"},
            "rationale":{"type":"string"}
        }}`,
		RiskTier: llm.RiskTierAdvise,
	}
}

func (t *proposeModelSwap) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p map[string]any
	_ = json.Unmarshal(args, &p)
	return map[string]any{"recommendation": p, "canary": false}, nil
}

// --- Tier 2: reversible ---

type pauseDraftPipeline struct{ ep *Endpoints }

func (t *pauseDraftPipeline) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "pause_draft_pipeline",
		Description: "Pause a DRAFT pipeline (never a running one). Reversible: resume by un-pausing.",
		ParametersSchemaJSON: `{"type":"object","required":["pipeline_id"],"properties":{"pipeline_id":{"type":"string"}}}`,
		RiskTier:    llm.RiskTierReversible,
	}
}

func (t *pauseDraftPipeline) Run(ctx context.Context, args json.RawMessage) (any, error) {
	var p struct {
		PipelineID string `json:"pipeline_id"`
	}
	_ = json.Unmarshal(args, &p)
	if p.PipelineID == "" {
		return nil, fmt.Errorf("pipeline_id required")
	}
	if t.ep.PipelineService == "" {
		return map[string]any{"id": p.PipelineID, "status": "paused", "note": "synthetic"}, nil
	}
	return postJSON(ctx, t.ep, t.ep.PipelineService+"/v1/pipelines/"+url.PathEscape(p.PipelineID)+":pause", nil)
}

// --- Tier 3: consequential (gated) ---

type promoteModel struct{ ep *Endpoints }

func (t *promoteModel) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "promote_model",
		Description: "Promote a model from canary to primary across the tenant. CONSEQUENTIAL — requires human approval.",
		ParametersSchemaJSON: `{"type":"object","required":["model_id"],"properties":{
            "model_id":{"type":"string"},
            "from_stage":{"type":"string"},
            "to_stage":{"type":"string"}
        }}`,
		RiskTier: llm.RiskTierConsequential,
	}
}

func (t *promoteModel) Run(ctx context.Context, args json.RawMessage) (any, error) {
	// The agent loop refuses to call this — execution only happens after a
	// gate is approved (and even then, the approver wires the action).
	return nil, fmt.Errorf("must be executed via approved gate, not directly")
}

type overrideRetention struct{ ep *Endpoints }

func (t *overrideRetention) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "override_retention",
		Description: "Override media retention policy for a tenant. CONSEQUENTIAL — requires human approval.",
		ParametersSchemaJSON: `{"type":"object","required":["tenant_id","new_ttl_days"],"properties":{
            "tenant_id":{"type":"string"},
            "new_ttl_days":{"type":"integer","minimum":1,"maximum":3650},
            "reason":{"type":"string"}
        }}`,
		RiskTier: llm.RiskTierConsequential,
	}
}

func (t *overrideRetention) Run(ctx context.Context, args json.RawMessage) (any, error) {
	return nil, fmt.Errorf("must be executed via approved gate, not directly")
}

// --- HTTP helpers ---

func getJSON(ctx context.Context, ep *Endpoints, u string) (any, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := ep.httpc().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return map[string]any{"error": resp.Status}, nil
	}
	var out any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func postJSON(ctx context.Context, ep *Endpoints, u string, body any) (any, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ep.httpc().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return map[string]any{"error": resp.Status}, nil
	}
	var out any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, nil
	}
	return out, nil
}
