// Package llm defines the platform-internal LLM/VLM contract.
//
// The proto definitions in proto/aegisvision/intelligence/v1/llm.proto are the
// canonical contract; the types here mirror them in pure Go so callers don't
// pay the protobuf dependency just to make a chat call. When the buf
// toolchain regenerates the Go bindings, both shapes remain interchangeable
// at the wire level (JSON tags match the proto field names exactly).
//
// Risk tiers, from the agent perspective, are part of the tool schema itself
// — see ADR-0014/0017. Tier-3 tools must NOT be auto-executed by the agent
// loop; instead, the agent opens a gate in policy-gate-service and waits.
package llm

import "context"

// Role is the speaker role in a chat conversation. We never expose raw
// "system" prompts to end users; system prompts are owned by the calling
// service and prepended internally.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ImageRef carries a claim-check reference to a frame/image in media-service.
// We never put image bytes on the bus or in protobuf payloads (ADR-008).
type ImageRef struct {
	URN      string `json:"urn"`
	MIMEType string `json:"mime_type,omitempty"`
}

// ToolCall is a model-emitted intent to invoke a tool with a JSON argument
// blob. The blob is opaque to the gateway — the agent dispatches it against
// its registry.
type ToolCall struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ArgumentsJSON string `json:"arguments_json"`
}

// Message is one turn in a chat. content + images + tool_calls are
// mutually-friendly: user turns carry content (+ optional images), assistant
// turns carry content and/or tool_calls, tool turns carry content + a
// tool_call_id binding them to the assistant's request.
type Message struct {
	Role        Role       `json:"role"`
	Content     string     `json:"content,omitempty"`
	Images      []ImageRef `json:"images,omitempty"`
	ToolCalls   []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID  string     `json:"tool_call_id,omitempty"`
	Name        string     `json:"name,omitempty"` // optional named author
}

// ToolSchema describes one tool exposed to the model. Risk tier governs
// whether the agent loop is allowed to execute the result automatically.
type ToolSchema struct {
	Name                 string `json:"name"`
	Description          string `json:"description"`
	ParametersSchemaJSON string `json:"parameters_schema_json"`
	RiskTier             int    `json:"risk_tier"`
}

// Risk tiers — keep stable; the agent loop & policy-gate use these as a wire
// contract. Tiers escalate; never reorder.
const (
	RiskTierRead         = 0 // read-only (no state change anywhere)
	RiskTierAdvise       = 1 // produces advice / draft; no side effects
	RiskTierReversible   = 2 // mutates state but trivially reversible
	RiskTierConsequential = 3 // requires human gate (policy-gate-service)
)

// ChatRequest is the input to Client.Chat.
type ChatRequest struct {
	TenantID           string       `json:"tenant_id"`
	Model              string       `json:"model"`
	Messages           []Message    `json:"messages"`
	Tools              []ToolSchema `json:"tools,omitempty"`
	ToolChoice         string       `json:"tool_choice,omitempty"`
	Temperature        float64      `json:"temperature,omitempty"`
	MaxTokens          int          `json:"max_tokens,omitempty"`
	JSONMode           bool         `json:"json_mode,omitempty"`
	ResponseSchemaJSON string       `json:"response_schema_json,omitempty"`
	Stop               []string     `json:"stop,omitempty"`
	Seed               int64        `json:"seed,omitempty"`
}

// TokenUsage is bookkeeping for metering / cost-accounting.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// SafetySignals is the gateway's view of what the safety layer found:
//
//	pii_redacted       (bool)   — input had PII redacted before send
//	injection_score    (float)  — 0..1; how injection-like the input looked
//	output_pii         (bool)   — output had PII redacted before return
//	refused            (bool)   — the safety layer refused to call the model
//	refusal_reason     (string) — human-readable
type SafetySignals map[string]any

// ChatResponse is the structured response.
type ChatResponse struct {
	ID            string        `json:"id"`
	Model         string        `json:"model"`
	Message       Message       `json:"message"`
	Usage         TokenUsage    `json:"usage"`
	FinishReason  string        `json:"finish_reason"`
	Safety        SafetySignals `json:"safety_signals,omitempty"`
}

// EmbeddingRequest / EmbeddingResponse mirror the gateway shape.
type EmbeddingRequest struct {
	TenantID string   `json:"tenant_id"`
	Model    string   `json:"model"`
	Input    []string `json:"input"`
}

type Embedding struct {
	Vector []float32 `json:"vector"`
	Index  int       `json:"index"`
}

type EmbeddingResponse struct {
	Data  []Embedding `json:"data"`
	Usage TokenUsage  `json:"usage"`
}

// VisionDescribeRequest asks a VLM to describe an image with optional
// instruction. The image MUST be supplied by URN — raw bytes are never
// accepted at this layer.
type VisionDescribeRequest struct {
	TenantID    string   `json:"tenant_id"`
	Model       string   `json:"model"`
	Image       ImageRef `json:"image"`
	Instruction string   `json:"instruction,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"`
}

type VisionDescribeResponse struct {
	Description string        `json:"description"`
	Usage       TokenUsage    `json:"usage"`
	Safety      SafetySignals `json:"safety_signals,omitempty"`
}

// Client is the platform-internal LLM client. Implementations include:
//   - HTTPClient   — talks to llm-gateway (OpenAI-compatible JSON)
//   - FakeClient   — deterministic, in-memory, for unit tests
//   - SafeClient   — wraps any Client with the safety layer (Sanitizer + PII)
type Client interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Embed(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error)
	Describe(ctx context.Context, req VisionDescribeRequest) (*VisionDescribeResponse, error)
}
