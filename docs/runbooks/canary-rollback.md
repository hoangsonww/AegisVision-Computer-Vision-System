# Runbook: Canary rollback

**Last updated:** 2026-05-21
**Owner:** ml-reliability

This runbook covers the case where `canary-controller` auto-rolled back
a promotion. The orchestrator should already be onto it — but you may
need to investigate or override.

## Detecting

```
# Recently rolled-back plans, any tenant
curl -s http://canary-controller.aegis-system:8460/v1/canary/plans?status=rolled_back | \
  jq '.items[] | {id, candidate_urn, failure_reason, last_step_at}'

# Bus replay of recent rollback events
nats sub 'canary.rollback.v1' --count 20
```

## Triage

The `failure_reason` field is one of:

- `proportion_regression` — candidate's Wilson lower bound is more
  than `max_proportion_delta` below baseline. Look at the most recent
  baseline + candidate observations:
  ```
  curl -s http://canary-controller.aegis-system:8460/v1/canary/plans/{id} | \
    jq '{baseline_urn, candidate_urn, max_proportion_delta}'
  ```
  Then look at the underlying detection-quality metrics in Prometheus
  to confirm the rollback was warranted (vs. e.g. an instrumentation
  bug that under-counted the candidate's successes).
- `latency_regression` — candidate's p95 exceeded the baseline by
  more than `max_latency_p95_ms_delta`. Verify against the Grafana
  panel for the candidate model's inference-router latency.
- `insufficient_samples` should NOT trigger a rollback — if it did,
  the state machine has a bug.

## Common causes

1. **Mis-instrumented success/failure.** The dataplane's
   `inference.baseline.v1` event must use the *same* success
   definition for baseline and candidate. A mismatch (e.g. baseline
   counts low-confidence as success, candidate doesn't) is a common
   source of phantom regressions.
2. **Cold-start latency artifact.** The first 30s after a step
   advance, the candidate is still warming up. The min-sample-size
   floor usually filters this — if not, the floor is set too low for
   the traffic level.
3. **Tenant skew.** A single high-traffic tenant landed entirely on
   the candidate (small percentage step + an unlucky hash). Confirm
   by partitioning the observations by tenant_id.

## Operator actions

- **Restart the canary on the same candidate.** Create a fresh plan
  with the same baseline + candidate URNs. The previous failure is
  preserved in the audit log.
- **Pause + investigate.** If you suspect the regression is real:
  ```
  curl -X POST http://canary-controller.aegis-system:8460/v1/canary/plans/{id}:pause
  ```
- **Manual override promotion** is the gated path — open a gate via
  policy-gate-service. There is intentionally no "force promote"
  endpoint.

## Don't

- **Don't disable auto-rollback** to push a regression through. The
  controller is the safety belt.
- **Don't ignore repeat rollbacks on the same candidate.** Three
  consecutive rollbacks → file an incident; the candidate is not
  ready.
