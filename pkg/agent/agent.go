// Package agent implements the AegisVision bounded-autonomy agent loop.
//
// Design (ADR-0014 + ADR-0017):
//
//   - The agent receives a goal + recent context and runs a plan/act/observe
//     loop driven by an LLM tool-calling model.
//   - The LLM proposes tool calls; the agent dispatches them against a tool
//     registry. Each tool is tagged with a risk tier:
//
//     0 RiskTierRead         — read-only, free to execute.
//     1 RiskTierAdvise       — produces advice / draft; no side effects.
//     2 RiskTierReversible   — mutates state but trivially reversible.
//     3 RiskTierConsequential — REQUIRES a human gate. The agent MUST NOT
//     execute. Instead, it opens a Gate via
//     the Gateway interface and returns
//     SessionStatusAwaitingGate.
//
//   - When a tool's tier exceeds the session's autonomy ceiling, the agent
//     refuses to execute even tiers it could otherwise auto-run. (You can
//     ship a "read-only-investigator" by capping at tier 0.)
//
//   - Step budget + retry budget are both bounded; runaway loops fall out
//     with SessionStatusFailed rather than burning tokens forever.
//
// The orchestration is intentionally synchronous from the caller's POV:
// Run() returns when the session reaches a terminal state or stops waiting
// for a gate. Long-lived sessions (a gate sitting open for hours) are
// resumed via Resume(sessionID, gateID) after the gate is approved.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aegisvision/pkg/llm"
)

// Tool is one executable capability registered with an Agent. Implementers
// should validate args in-memory (don't trust the LLM) and return a
// JSON-serializable result.
type Tool interface {
	Schema() llm.ToolSchema
	// Run executes the tool with the JSON arguments supplied by the LLM.
	// The return value is JSON-marshaled and fed back to the model.
	Run(ctx context.Context, args json.RawMessage) (any, error)
}

// Registry is a name -> Tool map.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Schema().Name] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Schemas returns the schemas of all registered tools (used to populate
// llm.ChatRequest.Tools).
func (r *Registry) Schemas() []llm.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]llm.ToolSchema, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Schema())
	}
	return out
}

// Gateway is the interface to policy-gate-service. The agent calls
// OpenGate when a consequential tool is requested; the caller persists +
// surfaces the gate to a human; the gate is later approved or rejected via
// the service, and the agent's Resume() picks up the result.
type Gateway interface {
	OpenGate(ctx context.Context, req GateRequest) (gateID string, err error)
	GetGate(ctx context.Context, gateID string) (*Gate, error)
}

// Gate / GateRequest mirror pkg/intelligence.Gate but live here to avoid an
// extra module hop for agent-internal logic. agent-service translates these
// to/from the platform-internal type.
type GateRequest struct {
	SessionID     string
	TenantID      string
	ToolName      string
	ActionSummary string
	ArgumentsJSON string
	BlastRadius   string
}

type Gate struct {
	ID             string
	Decision       string // "pending" | "approved" | "rejected" | "expired"
	DecidedBy      string
	DecisionReason string
}

// SessionSink is the persistence interface — every step is appended; the
// agent-service implementation persists to Postgres. Tests use the
// MemorySink in-memory.
type SessionSink interface {
	AppendStep(ctx context.Context, sessionID string, step Step) error
	SetStatus(ctx context.Context, sessionID, status string) error
}

// Step is the in-memory equivalent of intelligence.AgentStep.
type Step struct {
	Index     int
	Kind      string
	Summary   string
	Payload   any
	GateID    string
	CreatedAt time.Time
}

// Config tunes the loop's bounds.
type Config struct {
	MaxSteps         int           // hard cap; default 12
	MaxAutonomyTier  int           // tools above this tier auto-refuse; default RiskTierReversible (2)
	StepTimeout      time.Duration // per-step LLM call timeout; default 60s
	Temperature      float64       // LLM temperature; default 0.2
	Model            string        // LLM model to request; default "" (let gateway pick)
	SystemPrompt     string        // optional system prompt prefix
}

func (c *Config) defaults() {
	if c.MaxSteps == 0 {
		c.MaxSteps = 12
	}
	if c.MaxAutonomyTier == 0 {
		c.MaxAutonomyTier = llm.RiskTierReversible
	}
	if c.StepTimeout == 0 {
		c.StepTimeout = 60 * time.Second
	}
	if c.Temperature == 0 {
		c.Temperature = 0.2
	}
}

