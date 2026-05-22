# tools/integration

> **Cross-service contract smoke test.** Catches producer/consumer subject
> drift in CI before deploy.

A 5-test Go suite that uses only public `pkg/` packages (Go forbids
cross-module `internal/` imports), with an in-process `busGateway`
helper.

| Test | Asserts |
| --- | --- |
| `TestSmoke_BoundedAutonomy_GateRoundTrip` | A tier-3 tool routed through `policy-gate-service` resumes the agent on `gate.resolved.<id>`. |
| `TestSmoke_PlatformBusSubjects` | All 17 well-known bus subjects have a producer **and** a consumer. |
| `TestSmoke_PlatformWildcardSubjects` | All 7 wildcard subject pairs match in producer + consumer. |
| `TestSmoke_ConcurrentGateResolutions` | 32 parallel approvals resolve correctly under concurrency. |
| `TestSmoke_LLMSafety_RefusesInjection` | The safety layer refuses prompt-injection payloads. |

---

## Run

```bash
(cd tools/integration && go test -race -count=1 ./...)
# → 5 tests pass
```

---

## Why this exists

During the system-integration audit, three production bugs were found
by writing these tests:

1. `inference.completed.v1` had no publisher — metering would have
   billed nothing.
2. `inference.baseline.v1` had no publisher — shadow inference would
   never compare.
3. `gate.resolved.<id>` had no subscriber — auto-resume was vaporware.

The tests now catch this class of bug before deploy.
