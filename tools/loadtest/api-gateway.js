// k6 load test for the api-gateway with SLO gates.
//
// Gates (per architecture doc §5.1 — control-plane targets):
//   - p95 < 200 ms on read endpoints
//   - p99 < 500 ms on read endpoints
//   - error rate < 1%
//
// Run locally with:
//   k6 run tools/loadtest/api-gateway.js
// Or against a deployed gateway:
//   BASE_URL=https://api.example k6 run tools/loadtest/api-gateway.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost:8080';
const TENANT = __ENV.TENANT || 't-loadtest';
const TOKEN = __ENV.BEARER || '';

const errors = new Rate('aegis_errors');

export const options = {
  scenarios: {
    list_pipelines: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '15s', target: 25 },
        { duration: '30s', target: 25 },
        { duration: '10s', target: 0 },
      ],
      gracefulRampDown: '5s',
      exec: 'listPipelines',
    },
    list_streams: {
      executor: 'constant-vus',
      vus: 10,
      duration: '45s',
      exec: 'listStreams',
      startTime: '5s',
    },
  },
  thresholds: {
    // SLO gates — k6 fails the run if any are breached.
    'http_req_duration{endpoint:pipelines}': ['p(95)<200', 'p(99)<500'],
    'http_req_duration{endpoint:streams}':   ['p(95)<200', 'p(99)<500'],
    'aegis_errors': ['rate<0.01'],
  },
};

function headers() {
  const h = { 'X-Tenant-Id': TENANT };
  if (TOKEN) h['Authorization'] = `Bearer ${TOKEN}`;
  return h;
}

export function listPipelines() {
  const res = http.get(`${BASE}/v1/pipelines?page_size=20`, {
    headers: headers(),
    tags: { endpoint: 'pipelines' },
  });
  const ok = check(res, { 'status 200/401': (r) => r.status === 200 || r.status === 401 });
  errors.add(!ok);
  sleep(0.2);
}

export function listStreams() {
  const res = http.get(`${BASE}/v1/streams?page_size=20`, {
    headers: headers(),
    tags: { endpoint: 'streams' },
  });
  const ok = check(res, { 'status 200/401': (r) => r.status === 200 || r.status === 401 });
  errors.add(!ok);
  sleep(0.25);
}
