// Single-source typed client for the entire AegisVision public API.
// Centralizes tenant header, idempotency, problem+json parsing, base URL.
//
// API_BASE is read from NEXT_PUBLIC_AEGIS_API_BASE (browser-visible) and
// defaults to http://localhost:8080 for the walking-skeleton.

import type {
  Page, ProblemJSON,
  Pipeline, PipelineRevision, OperatorSpec,
  Stream,
  Model, ModelVersion,
  Dataset, DatasetVersion, Sample,
  LabelPolicy, LabelPolicyRevision, Annotation,
  TrainingJob,
  Clip, RetentionPolicy,
  Tenant, Project, Member,
  Event,
  AgentSession, AgentStep, AgentMessageRequest,
  Gate,
  CanaryPlan, CanaryState,
  DriftRun,
  SloStatus,
  PrefetchCell,
  KnowledgeSnippet,
  AlSample,
  UsageRow, Invoice,
  Control, EvidenceBundle,
  AuditRecord,
} from './types';

const DEFAULT_BASE =
  typeof window === 'undefined'
    ? process.env.AEGIS_API_BASE ?? 'http://localhost:8080'
    : process.env.NEXT_PUBLIC_AEGIS_API_BASE ?? 'http://localhost:8080';

/* ---------- core client ---------- */

export class ApiError extends Error {
  problem: ProblemJSON;
  constructor(p: ProblemJSON) {
    super(p.title || p.detail || `HTTP ${p.status}`);
    this.problem = p;
  }
}

export interface ApiOpts {
  tenant?: string;
  idempotencyKey?: string;
  signal?: AbortSignal;
}

function uuid() {
  return (
    (typeof crypto !== 'undefined' && crypto.randomUUID && crypto.randomUUID()) ||
    Math.random().toString(36).slice(2) + Date.now().toString(36)
  );
}

async function request<T>(
  method: 'GET' | 'POST' | 'PATCH' | 'DELETE',
  path: string,
  body: unknown | undefined,
  opts: ApiOpts = {}
): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
  };
  if (opts.tenant) {
    headers['X-Tenant-Id'] = opts.tenant;
    headers['X-Aegis-Tenant'] = opts.tenant;
  }
  if (body !== undefined && body !== null) {
    headers['Content-Type'] = 'application/json';
  }
  if (method !== 'GET' && method !== 'DELETE') {
    headers['Idempotency-Key'] = opts.idempotencyKey ?? uuid();
  }

  const res = await fetch(`${DEFAULT_BASE}${path}`, {
    method,
    headers,
    body: body == null ? undefined : JSON.stringify(body),
    signal: opts.signal,
    credentials: 'include',
  });
  if (!res.ok) {
    let p: ProblemJSON;
    try {
      p = await res.json();
    } catch {
      p = { title: res.statusText, status: res.status };
    }
    if (!p.status) p.status = res.status;
    throw new ApiError(p);
  }
  if (res.status === 204) return undefined as unknown as T;
  const text = await res.text();
  if (!text) return undefined as unknown as T;
  return JSON.parse(text) as T;
}

/* ---------- API surface ---------- */

