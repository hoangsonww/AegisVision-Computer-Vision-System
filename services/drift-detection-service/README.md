# drift-detection-service

> **Detects distribution drift in model outputs.** JS / KL / TVD
> divergence vs reference. ADR-0025.

`drift-detection-service` consumes `inference.completed.v1`, maintains
sliding-window class-distribution histograms per (tenant, stream,
model), and compares against a reference distribution stored in
`model-registry`. When divergence exceeds the threshold, an alert is
emitted via `slo-watchdog`.

---

## Why three divergences

Different shapes, different sensitivities:

- **JS** (Jensen-Shannon) — bounded, symmetric, smooth. Good default.
- **KL** (Kullback-Leibler) — asymmetric, sensitive to tail mass shifts.
- **TVD** (Total Variation Distance) — bounded, interpretable as
  probability mass moved. Easy to explain to operators.

Each is computed; alerts fire on configurable thresholds per metric.
Implementation in `pkg/autonomy/divergence.go`.

---

## Sliding window

Default 1h window with 5-minute step. Reference distribution comes
from the model's training-time empirical distribution, stored in
`model-registry` alongside the artifact.

---

## API

- `GET /v1/drift/runs` — list runs.
- `GET /v1/drift/runs/{id}` — inspect.
- `POST /v1/drift/runs` — manual trigger.

---

## Metrics

- `aegis_drift_js_divergence{tenant,model}` — gauge.
- `aegis_drift_kl_divergence{tenant,model}` — gauge.
- `aegis_drift_tvd_divergence{tenant,model}` — gauge.
- `aegis_drift_alerts_total{tenant,model,metric}` — alerts.

---

## See also

- [`../slo-watchdog/README.md`](../slo-watchdog/README.md) — alert sink.
- [`../model-registry/README.md`](../model-registry/README.md) — reference distributions.
- [`../../pkg/autonomy/README.md`](../../pkg/autonomy/README.md) — math.
- ADR-0025.