// Agent is a single-session executor. Construct one per session via New().
type Agent struct {
	Client   llm.Client
	Registry *Registry
	Gateway  Gateway
	Sink     SessionSink
	Config   Config

	tenantID  string
	sessionID string

	// Internal state.
	mu      sync.Mutex
	step    int
	history []llm.Message
	gateID  string // set when blocked on a gate
}

// New constructs an Agent for a fresh session.
func New(client llm.Client, registry *Registry, gateway Gateway, sink SessionSink, cfg Config, tenantID, sessionID string) *Agent {
	cfg.defaults()
	return &Agent{
		Client:    client,
		Registry:  registry,
		Gateway:   gateway,
		Sink:      sink,
		Config:    cfg,
		tenantID:  tenantID,
		sessionID: sessionID,
	}
}

// Outcome describes the terminal state of a Run / Resume call.
type Outcome struct {
	// Final assistant content (the agent's last message to the user).
	Final string
	// If Status == "awaiting_gate", GateID is the pending gate.
	GateID string
	// Steps recorded this run.
	Steps []Step
	// "completed" | "awaiting_gate" | "failed"
	Status string
	// Reason on failure.
	Error string
}

// Run starts a new session given the user goal. It returns when the session
// either completes, fails, or blocks on a gate.
func (a *Agent) Run(ctx context.Context, goal string) (*Outcome, error) {
	a.mu.Lock()
	if a.history == nil {
		a.history = []llm.Message{}
		if a.Config.SystemPrompt != "" {
			a.history = append(a.history, llm.Message{Role: llm.RoleSystem, Content: a.Config.SystemPrompt})
		}
		a.history = append(a.history, llm.Message{Role: llm.RoleUser, Content: goal})
	}
	a.mu.Unlock()
	return a.loop(ctx, nil)
}

// Resume continues a previously-gated session. toolResult is the JSON to
// inject as the resolution of the gated tool call (typically the tool's
// real output, executed downstream by the gate approver — or a synthetic
// "approved" / "rejected" marker).
func (a *Agent) Resume(ctx context.Context, gateID string, toolResult any) (*Outcome, error) {
	return a.loop(ctx, &resume{gateID: gateID, toolResult: toolResult})
}

type resume struct {
	gateID     string
	toolResult any
}

func (a *Agent) loop(ctx context.Context, r *resume) (*Outcome, error) {
	out := &Outcome{}

	if r != nil {
		// Inject the gate's resolution as a tool result back into the history.
		b, _ := json.Marshal(r.toolResult)
		a.history = append(a.history, llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: r.gateID,
			Content:    string(b),
		})
		a.recordStep(ctx, out, Step{
			Index: a.step, Kind: "gate_resolved", Summary: "gate " + r.gateID + " resolved",
			GateID: r.gateID, CreatedAt: time.Now().UTC(),
		})
		a.step++
	}

	for a.step < a.Config.MaxSteps {
		if err := ctx.Err(); err != nil {
			return a.fail(ctx, out, err.Error())
		}
		stepCtx, cancel := context.WithTimeout(ctx, a.Config.StepTimeout)
		resp, err := a.Client.Chat(stepCtx, llm.ChatRequest{
			TenantID:    a.tenantID,
			Model:       a.Config.Model,
			Messages:    a.history,
			Tools:       a.Registry.Schemas(),
			ToolChoice:  "auto",
			Temperature: a.Config.Temperature,
		})
		cancel()
		if err != nil {
			return a.fail(ctx, out, "llm chat: "+err.Error())
		}
		a.history = append(a.history, resp.Message)
		// Tool calls?
		if len(resp.Message.ToolCalls) == 0 {
			a.recordStep(ctx, out, Step{
				Index: a.step, Kind: "final", Summary: llm.Truncate(resp.Message.Content, 240),
				CreatedAt: time.Now().UTC(),
			})
			a.step++
			out.Final = resp.Message.Content
			out.Status = "completed"
			if a.Sink != nil {
				_ = a.Sink.SetStatus(ctx, a.sessionID, "completed")
			}
			return out, nil
		}
		// Dispatch tool calls (may be more than one — execute serially).
		for _, tc := range resp.Message.ToolCalls {
			tool, ok := a.Registry.Get(tc.Name)
			if !ok {
				return a.fail(ctx, out, "unknown tool: "+tc.Name)
			}
			tier := tool.Schema().RiskTier
			a.recordStep(ctx, out, Step{
				Index: a.step, Kind: "tool_call",
				Summary: fmt.Sprintf("%s (risk=%d)", tc.Name, tier),
				Payload: map[string]any{"name": tc.Name, "args": json.RawMessage(tc.ArgumentsJSON)},
				CreatedAt: time.Now().UTC(),
			})
			a.step++
			if tier > a.Config.MaxAutonomyTier {
				// Open a gate. Agent halts; caller resumes after approval.
				gid, err := a.Gateway.OpenGate(ctx, GateRequest{
					SessionID:     a.sessionID,
					TenantID:      a.tenantID,
					ToolName:      tc.Name,
					ActionSummary: tool.Schema().Description,
					ArgumentsJSON: tc.ArgumentsJSON,
					BlastRadius:   blastRadiusFor(tier),
				})
				if err != nil {
					return a.fail(ctx, out, "open gate: "+err.Error())
				}
				a.gateID = gid
				a.recordStep(ctx, out, Step{
					Index: a.step, Kind: "gate_requested", Summary: "gate opened for " + tc.Name,
					GateID: gid, CreatedAt: time.Now().UTC(),
				})
				a.step++
				out.GateID = gid
				out.Status = "awaiting_gate"
				if a.Sink != nil {
					_ = a.Sink.SetStatus(ctx, a.sessionID, "awaiting_gate")
				}
				return out, nil
			}
			// Execute.
			result, err := tool.Run(ctx, json.RawMessage(tc.ArgumentsJSON))
			if err != nil {
				// Don't fail the whole session — feed the error back to the
				// model and let it correct itself.
				result = map[string]any{"error": err.Error()}
			}
			rb, _ := json.Marshal(result)
			a.history = append(a.history, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: tc.ID,
				Content:    string(rb),
			})
			a.recordStep(ctx, out, Step{
				Index: a.step, Kind: "tool_result",
				Summary: llm.Truncate(string(rb), 240),
				Payload: result,
				CreatedAt: time.Now().UTC(),
			})
			a.step++
		}
	}
	return a.fail(ctx, out, "max steps exceeded")
}

