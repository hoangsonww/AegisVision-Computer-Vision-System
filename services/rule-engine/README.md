# rule-engine

> **Rule evaluation library + service.** Dwell, count, line-cross,
> zone-enter.

`rule-engine` is callable two ways:

1. **In-process** by `dataplane-runner` via `pkg/dataplane/operators/rule`.
2. **Out-of-process** by `event-service` for re-evaluation against
   stored events.

Either path uses the same predicate definitions.

---

## Predicates

| Predicate | Parameters |
| --- | --- |
| `dwell` | class, zone_polygon, min_duration_ms |
| `count` | class, zone_polygon, window_ms, threshold |
| `line_cross` | class, line_segment, direction |
| `zone_enter` | class, zone_polygon |
| `zone_exit` | class, zone_polygon |

Predicates are composed with boolean operators (AND, OR, NOT).

---

## API

```
POST /v1/rules               Create a rule.
GET  /v1/rules/{id}          Read.
POST /v1/rules:evaluate      Evaluate against a set of events (for
                             testing or replaying historical data).
```

---

## See also

- [`../dataplane-runner/README.md`](../dataplane-runner/README.md).
- [`../../pkg/dataplane/README.md`](../../pkg/dataplane/README.md).
