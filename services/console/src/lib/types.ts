// Shared types — the wire-shape of every public AegisVision API.
// Hand-curated for now; future iteration could generate from /proto.

export interface Page<T> {
  items: T[];
  next_page_token?: string;
}

export interface ProblemJSON {
  type?: string;
  title: string;
  status: number;
  detail?: string;
  instance?: string;
  trace_id?: string;
  [k: string]: unknown;
}

/* ---------- Pipelines ---------- */

export interface OperatorSpec {
  operator: 'ingest' | 'sampler' | 'detect' | 'tracker' | 'rule' | 'emit' | string;
  params?: Record<string, unknown>;
}

export interface Pipeline {
  id: string;
  tenant_id: string;
  project_id: string;
  name: string;
  description?: string;
  dag: OperatorSpec[];
  current_revision_id: string;
  created_at: string;
  updated_at: string;
  etag: string;
}

export interface PipelineRevision {
  id: string;
  pipeline_id: string;
  dag: OperatorSpec[];
  created_by: string;
  created_at: string;
}

/* ---------- Streams ---------- */

export interface Stream {
  id: string;
  tenant_id: string;
  project_id: string;
  name: string;
  protocol: 'rtsp' | 'rtmp' | 'file' | 'webrtc' | 'synthetic';
  url: string;
  pipeline_id: string;
  pipeline_revision_id?: string;
  status: 'pending' | 'running' | 'paused' | 'stopped' | 'error';
  shard?: number;
  tags?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

/* ---------- Models ---------- */

export interface Model {
  id: string;
  name: string;
  triton_name?: string;
  tags?: Record<string, string>;
  created_at: string;
  current_version?: string;
}

export interface ModelVersion {
  id: string;
  model_id: string;
  version: string;
  artifact_uri: string;
  signature?: string;
  promoted_at?: string;
  is_current?: boolean;
}

/* ---------- Datasets ---------- */

export interface Dataset {
  id: string;
  name: string;
  description?: string;
  current_version?: string;
  created_at: string;
}

export interface DatasetVersion {
  id: string;
  dataset_id: string;
  version: string;
  sample_count: number;
  created_at: string;
}

export interface Sample {
  id: string;
  source_stream_id?: string;
  frame_urn?: string;
  created_by?: string;
  created_at: string;
  labels?: Record<string, unknown>;
}

/* ---------- Annotations ---------- */

export interface LabelPolicy {
  id: string;
  name: string;
  current_revision: string;
  created_at: string;
}

export interface LabelPolicyRevision {
  id: string;
  policy_id: string;
  schema: Record<string, unknown>;
  created_at: string;
}

export interface Annotation {
  id: string;
  sample_id: string;
  policy_revision_id: string;
  labels: Record<string, unknown>;
  status?: 'pending' | 'reviewed' | 'rejected';
  reviewer?: string;
  created_at: string;
}

/* ---------- Training ---------- */

export interface TrainingJob {
  id: string;
  dataset_id: string;
  dataset_version: string;
  base_model: string;
  hyperparameters?: Record<string, unknown>;
  gpu_request?: string;
  status: 'pending' | 'running' | 'succeeded' | 'failed' | 'cancelled';
  result_model_id?: string;
  metrics?: Record<string, number>;
  created_at: string;
  finished_at?: string;
}

/* ---------- Media ---------- */

export interface Clip {
  id: string;
  stream_id: string;
  start_ms: string;
  end_ms: string;
  url?: string;
  redacted?: boolean;
  created_at: string;
}

export interface RetentionPolicy {
  id: string;
  ttl_seconds: number;
  redact_faces?: boolean;
  redact_plates?: boolean;
  refuse_raw_sinks?: boolean;
}

/* ---------- Tenants ---------- */

export interface Tenant {
  id: string;
  name: string;
  plan: 'free' | 'pro' | 'enterprise' | 'internal' | string;
  created_at: string;
  member_count?: number;
  project_count?: number;
}

export interface Project {
  id: string;
  tenant_id: string;
  name: string;
  created_at: string;
}

export interface Member {
  id: string;
  user_id: string;
  email?: string;
  role: 'owner' | 'admin' | 'operator' | 'viewer';
  added_at: string;
}

/* ---------- Events ---------- */

export interface Event {
  event_id: string;
  tenant_id: string;
  stream_id?: string;
  pipeline_id?: string;
  ts: string;
  kind: string;
  severity: string;
  payload?: Record<string, unknown>;
  trace_id?: string;
  frame_urn?: string;
}

/* ---------- Agents ---------- */

export interface AgentSession {
  id: string;
  tenant_id: string;
  role: string;
  goal: string;
  max_steps: number;
  status: 'open' | 'pending_gate' | 'completed' | 'failed';
  pending_gate_id?: string;
  created_at: string;
  finished_at?: string;
}

export interface AgentStep {
  id: string;
  session_id: string;
  kind: 'thought' | 'tool_call' | 'tool_result' | 'final';
  tool_name?: string;
  tier?: 0 | 1 | 2 | 3;
  payload?: Record<string, unknown>;
  citations?: { source: string; snippet?: string }[];
  created_at: string;
}

export interface AgentMessageRequest {
  content: string;
}

/* ---------- Gates ---------- */

export interface Gate {
  id: string;
  tool: string;
  args: Record<string, unknown>;
  requester: string;
  tenant_id: string;
  status: 'pending' | 'approved' | 'denied' | 'expired';
  created_at: string;
  resolved_at?: string;
  approver?: string;
  reason?: string;
}

/* ---------- Canary ---------- */

export interface CanaryPlan {
  id: string;
  candidate_model: string;
  baseline_model: string;
  traffic_pct: number;
  min_samples: number;
  wilson_alpha: number;
  promotion_margin_pp: number;
  rollback_margin_pp: number;
  status: 'running' | 'promoted' | 'rolled_back' | 'cancelled' | 'paused';
  created_at: string;
}

export interface CanaryState {
  plan_id: string;
  baseline: { successes: number; failures: number; lower_bound: number };
  candidate: { successes: number; failures: number; lower_bound: number };
  decision: 'hold' | 'promote' | 'rollback';
  reason?: string;
  last_updated_at: string;
}

/* ---------- Drift ---------- */

export interface DriftRun {
  id: string;
  tenant_id: string;
  model: string;
  js: number;
  kl: number;
  tvd: number;
  threshold_breached?: ('js' | 'kl' | 'tvd')[];
  started_at: string;
  finished_at?: string;
}

/* ---------- SLO ---------- */

export interface SloStatus {
  slo: string;
  target_pct: number;
  burn_rate_1h: number;
  burn_rate_6h: number;
  page: boolean;
  ticket: boolean;
}

/* ---------- Prefetch ---------- */

export interface PrefetchCell {
  tenant_id: string;
  model: string;
  hour_of_week: number;
  predicted_rpm: number;
}

/* ---------- Knowledge ---------- */

export interface KnowledgeSnippet {
  text: string;
  source: string;
  score: number;
}

/* ---------- Active learning ---------- */

export interface AlSample {
  id: string;
  tenant_id: string;
  frame_urn: string;
  uncertainty: number;
  diversity_cluster?: string;
  queued_at: string;
  claimed_by?: string;
}

/* ---------- Cost + metering ---------- */

export interface UsageRow {
  tenant_id: string;
  period_start: string;
  period_end: string;
  gpu_seconds: number;
  llm_input_tokens: number;
  llm_output_tokens: number;
  object_store_bytes: number;
  events_persisted: number;
}

export interface Invoice {
  id: string;
  tenant_id: string;
  period: string;
  total_cents: number;
  created_at: string;
}

/* ---------- Compliance ---------- */

export interface Control {
  id: string;
  framework: 'SOC2' | 'EU_AI_ACT' | 'GDPR' | 'CIS_K8S' | string;
  name: string;
  description?: string;
}

export interface EvidenceBundle {
  id: string;
  framework: string;
  controls: string[];
  signed_url?: string;
  created_at: string;
}

/* ---------- Audit ---------- */

export interface AuditRecord {
  id: number;
  hash: string;
  prev_hash: string;
  tenant_id: string;
  actor: string;
  action: string;
  resource: string;
  payload?: Record<string, unknown>;
  ts: string;
}