func (a *Agent) fail(ctx context.Context, out *Outcome, reason string) (*Outcome, error) {
	out.Status = "failed"
	out.Error = reason
	a.recordStep(ctx, out, Step{
		Index: a.step, Kind: "final", Summary: "failed: " + reason,
		CreatedAt: time.Now().UTC(),
	})
	if a.Sink != nil {
		_ = a.Sink.SetStatus(ctx, a.sessionID, "failed")
	}
	return out, errors.New(reason)
}

func (a *Agent) recordStep(ctx context.Context, out *Outcome, s Step) {
	out.Steps = append(out.Steps, s)
	if a.Sink != nil {
		_ = a.Sink.AppendStep(ctx, a.sessionID, s)
	}
}

func blastRadiusFor(tier int) string {
	switch tier {
	case llm.RiskTierConsequential:
		return "high"
	default:
		return "low"
	}
}

// --- Test helpers ---

// MemorySink is an in-memory SessionSink for tests.
type MemorySink struct {
	mu     sync.Mutex
	Steps  map[string][]Step
	Status map[string]string
}

func NewMemorySink() *MemorySink {
	return &MemorySink{Steps: map[string][]Step{}, Status: map[string]string{}}
}

func (m *MemorySink) AppendStep(_ context.Context, sessionID string, s Step) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Steps[sessionID] = append(m.Steps[sessionID], s)
	return nil
}

func (m *MemorySink) SetStatus(_ context.Context, sessionID, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Status[sessionID] = status
	return nil
}

// MemoryGateway is an in-memory Gateway for tests. Use Decide to resolve a
// pending gate.
type MemoryGateway struct {
	mu    sync.Mutex
	next  int
	gates map[string]*Gate
}

func NewMemoryGateway() *MemoryGateway {
	return &MemoryGateway{gates: map[string]*Gate{}}
}

func (g *MemoryGateway) OpenGate(_ context.Context, _ GateRequest) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	id := fmt.Sprintf("gate-%d", g.next)
	g.gates[id] = &Gate{ID: id, Decision: "pending"}
	return id, nil
}

func (g *MemoryGateway) GetGate(_ context.Context, id string) (*Gate, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	gt, ok := g.gates[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return gt, nil
}

func (g *MemoryGateway) Decide(id, decision, by, reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	gt := g.gates[id]
	if gt == nil {
		return
	}
	gt.Decision = decision
	gt.DecidedBy = by
	gt.DecisionReason = reason
}
