# pkg/intelligence

> **Active-learning + uncertainty + NLQ types.**

Pure types + small helpers. Used by `active-learning-service`,
`nlq-service`, and the agent toolbox.

---

## Contents

| File | Purpose |
| --- | --- |
| `autonomy.go` | Tier classification helpers. |
| `types.go` | Detection, Sample, NLQQuery, Uncertainty score. |

---

## Why a separate package

These types cross service boundaries (active-learning emits, agent
consumes). Keeping them in one place avoids divergent definitions.
