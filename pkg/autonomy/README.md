# pkg/autonomy

> **Cron + signal scheduler + divergence math.** ADR-0022, ADR-0025.

Used by `autonomy-orchestrator` and `drift-detection-service`.

---

## Contents

| File | Purpose |
| --- | --- |
| `cron.go` | Cron expression parser + ticker. |
| `scheduler.go` | Schedule registry, signal-driven triggers, deadline enforcement. |
| `divergence.go` | JS / KL / TVD divergence math (vectorised). |

---

## Divergence

```go
js  := autonomy.JensenShannon(p, q)
kl  := autonomy.KLDivergence(p, q)
tvd := autonomy.TotalVariation(p, q)
```

All three return `[0, 1]`-bounded distances (TVD always; KL/JS we
normalise). Used to compare current class distributions to a
reference (drift-detection-service).

---

## See also

- ADR-0022, ADR-0025.
- [`services/drift-detection-service/README.md`](../../services/drift-detection-service/README.md).
- [`services/autonomy-orchestrator/README.md`](../../services/autonomy-orchestrator/README.md).
