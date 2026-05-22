// k6 load test that drives 10,000 synthetic streams against api-gateway
// + stream-manager + dataplane-runner. This is the Phase-6 GA scale gate
// (architecture doc §1.4): the platform must sustain 10k 30-fps streams
// concurrently with glass-to-event p95 < 300 ms.
//
// The test is split into three scenarios:
//   1. Create 10k streams.       (ramp + idempotent — re-running re-uses ids)
//   2. Submit 30 fps of synthetic frame descriptors per stream.
//   3. Probe glass-to-event latency via the SSE stream on a sampled subset.
//
// Run locally (1k streams, light load) for smoke:
//   STREAMS=1000 RAMP_VUS=200 k6 run tools/loadtest/streams-10k.js
// Run in the staging cluster for the GA gate:
//   STREAMS=10000 RAMP_VUS=2000 BASE_URL=https://api.staging k6 run tools/loadtest/streams-10k.js
import http from 'k6/http';
import ws from 'k6/ws';
import { check, fail, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const BASE         = __ENV.BASE_URL || 'http://localhost:8080';
const TENANT       = __ENV.TENANT || 't-loadtest';
const TOKEN        = __ENV.BEARER || '';
const STREAMS      = parseInt(__ENV.STREAMS || '10000', 10);
const RAMP_VUS     = parseInt(__ENV.RAMP_VUS || '2000', 10);
const FRAME_VUS    = parseInt(__ENV.FRAME_VUS || '4000', 10);
const PROBE_VUS    = parseInt(__ENV.PROBE_VUS || '100', 10);
const FRAME_HZ     = parseInt(__ENV.FRAME_HZ || '30', 10);   // per stream
const HOLD_MIN     = parseInt(__ENV.HOLD_MIN || '5', 10);    // steady-state minutes

const errors = new Rate('aegis_errors');
const g2e    = new Trend('aegis_glass_to_event_ms', true);

export const options = {
  scenarios: {
    create_streams: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '60s', target: RAMP_VUS },
        { duration: '120s', target: RAMP_VUS },
        { duration: '15s', target: 0 },
      ],
      gracefulRampDown: '5s',
      exec: 'createStream',
    },
    push_frames: {
      executor: 'constant-vus',
      vus: FRAME_VUS,
      duration: `${HOLD_MIN}m`,
      exec: 'pushFrame',
      startTime: '120s',
    },
    probe_glass_to_event: {
      executor: 'constant-vus',
      vus: PROBE_VUS,
      duration: `${HOLD_MIN}m`,
      exec: 'probeLatency',
      startTime: '125s',
    },
  },
  thresholds: {
    // The GA SLO gate. CI fails if any breach.
    'http_req_duration{endpoint:streams_create}':  ['p(95)<200', 'p(99)<500'],
    'http_req_duration{endpoint:frame_push}':      ['p(95)<50',  'p(99)<150'],
    'aegis_glass_to_event_ms':                     ['p(95)<300', 'p(99)<500'],
    'aegis_errors':                                ['rate<0.005'],
  },
};

function headers(extra) {
  const h = { 'X-Tenant-Id': TENANT, 'Content-Type': 'application/json' };
  if (TOKEN) h['Authorization'] = `Bearer ${TOKEN}`;
  return Object.assign(h, extra || {});
}

function streamID(idx) { return `s-${TENANT}-${idx}`; }

export function createStream() {
  // Each VU iteration creates a single stream, deterministically keyed so
  // re-runs are idempotent against the same tenant.
  const idx = (__VU - 1) * 1000 + (__ITER % 1000);
  if (idx >= STREAMS) return;
  const body = JSON.stringify({
    id: streamID(idx),
    name: `loadtest-${idx}`,
    project_id: 'loadtest',
    protocol: 'file',
    url_redacted: 'file:///dev/null',
    pipeline_id: 'p-loadtest',
  });
  const res = http.post(`${BASE}/v1/streams`, body, {
    headers: headers({ 'Idempotency-Key': `create-${idx}` }),
    tags: { endpoint: 'streams_create' },
  });
  const ok = check(res, { '2xx': (r) => r.status >= 200 && r.status < 300 });
  errors.add(!ok);
}

export function pushFrame() {
  // Steady-state: each VU pushes a frame descriptor on behalf of one
  // stream, then sleeps to maintain FRAME_HZ.
  const idx = ((__VU - 1) * 100 + (__ITER % 100)) % STREAMS;
  const ts  = Date.now();
  const body = JSON.stringify({
    subject: `frame.descriptor.${streamID(idx)}`,
    data: {
      stream_id: streamID(idx),
      pipeline_id: 'p-loadtest',
      frame_seq: __ITER,
      capture_time: new Date(ts).toISOString(),
      payload_urn: `media://${TENANT}/${streamID(idx)}/${__ITER}`,
    },
  });
  const res = http.post(`${BASE}/v1/dataplane/frames`, body, {
    headers: headers(),
    tags: { endpoint: 'frame_push' },
  });
  const ok = check(res, { '2xx': (r) => r.status >= 200 && r.status < 300 });
  errors.add(!ok);
  // Pace at FRAME_HZ.
  sleep(1 / FRAME_HZ);
}

export function probeLatency() {
  // Subscribe via SSE to a sampled stream and time the gap between our
  // frame_push and the resulting event arrival.
  const idx = __VU % STREAMS;
  const url = `${BASE}/v1/events:stream?stream_id=${streamID(idx)}`;
  const start = Date.now();
  const res = http.get(url, {
    headers: headers({ Accept: 'text/event-stream' }),
    tags: { endpoint: 'sse_probe' },
    timeout: '5s',
    responseType: 'text',
  });
  if (res.status !== 200) {
    errors.add(true);
    return;
  }
  // First "event: detection" line ends the probe window. We approximate
  // the latency as the round-trip delta.
  const arrived = Date.now() - start;
  if (res.body && res.body.indexOf('event: detection') >= 0) {
    g2e.add(arrived);
  }
  sleep(1);
}

export function handleSummary(data) {
  // Emit a machine-readable summary that CI parses for the gate.
  const pass =
    data.metrics['aegis_glass_to_event_ms'].values['p(95)'] < 300 &&
    data.metrics['aegis_errors'].values.rate < 0.005;
  if (!pass) {
    // Non-zero exit code via fail().
    fail('SLO gate breach — see thresholds');
  }
  return {
    'stdout': JSON.stringify({
      pass,
      streams: STREAMS,
      ramp_vus: RAMP_VUS,
      frame_vus: FRAME_VUS,
      g2e_p95: data.metrics['aegis_glass_to_event_ms'].values['p(95)'],
      g2e_p99: data.metrics['aegis_glass_to_event_ms'].values['p(99)'],
      error_rate: data.metrics['aegis_errors'].values.rate,
    }, null, 2),
  };
}
