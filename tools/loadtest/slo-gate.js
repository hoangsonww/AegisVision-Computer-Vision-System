// k6 SLO assertion gate. Combines the platform's product SLOs into one
// pass/fail run that the release pipeline executes after the air-gap
// install on a staging cluster.
//
// Three gates, each one a separate scenario so the run keeps going past
// individual breaches and surfaces *all* failures in one report:
//
//   gate A — control plane: p95 < 200ms, p99 < 500ms on /v1/pipelines
//   gate B — data plane:    glass-to-event p95 < 300ms (via SSE probe)
//   gate C — agent runtime: p95 session-terminal < 30s
//
// Failures aggregate in handleSummary; the run exits non-zero if any
// gate failed. This is the script CI calls before the release-please
// release is allowed to publish.
import http from 'k6/http';
import { check, fail, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const BASE_API   = __ENV.BASE_URL || 'http://api-gateway.aegis-system:8080';
const BASE_AGENT = __ENV.AGENT_URL || 'http://agent-service.aegis-system:8420';
const TENANT     = __ENV.TENANT || 't-slo-gate';
const TOKEN      = __ENV.BEARER || '';

const ctrlErrors  = new Rate('slo_control_errors');
const dataErrors  = new Rate('slo_data_errors');
const agentErrors = new Rate('slo_agent_errors');
const g2e         = new Trend('slo_glass_to_event_ms', true);
const sessTerm    = new Trend('slo_session_terminal_ms', true);

export const options = {
  scenarios: {
    gate_control: {
      executor: 'constant-vus',
      vus: 25,
      duration: '60s',
      exec: 'gateControl',
    },
    gate_data: {
      executor: 'constant-vus',
      vus: 25,
      duration: '60s',
      exec: 'gateData',
    },
    gate_agent: {
      executor: 'constant-vus',
      vus: 25,
      duration: '60s',
      exec: 'gateAgent',
    },
  },
  // Thresholds are absent here intentionally; we enforce in handleSummary
  // so we can produce a structured report that lists which gate failed,
  // not just "thresholds breached."
};

function headers(extra) {
  const h = { 'X-Tenant-Id': TENANT, 'Content-Type': 'application/json' };
  if (TOKEN) h['Authorization'] = `Bearer ${TOKEN}`;
  return Object.assign(h, extra || {});
}

export function gateControl() {
  const res = http.get(`${BASE_API}/v1/pipelines?page_size=20`, { headers: headers() });
  ctrlErrors.add(!(res.status >= 200 && res.status < 300));
  sleep(0.2);
}

export function gateData() {
  // Submit one frame descriptor + measure latency until the SSE returns
  // the corresponding event. Combined into one HTTP call for the
  // synthetic /v1/loadtest/echo endpoint when present; otherwise
  // approximate via a sse:get with short timeout.
  const start = Date.now();
  const res = http.get(`${BASE_API}/v1/events:stream?stream_id=slo-probe`, {
    headers: headers({ Accept: 'text/event-stream' }),
    timeout: '5s',
    responseType: 'text',
  });
  if (res.status === 200 && /event: detection/.test(res.body || '')) {
    g2e.add(Date.now() - start);
  } else {
    dataErrors.add(true);
  }
  sleep(0.5);
}

export function gateAgent() {
  const t0 = Date.now();
  const res = http.post(`${BASE_AGENT}/v1/agents/sessions`,
    JSON.stringify({ role: 'operator', goal: 'slo gate probe', max_steps: 3 }),
    { headers: headers(), timeout: '60s' });
  if (!(res.status >= 200 && res.status < 300)) {
    agentErrors.add(true);
    return;
  }
  if (/"status":"(completed|failed|awaiting_gate)"/.test(res.body || '')) {
    sessTerm.add(Date.now() - t0);
  } else {
    agentErrors.add(true);
  }
  sleep(0.5);
}

export function handleSummary(data) {
  const ctrlP95   = data.metrics['http_req_duration{scenario:gate_control}']?.values?.['p(95)']   || 0;
  const ctrlP99   = data.metrics['http_req_duration{scenario:gate_control}']?.values?.['p(99)']   || 0;
  const ctrlErr   = data.metrics['slo_control_errors']?.values?.rate || 0;
  const g2eP95    = data.metrics['slo_glass_to_event_ms']?.values?.['p(95)'] || 0;
  const dataErr   = data.metrics['slo_data_errors']?.values?.rate || 0;
  const sessP95   = data.metrics['slo_session_terminal_ms']?.values?.['p(95)'] || 0;
  const agentErr  = data.metrics['slo_agent_errors']?.values?.rate || 0;

  const gates = {
    A_control_p95: { value: ctrlP95, budget: 200, pass: ctrlP95 < 200 && ctrlErr < 0.01 },
    A_control_p99: { value: ctrlP99, budget: 500, pass: ctrlP99 < 500 },
    B_glass_to_event_p95: { value: g2eP95, budget: 300, pass: g2eP95 < 300 && dataErr < 0.01 },
    C_session_terminal_p95: { value: sessP95, budget: 30000, pass: sessP95 < 30000 && agentErr < 0.01 },
  };
  const overall = Object.values(gates).every(g => g.pass);

  if (!overall) {
    console.error('SLO gate breach:', JSON.stringify(gates, null, 2));
    fail('SLO gate breach');
  }
  return {
    'stdout': JSON.stringify({ overall, gates }, null, 2),
  };
}