export const api = {
  base: () => DEFAULT_BASE,

  /* health */
  health: (o?: ApiOpts) => request<{ status: string }>('GET', '/v1/health', undefined, o),

  /* pipelines */
  listPipelines: (o?: ApiOpts) => request<Page<Pipeline>>('GET', '/v1/pipelines', undefined, o),
  getPipeline: (id: string, o?: ApiOpts) => request<Pipeline>('GET', `/v1/pipelines/${id}`, undefined, o),
  createPipeline: (body: { name: string; project_id: string; description?: string; dag: OperatorSpec[] }, o?: ApiOpts) =>
    request<Pipeline>('POST', '/v1/pipelines', body, o),
  updatePipeline: (id: string, body: Partial<Pipeline>, o?: ApiOpts) =>
    request<Pipeline>('PATCH', `/v1/pipelines/${id}`, body, o),
  deletePipeline: (id: string, o?: ApiOpts) => request<void>('DELETE', `/v1/pipelines/${id}`, undefined, o),
  listRevisions: (id: string, o?: ApiOpts) =>
    request<Page<PipelineRevision>>('GET', `/v1/pipelines/${id}/revisions`, undefined, o),
  createRevision: (id: string, body: { dag: OperatorSpec[] }, o?: ApiOpts) =>
    request<PipelineRevision>('POST', `/v1/pipelines/${id}/revisions`, body, o),
  promoteRevision: (id: string, revisionId: string, o?: ApiOpts) =>
    request<Pipeline>('POST', `/v1/pipelines/${id}/promote-revision`, { revision_id: revisionId }, o),

  /* streams */
  listStreams: (o?: ApiOpts) => request<Page<Stream>>('GET', '/v1/streams', undefined, o),
  getStream: (id: string, o?: ApiOpts) => request<Stream>('GET', `/v1/streams/${id}`, undefined, o),
  createStream: (
    body: { name: string; project_id: string; protocol: Stream['protocol']; url: string; pipeline_id: string; pipeline_revision_id?: string; tags?: Record<string, string> },
    o?: ApiOpts
  ) => request<Stream>('POST', '/v1/streams', body, o),
  pauseStream: (id: string, o?: ApiOpts) => request<Stream>('POST', `/v1/streams/${id}/pause`, {}, o),
  resumeStream: (id: string, o?: ApiOpts) => request<Stream>('POST', `/v1/streams/${id}/resume`, {}, o),
  deleteStream: (id: string, o?: ApiOpts) => request<void>('DELETE', `/v1/streams/${id}`, undefined, o),

  /* models */
  listModels: (o?: ApiOpts) => request<Page<Model>>('GET', '/v1/models', undefined, o),
  getModel: (id: string, o?: ApiOpts) => request<Model>('GET', `/v1/models/${id}`, undefined, o),
  registerModel: (body: { name: string; triton_name?: string; tags?: Record<string, string> }, o?: ApiOpts) =>
    request<Model>('POST', '/v1/models', body, o),
  listVersions: (id: string, o?: ApiOpts) =>
    request<Page<ModelVersion>>('GET', `/v1/models/${id}/versions`, undefined, o),
  addVersion: (id: string, body: { version: string; artifact_uri: string; signature?: string }, o?: ApiOpts) =>
    request<ModelVersion>('POST', `/v1/models/${id}/versions`, body, o),
  promoteModel: (id: string, body: { version: string; gate_id?: string }, o?: ApiOpts) =>
    request<Model>('POST', `/v1/models/${id}/promote`, body, o),

  /* datasets */
  listDatasets: (o?: ApiOpts) => request<Page<Dataset>>('GET', '/v1/datasets', undefined, o),
  getDataset: (id: string, o?: ApiOpts) => request<Dataset>('GET', `/v1/datasets/${id}`, undefined, o),
  createDataset: (body: { name: string; description?: string }, o?: ApiOpts) =>
    request<Dataset>('POST', '/v1/datasets', body, o),
  listDatasetVersions: (id: string, o?: ApiOpts) =>
    request<Page<DatasetVersion>>('GET', `/v1/datasets/${id}/versions`, undefined, o),
  cutDatasetVersion: (id: string, body: { version: string }, o?: ApiOpts) =>
    request<DatasetVersion>('POST', `/v1/datasets/${id}/versions`, body, o),
  listSamples: (id: string, ver: string, o?: ApiOpts) =>
    request<Page<Sample>>('GET', `/v1/datasets/${id}/versions/${ver}/samples`, undefined, o),

  /* annotations */
  listLabelPolicies: (o?: ApiOpts) => request<Page<LabelPolicy>>('GET', '/v1/label-policies', undefined, o),
  createLabelPolicy: (body: { name: string; schema: Record<string, unknown> }, o?: ApiOpts) =>
    request<LabelPolicy>('POST', '/v1/label-policies', body, o),
  listLabelPolicyRevisions: (id: string, o?: ApiOpts) =>
    request<Page<LabelPolicyRevision>>('GET', `/v1/label-policies/${id}/revisions`, undefined, o),
  listAnnotations: (o?: ApiOpts) => request<Page<Annotation>>('GET', '/v1/annotations', undefined, o),
  createAnnotation: (body: { sample_id: string; policy_revision_id: string; labels: Record<string, unknown> }, o?: ApiOpts) =>
    request<Annotation>('POST', '/v1/annotations', body, o),
  reviewAnnotation: (id: string, body: { status: 'reviewed' | 'rejected'; reviewer: string }, o?: ApiOpts) =>
    request<Annotation>('POST', `/v1/annotations/${id}/review`, body, o),

  /* training */
  listTrainingJobs: (o?: ApiOpts) => request<Page<TrainingJob>>('GET', '/v1/training-jobs', undefined, o),
  getTrainingJob: (id: string, o?: ApiOpts) => request<TrainingJob>('GET', `/v1/training-jobs/${id}`, undefined, o),
  startTraining: (body: {
    dataset_id: string; dataset_version: string; base_model: string;
    hyperparameters?: Record<string, unknown>; gpu_request?: string;
  }, o?: ApiOpts) => request<TrainingJob>('POST', '/v1/training-jobs', body, o),
  cancelTraining: (id: string, o?: ApiOpts) => request<TrainingJob>('POST', `/v1/training-jobs/${id}/cancel`, {}, o),

  /* media */
  listClips: (o?: ApiOpts) => request<Page<Clip>>('GET', '/v1/media/clips', undefined, o),
  requestClip: (body: { stream_id: string; start_ms: string; end_ms: string }, o?: ApiOpts) =>
    request<Clip>('POST', '/v1/media/clips', body, o),
  getClip: (id: string, o?: ApiOpts) => request<Clip>('GET', `/v1/media/clips/${id}`, undefined, o),
  listRetention: (o?: ApiOpts) => request<Page<RetentionPolicy>>('GET', '/v1/media/retention-policies', undefined, o),
  setRetention: (body: RetentionPolicy, o?: ApiOpts) =>
    request<RetentionPolicy>('POST', '/v1/media/retention-policies', body, o),

  /* tenants */
  listTenants: (o?: ApiOpts) => request<Page<Tenant>>('GET', '/v1/tenants', undefined, o),
  getTenant: (id: string, o?: ApiOpts) => request<Tenant>('GET', `/v1/tenants/${id}`, undefined, o),
  createTenant: (body: { name: string; plan: string }, o?: ApiOpts) =>
    request<Tenant>('POST', '/v1/tenants', body, o),
  deleteTenant: (id: string, o?: ApiOpts) => request<void>('DELETE', `/v1/tenants/${id}`, undefined, o),
  listProjects: (id: string, o?: ApiOpts) =>
    request<Page<Project>>('GET', `/v1/tenants/${id}/projects`, undefined, o),
  createProject: (id: string, body: { name: string }, o?: ApiOpts) =>
    request<Project>('POST', `/v1/tenants/${id}/projects`, body, o),
  listMembers: (id: string, o?: ApiOpts) =>
    request<Page<Member>>('GET', `/v1/tenants/${id}/members`, undefined, o),
  addMember: (id: string, body: { user_id: string; role: Member['role'] }, o?: ApiOpts) =>
    request<Member>('POST', `/v1/tenants/${id}/members`, body, o),

  /* events */
  listEvents: (params: { stream_id?: string; from?: string; to?: string; page_size?: number; page_token?: string }, o?: ApiOpts) => {
    const q = new URLSearchParams(Object.entries(params).filter(([, v]) => v != null).map(([k, v]) => [k, String(v)]));
    return request<Page<Event>>('GET', `/v1/events?${q}`, undefined, o);
  },
  getEvent: (id: string, o?: ApiOpts) => request<Event>('GET', `/v1/events/${id}`, undefined, o),
  sseUrl: (params: { stream_id?: string; tenant_id: string }) => {
    const q = new URLSearchParams(Object.entries(params).filter(([, v]) => v != null).map(([k, v]) => [k, String(v)]));
    return `${DEFAULT_BASE}/v1/events:stream?${q}`;
  },

  /* agents */
  openAgentSession: (body: { role: string; goal: string; max_steps?: number }, o?: ApiOpts) =>
    request<AgentSession>('POST', '/v1/agents/sessions', body, o),
  getAgentSession: (id: string, o?: ApiOpts) => request<AgentSession>('GET', `/v1/agents/sessions/${id}`, undefined, o),
  listAgentSessions: (o?: ApiOpts) => request<Page<AgentSession>>('GET', '/v1/agents/sessions', undefined, o),
  listAgentSteps: (id: string, o?: ApiOpts) =>
    request<Page<AgentStep>>('GET', `/v1/agents/sessions/${id}/steps`, undefined, o),
  sendAgentMessage: (id: string, body: AgentMessageRequest, o?: ApiOpts) =>
    request<AgentSession>('POST', `/v1/agents/sessions/${id}/messages`, body, o),
  resumeAgent: (id: string, body: { gate_id: string; tool_result?: unknown }, o?: ApiOpts) =>
    request<AgentSession>('POST', `/v1/agents/sessions/${id}/resume`, body, o),
  closeAgentSession: (id: string, o?: ApiOpts) =>
    request<void>('DELETE', `/v1/agents/sessions/${id}`, undefined, o),

  /* gates */
  listGates: (o?: ApiOpts) => request<Page<Gate>>('GET', '/v1/gates', undefined, o),
  getGate: (id: string, o?: ApiOpts) => request<Gate>('GET', `/v1/gates/${id}`, undefined, o),
  approveGate: (id: string, body: { reason?: string }, o?: ApiOpts) =>
    request<Gate>('POST', `/v1/gates/${id}/approve`, body, o),
  denyGate: (id: string, body: { reason: string }, o?: ApiOpts) =>
    request<Gate>('POST', `/v1/gates/${id}/deny`, body, o),

  /* canary */
  listCanary: (o?: ApiOpts) => request<Page<CanaryPlan>>('GET', '/v1/canary-plans', undefined, o),
  getCanary: (id: string, o?: ApiOpts) => request<CanaryPlan>('GET', `/v1/canary-plans/${id}`, undefined, o),
  getCanaryState: (id: string, o?: ApiOpts) =>
    request<CanaryState>('GET', `/v1/canary-plans/${id}/state`, undefined, o),
  submitCanary: (body: Omit<CanaryPlan, 'id' | 'status' | 'created_at'>, o?: ApiOpts) =>
    request<CanaryPlan>('POST', '/v1/canary-plans', body, o),
  cancelCanary: (id: string, o?: ApiOpts) =>
    request<CanaryPlan>('POST', `/v1/canary-plans/${id}/cancel`, {}, o),
  pauseCanary: (id: string, o?: ApiOpts) =>
    request<CanaryPlan>('POST', `/v1/canary/plans/${id}/pause`, {}, o),
  resumeCanary: (id: string, o?: ApiOpts) =>
    request<CanaryPlan>('POST', `/v1/canary/plans/${id}/resume`, {}, o),

  /* drift */
  listDriftRuns: (o?: ApiOpts) => request<Page<DriftRun>>('GET', '/v1/drift/runs', undefined, o),
  getDriftRun: (id: string, o?: ApiOpts) => request<DriftRun>('GET', `/v1/drift/runs/${id}`, undefined, o),
  triggerDriftRun: (body: { model: string }, o?: ApiOpts) =>
    request<DriftRun>('POST', '/v1/drift/runs', body, o),

  /* slo */
  listSlos: (o?: ApiOpts) => request<{ items: SloStatus[] }>('GET', '/v1/slo', undefined, o),

  /* prefetch */
  getPrefetchGrid: (o?: ApiOpts) => request<{ cells: PrefetchCell[] }>('GET', '/v1/prefetch/grid', undefined, o),

  /* knowledge */
  queryKnowledge: (body: { query: string; k?: number }, o?: ApiOpts) =>
    request<{ snippets: KnowledgeSnippet[]; trace_id?: string }>('POST', '/v1/knowledge/query', body, o),
  ingestKnowledge: (body: { path?: string }, o?: ApiOpts) =>
    request<{ chunks_indexed: number }>('POST', '/v1/knowledge/ingest', body, o),
  getKnowledgeIndex: (o?: ApiOpts) =>
    request<{ size_chunks: number; last_ingest_at?: string }>('GET', '/v1/knowledge/index', undefined, o),

  /* nlq */
  parseNlq: (body: { query: string; tenant_id?: string; time_zone?: string }, o?: ApiOpts) =>
    request<{ filter: Record<string, unknown> }>('POST', '/v1/nlq/parse', body, o),

  /* active learning */
  listAlQueue: (o?: ApiOpts) => request<Page<AlSample>>('GET', '/v1/active-learning/queue', undefined, o),
  claimAlSample: (o?: ApiOpts) => request<AlSample>('POST', '/v1/active-learning/sample', {}, o),

  /* semantic search */
  semanticSearch: (body: { query: string; k?: number; filter?: Record<string, unknown> }, o?: ApiOpts) =>
    request<Page<Event>>('POST', '/v1/search', body, o),

  /* cost + metering */
  usage: (params: { from?: string; to?: string }, o?: ApiOpts) => {
    const q = new URLSearchParams(Object.entries(params).filter(([, v]) => v != null).map(([k, v]) => [k, String(v)]));
    return request<Page<UsageRow>>('GET', `/v1/metering/usage?${q}`, undefined, o);
  },
  invoices: (o?: ApiOpts) => request<Page<Invoice>>('GET', '/v1/metering/invoices', undefined, o),
  costByService: (params: { from?: string; to?: string }, o?: ApiOpts) => {
    const q = new URLSearchParams(Object.entries(params).filter(([, v]) => v != null).map(([k, v]) => [k, String(v)]));
    return request<Page<{ service: string; gpu_seconds: number; tokens: number; cost_cents: number }>>('GET', `/v1/cost/by-service?${q}`, undefined, o);
  },

  /* compliance */
  listControls: (o?: ApiOpts) => request<{ items: Control[] }>('GET', '/v1/controls', undefined, o),
  bundleEvidence: (body: { framework: string; controls: string[] }, o?: ApiOpts) =>
    request<EvidenceBundle>('POST', '/v1/evidence/bundle', body, o),

  /* audit */
  listAudit: (params: { since?: string; tenant_id?: string; page_size?: number; page_token?: string }, o?: ApiOpts) => {
    const q = new URLSearchParams(Object.entries(params).filter(([, v]) => v != null).map(([k, v]) => [k, String(v)]));
    return request<Page<AuditRecord>>('GET', `/v1/audit?${q}`, undefined, o);
  },
  verifyAudit: (params: { since?: string }, o?: ApiOpts) => {
    const q = new URLSearchParams(Object.entries(params).filter(([, v]) => v != null).map(([k, v]) => [k, String(v)]));
    return request<{ verified: boolean; records: number; broken_at?: number }>('GET', `/v1/audit/verify?${q}`, undefined, o);
  },

  /* rules */
  listRules: (o?: ApiOpts) => request<Page<{ id: string; name: string; spec: Record<string, unknown> }>>('GET', '/v1/rules', undefined, o),
  createRule: (body: { name: string; spec: Record<string, unknown> }, o?: ApiOpts) =>
    request<{ id: string; name: string; spec: Record<string, unknown> }>('POST', '/v1/rules', body, o),
};

export type Api = typeof api;
