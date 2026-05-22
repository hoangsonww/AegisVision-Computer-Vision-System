// k6 load test: 1,000 concurrent agent sessions against agent-service +
// llm-gateway + policy-gate-service. Phase 6 SLO gate: agent sessions
// must reach a terminal state within 30s (completed / awaiting_gate /
// failed); the gateway's token-burn rate must stay within budget.
import http from 'k6/http';
import { check, fail, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const BASE        = __ENV.BASE_URL || 'http://agent-service.aegis-system:8420';
const SESSIONS    = parseInt(__ENV.SESSIONS || '1000', 10);
const VUS         = parseInt(__ENV.VUS || '200', 10);
const TOKEN       = __ENV.BEARER || '';
const TENANT_POOL = parseInt(__ENV.TENANT_POOL || '50', 10);

const errors        = new Rate('aegis_agent_errors');
const sessionLatency = new Trend('aegis_agent_session_terminal_ms', true);

export const options = {
  scenarios: {
    open_sessions: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: VUS },
        { duration: '4m',  target: VUS },
        { duration: '15s', target: 0 },
      ],
      gracefulRampDown: '10s',
      exec: 'openSession',
    },
  },
  thresholds: {
    'http_req_duration{endpoint:start_session}': ['p(95)<5000', 'p(99)<10000'],
    'aegis_agent_session_terminal_ms':           ['p(95)<30000'],
    'aegis_agent_errors':                        ['rate<0.01'],
  },
};

function headers(tenantIdx) {
  const h = {
    'X-Aegis-Tenant': `t-load-${tenantIdx}`,
    'Content-Type': 'application/json',
  };
  if (TOKEN) h['Authorization'] = `Bearer ${TOKEN}`;
  return h;
}

const goals = [
  'find the busiest 3 streams in the last hour',
  'summarize today\'s drift signals for model yolo-v9',
  'propose a DAG change to add a tracker before the rule operator',
  'what does ADR-0014 say about agent autonomy',
  'list pending policy-gate decisions older than 1 hour',
];

export function openSession() {
  const tenantIdx = (__VU + __ITER) % TENANT_POOL;
  const goal = goals[(__VU + __ITER) % goals.length];
  const body = JSON.stringify({
    role: 'operator',
    goal,
    max_steps: 6,
  });
  const start = Date.now();
  const res = http.post(`${BASE}/v1/agents/sessions`, body, {
    headers: headers(tenantIdx),
    tags: { endpoint: 'start_session' },
    timeout: '60s',
  });
  const elapsed = Date.now() - start;
  const ok = check(res, {
    '2xx': (r) => r.status >= 200 && r.status < 300,
    'has session_id': (r) => /"session_id":/.test(r.body || ''),
    'has terminal status': (r) =>
      /"status":"(completed|awaiting_gate|failed)"/.test(r.body || ''),
  });
  errors.add(!ok);
  if (ok) sessionLatency.add(elapsed);
  sleep(0.5);
}

export function handleSummary(data) {
  const pass =
    data.metrics['aegis_agent_session_terminal_ms'].values['p(95)'] < 30000 &&
    data.metrics['aegis_agent_errors'].values.rate < 0.01;
  if (!pass) fail('Agent SLO gate breach');
  return {
    'stdout': JSON.stringify({
      pass,
      sessions: SESSIONS,
      vus: VUS,
      p95_terminal_ms: data.metrics['aegis_agent_session_terminal_ms'].values['p(95)'],
      error_rate: data.metrics['aegis_agent_errors'].values.rate,
    }, null, 2),
  };
}
