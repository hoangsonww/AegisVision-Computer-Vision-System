# Runbook: Drift spike

**Last updated:** 2026-05-21
**Owner:** ml-monitoring

A `critical` drift signal (`autonomy.signal.drift.v1`) fired for a
(tenant, model). The monitor autonomy agent will *propose* a mitigation,
but consequential mitigations (model swap, retention override, etc.)
require human approval through `policy-gate-service`.

## Detecting

```
# Recent critical signals
curl -s 'http://drift-detection-service.aegis-system:8480/v1/drift/signals?since=2026-05-20T00:00:00Z' \
  -H 'X-Aegis-Tenant: <tenant>' | jq '.items[] | select(.severity=="critical")'
```

## Triage

1. **Is the reference distribution stale?** A reference captured
   12 months ago will false-fire after every seasonal change. Check
   the reference's `captured_at` and re-capture if it predates the
   current model promotion:
   ```
   # Capture the current production distribution as the new reference
   # (operator-controlled — the new reference becomes the comparison
   # baseline for future drift evaluations).
   curl -X POST http://drift-detection-service.aegis-system:8480/v1/drift/references \
     -H 'X-Aegis-Tenant: <tenant>' -H 'Content-Type: application/json' \
     -d '{"model_id":"...", "distribution":{"classes":{...},"samples":NNN}}'
   ```
2. **Is one class dominant?** A new dominant class (say, the tenant
   started getting deliveries of a vehicle type the model wasn't
   trained on) shows up as a high-mass shift on TVD. Confirm by
   inspecting the signal's `observed` field.
3. **Is the model still accurate?** Drift predicts a fall in
   accuracy; it doesn't directly observe it. Check inference-quality
   metrics (and the active-learning loop's labeled samples) to see
   whether accuracy actually degraded.

## Common mitigations

| Drift type | Mitigation |
| ---------- | ---------- |
| Seasonal shift | Increase `warn_threshold`, keep `critical_threshold`; re-capture reference at month boundary. |
| Genuine new class | Trigger a re-training run via training-orchestrator; expand label vocabulary. |
| Instrumentation bug | Fix upstream and dismiss the signal. Don't change thresholds to "make it go away." |
| Tenant configuration change | Communicate with tenant; their pipeline may be sending wildly different streams. |

## Operator actions

- **Open an autonomy run.** Manually trigger the monitor schedule:
  ```
  curl -X POST http://autonomy-orchestrator.aegis-system:8510/v1/autonomy/schedules/{id}:trigger
  ```
  The agent will read the drift signal, query the knowledge corpus,
  and propose actions. Consequential actions appear in the gate queue.
- **Acknowledge the signal.** There is intentionally no
  "acknowledge" endpoint — signals are forensic records. If you want
  to suppress further alerts for a known issue, raise the threshold
  on the policy temporarily and re-evaluate at the next sprint.

## Don't

- **Don't raise thresholds permanently** to silence a signal you
  don't understand. The signal is doing its job.
- **Don't promote a new model in response to drift without
  validation.** The drift signal predicts a fall in accuracy; it
  doesn't prove the new model is better. Run the canary (ADR-0023)
  with shadow inference (ADR-0024) first.
