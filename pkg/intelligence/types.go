// Package intelligence holds the Phase 4 platform-internal domain types that
// mirror proto/aegisvision/intelligence/v1. Services and the agent/LLM libs
// import these directly so they don't pull the protobuf dependency just to
// shape an HTTP payload. The proto files remain the canonical contract
// (regenerate with `task proto`).
package intelligence

import "time"

// --- Agent ---

type AgentRole string

const (
	AgentRoleOperator     AgentRole = "operator"
	AgentRoleOptimizer    AgentRole = "optimizer"
	AgentRoleInvestigator AgentRole = "investigator"
)

type SessionStatus string

const (
	SessionStatusActive        SessionStatus = "active"
	SessionStatusAwaitingGate  SessionStatus = "awaiting_gate"
	SessionStatusCompleted     SessionStatus = "completed"
	SessionStatusFailed        SessionStatus = "failed"
	SessionStatusCanceled      SessionStatus = "canceled"
)

type AgentSession struct {
	ID         string        `json:"id"`
	TenantID   string        `json:"tenant_id"`
	Role       AgentRole     `json:"role"`
	Goal       string        `json:"goal"`
	Status     SessionStatus `json:"status"`
	StepCount  int           `json:"step_count"`
	MaxSteps   int           `json:"max_steps"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

type StepKind string

const (
	StepKindPlan          StepKind = "plan"
	StepKindToolCall      StepKind = "tool_call"
	StepKindToolResult    StepKind = "tool_result"
	StepKindGateRequested StepKind = "gate_requested"
	StepKindGateResolved  StepKind = "gate_resolved"
	StepKindFinal         StepKind = "final"
)

type AgentStep struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Index       int       `json:"index"`
	Kind        StepKind  `json:"kind"`
	Summary     string    `json:"summary"`
	PayloadJSON string    `json:"payload_json,omitempty"`
	GateID      string    `json:"gate_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// --- Gate ---

type GateDecision string

const (
	GateDecisionPending  GateDecision = "pending"
	GateDecisionApproved GateDecision = "approved"
	GateDecisionRejected GateDecision = "rejected"
	GateDecisionExpired  GateDecision = "expired"
)

type GateRequest struct {
	ID             string    `json:"id"`
	SessionID      string    `json:"session_id"`
	TenantID       string    `json:"tenant_id"`
	ToolName       string    `json:"tool_name"`
	ActionSummary  string    `json:"action_summary"`
	ArgumentsJSON  string    `json:"arguments_json"`
	BlastRadius    string    `json:"blast_radius"`
	RequestedAt    time.Time `json:"requested_at"`
}

type Gate struct {
	Request        GateRequest  `json:"request"`
	Decision       GateDecision `json:"decision"`
	DecidedBy      string       `json:"decided_by,omitempty"`
	DecisionReason string       `json:"decision_reason,omitempty"`
	DecidedAt      time.Time    `json:"decided_at,omitempty"`
}

// --- Active learning ---

type UncertaintyStrategy string

const (
	UncertaintyLeastConfidence UncertaintyStrategy = "least_confidence"
	UncertaintyMargin          UncertaintyStrategy = "margin"
	UncertaintyEntropy         UncertaintyStrategy = "entropy"
	UncertaintyDisagreement    UncertaintyStrategy = "disagreement"
)

type LearningPolicy struct {
	ID               string              `json:"id"`
	TenantID         string              `json:"tenant_id"`
	ModelID          string              `json:"model_id"`
	Strategy         UncertaintyStrategy `json:"strategy"`
	SampleRate       float64             `json:"sample_rate"`
	MinUncertainty   float64             `json:"min_uncertainty"`
	MaxUncertainty   float64             `json:"max_uncertainty"`
	DiversityClasses []string            `json:"diversity_classes,omitempty"`
	DailyBudget      int                 `json:"daily_budget"`
	SnapshotTTLDays  int                 `json:"snapshot_ttl_days"`
}

type SampleStatus string

const (
	SampleStatusQueued   SampleStatus = "queued"
	SampleStatusAssigned SampleStatus = "assigned"
	SampleStatusLabeled  SampleStatus = "labeled"
	SampleStatusExpired  SampleStatus = "expired"
	SampleStatusRejected SampleStatus = "rejected"
)

type Sample struct {
	ID                  string       `json:"id"`
	TenantID            string       `json:"tenant_id"`
	ModelID             string       `json:"model_id"`
	PolicyID            string       `json:"policy_id"`
	SnapshotURN         string       `json:"snapshot_urn"`
	PredictedClass      string       `json:"predicted_class"`
	PredictedConfidence float64      `json:"predicted_confidence"`
	Uncertainty         float64      `json:"uncertainty"`
	Status              SampleStatus `json:"status"`
	AnnotationTaskID    string       `json:"annotation_task_id,omitempty"`
	FinalClass          string       `json:"final_class,omitempty"`
	SampledAt           time.Time    `json:"sampled_at"`
	LabeledAt           time.Time    `json:"labeled_at,omitempty"`
}

// --- Knowledge ---

type DocKind string

const (
	DocKindADR          DocKind = "adr"
	DocKindRunbook      DocKind = "runbook"
	DocKindIncident     DocKind = "incident"
	DocKindReadme       DocKind = "readme"
	DocKindAPIReference DocKind = "api_reference"
	DocKindTenantNote   DocKind = "tenant_note"
)

type Doc struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id,omitempty"` // empty == platform-global
	Kind       DocKind   `json:"kind"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	SourceURI  string    `json:"source_uri,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	IngestedAt time.Time `json:"ingested_at"`
}

type SearchHit struct {
	Doc     Doc     `json:"doc"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

// --- NLQ ---

type QueryKind string

const (
	QueryKindEventSearch QueryKind = "event_search"
	QueryKindDAGDraft    QueryKind = "dag_draft"
	QueryKindKnowledge   QueryKind = "knowledge"
)

type EventSearch struct {
	ClickHouseSQL string   `json:"clickhouse_sql"`
	Parameters    []string `json:"parameters,omitempty"`
	Explanation   string   `json:"explanation"`
}

type DAGDraft struct {
	DAGJSON         string `json:"dag_json"`
	Explanation     string `json:"explanation"`
	Validates       bool   `json:"validates"`
	ValidationError string `json:"validation_error,omitempty"`
}

type KnowledgeAnswer struct {
	Text      string   `json:"text"`
	Citations []string `json:"citations,omitempty"`
}

type NLQResponse struct {
	Kind        QueryKind        `json:"kind"`
	EventSearch *EventSearch     `json:"event_search,omitempty"`
	DAGDraft    *DAGDraft        `json:"dag_draft,omitempty"`
	Knowledge   *KnowledgeAnswer `json:"knowledge,omitempty"`
	Safety      map[string]any   `json:"safety_signals,omitempty"`
}
