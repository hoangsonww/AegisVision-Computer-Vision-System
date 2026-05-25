// Adaptive-autonomy domain types. These mirror
// proto/aegisvision/autonomy/v1; the proto definitions remain the
// canonical contract.
package intelligence

import "time"

// --- Canary ---

type CanaryStatus string

const (
	CanaryStatusDraft       CanaryStatus = "draft"
	CanaryStatusRamping     CanaryStatus = "ramping"
	CanaryStatusHolding     CanaryStatus = "holding"
	CanaryStatusPromoted    CanaryStatus = "promoted"
	CanaryStatusRolledBack  CanaryStatus = "rolled_back"
	CanaryStatusFailed      CanaryStatus = "failed"
	CanaryStatusPaused      CanaryStatus = "paused"
)

type CanaryStep struct {
	CandidatePercent int   `json:"candidate_percent"`
	HoldSeconds      int   `json:"hold_seconds"`
	MinSamples       int64 `json:"min_samples"`
}

type CanaryPlan struct {
	ID                   string       `json:"id"`
	TenantID             string       `json:"tenant_id"`
	ResourceURN          string       `json:"resource_urn"`
	BaselineURN          string       `json:"baseline_urn"`
	CandidateURN         string       `json:"candidate_urn"`
	Ladder               []CanaryStep `json:"ladder"`
	MaxProportionDelta   float64      `json:"max_proportion_delta"`
	MaxLatencyP95MsDelta int          `json:"max_latency_p95_ms_delta"`
	AutoPromote          bool         `json:"auto_promote"`
	Status               CanaryStatus `json:"status"`
	CurrentStep          int          `json:"current_step"`
	LastStepAt           time.Time    `json:"last_step_at,omitempty"`
	FailureReason        string       `json:"failure_reason,omitempty"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

type CanaryObservation struct {
	PlanID         string    `json:"plan_id"`
	Variant        string    `json:"variant"` // "baseline" | "candidate"
	Successes      int64     `json:"successes"`
	Failures       int64     `json:"failures"`
	LatencyP50Ms   int64     `json:"latency_p50_ms"`
	LatencyP95Ms   int64     `json:"latency_p95_ms"`
	LatencyP99Ms   int64     `json:"latency_p99_ms"`
	WindowStart    time.Time `json:"window_start"`
	WindowEnd      time.Time `json:"window_end"`
}

// --- Drift ---

type DivergenceMethod string

const (
	DivergenceKL  DivergenceMethod = "kl"
	DivergenceJS  DivergenceMethod = "js"
	DivergenceTVD DivergenceMethod = "tvd"
)

type Distribution struct {
	Classes     map[string]float64 `json:"classes"`
	Samples     int64              `json:"samples"`
	WindowStart time.Time          `json:"window_start,omitempty"`
	WindowEnd   time.Time          `json:"window_end,omitempty"`
}

type DriftPolicy struct {
	ID                string           `json:"id"`
	TenantID          string           `json:"tenant_id"`
	ModelID           string           `json:"model_id"`
	Method            DivergenceMethod `json:"method"`
	WarnThreshold     float64          `json:"warn_threshold"`
	CriticalThreshold float64          `json:"critical_threshold"`
	WindowSeconds     int              `json:"window_seconds"`
	MinSamples        int64            `json:"min_samples"`
}

type DriftReference struct {
	ID           string       `json:"id"`
	TenantID     string       `json:"tenant_id"`
	ModelID      string       `json:"model_id"`
	Distribution Distribution `json:"distribution"`
	CapturedAt   time.Time    `json:"captured_at"`
}

type DriftSignal struct {
	ID         string           `json:"id"`
	TenantID   string           `json:"tenant_id"`
	ModelID    string           `json:"model_id"`
	Method     DivergenceMethod `json:"method"`
	Divergence float64          `json:"divergence"`
	Severity   string           `json:"severity"` // info | warn | critical
	Observed   Distribution     `json:"observed"`
	EmittedAt  time.Time        `json:"emitted_at"`
}

// --- SLO ---

type SLOTarget struct {
	ID                  string    `json:"id"`
	TenantID            string    `json:"tenant_id"`
	Name                string    `json:"name"`
	PromQL              string    `json:"promql"`
	Target              float64   `json:"target"`
	RollingWindowSeconds int      `json:"rolling_window_seconds"`
	Paused              bool      `json:"paused"`
	CreatedAt           time.Time `json:"created_at,omitempty"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
}

type SLOSignal struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	TargetID   string    `json:"target_id"`
	BurnRate1h float64   `json:"burn_rate_1h"`
	BurnRate6h float64   `json:"burn_rate_6h"`
	BurnRate24h float64  `json:"burn_rate_24h"`
	Severity   string    `json:"severity"` // info | warn | critical
	EmittedAt  time.Time `json:"emitted_at"`
}

// --- Autonomy scheduling ---

type AutonomyAgentRole string

const (
	AutonomyRoleOptimizer   AutonomyAgentRole = "optimizer"
	AutonomyRoleMonitor     AutonomyAgentRole = "monitor"
	AutonomyRoleTrainer     AutonomyAgentRole = "trainer"
	AutonomyRoleReliability AutonomyAgentRole = "reliability"
)

type TriggerKind string

const (
	TriggerCron   TriggerKind = "cron"
	TriggerSignal TriggerKind = "signal"
	TriggerManual TriggerKind = "manual"
)

type Schedule struct {
	ID              string            `json:"id"`
	TenantID        string            `json:"tenant_id"`
	Role            AutonomyAgentRole `json:"role"`
	Trigger         TriggerKind       `json:"trigger"`
	CronExpression  string            `json:"cron_expression,omitempty"`
	SignalSubject   string            `json:"signal_subject,omitempty"`
	MaxSteps        int               `json:"max_steps"`
	MaxAutonomyTier int               `json:"max_autonomy_tier"`
	Paused          bool              `json:"paused"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type RunStatus string

const (
	RunStatusQueued        RunStatus = "queued"
	RunStatusRunning       RunStatus = "running"
	RunStatusAwaitingGate  RunStatus = "awaiting_gate"
	RunStatusCompleted     RunStatus = "completed"
	RunStatusFailed        RunStatus = "failed"
	RunStatusSkipped       RunStatus = "skipped"
)

type AutonomyRun struct {
	ID            string            `json:"id"`
	ScheduleID    string            `json:"schedule_id"`
	TenantID      string            `json:"tenant_id"`
	Role          AutonomyAgentRole `json:"role"`
	Trigger       TriggerKind       `json:"trigger"`
	Status        RunStatus         `json:"status"`
	SessionID     string            `json:"session_id,omitempty"`
	TriggerReason string            `json:"trigger_reason,omitempty"`
	StartedAt     time.Time         `json:"started_at"`
	FinishedAt    time.Time         `json:"finished_at,omitempty"`
	GateID        string            `json:"gate_id,omitempty"`
}
