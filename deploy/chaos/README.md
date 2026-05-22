# Chaos experiments

Per ADR-0028, chaos engineering is a production-readiness gate. Every
load-bearing failure mode in the architecture has a corresponding chaos
manifest here that:

1. Reproduces the failure mode deterministically against a kind/k3s cluster
   or staging environment.
2. Asserts the system's expected behaviour via a check job that runs
   alongside the experiment.
3. Emits PASS/FAIL into the game-day record (`docs/runbooks/chaos-game-day.md`).

We use [chaos-mesh](https://chaos-mesh.org/) as the engine. Experiments
target specific ADR commitments:

| Experiment | ADR | Asserts |
| --- | --- | --- |
| `nats-pod-kill.yaml` | ADR-0005 | edge-gateway's disk queue absorbs the outage; no lost events |
| `clickhouse-rolling-restart.yaml` | ADR-0002 | event-service write path keeps draining; no detection loss |
| `gpu-oom-reject.yaml` | ADR-0003 | gpu-scheduler refuses placement gracefully (returns 503), never crashes |
| `llm-gateway-timeout.yaml` | ADR-0017/0018 | agent loop bounded; no token-burn runaway |
| `policy-gate-down.yaml` | ADR-0014/0017 | consequential tool calls fail closed (never auto-execute) |
| `postgres-failover.yaml` | ADR-0002 (HA) | Patroni promotes the new primary < 30s; in-flight writes retried |
| `kafka-broker-loss.yaml` | ADR-0002 | metering consumer reconnects; counter convergence within 60s |
| `az-partition.yaml` | none (HA expectation) | cross-AZ replicas don't split-brain; one side becomes read-only |
| `triton-model-evict.yaml` | ADR-0003 | inference-router falls back to the in-pool replica; latency spike < 5s |
| `webhook-receiver-5xx.yaml` | none (notification reliability) | notification-service retries with backoff, lands in DLQ after N attempts |

## Running

```sh
# Pre-req: chaos-mesh installed in `chaos-mesh` namespace; cluster has
# the target service running.
kubectl apply -f deploy/chaos/nats-pod-kill.yaml
# Wait the experiment's duration; check the result job:
kubectl -n aegis-chaos logs job/chaos-check-nats-pod-kill -f
```

The `tools/dr-drills/run-game-day.sh` script orchestrates the full set on a
quarterly schedule, collects each PASS/FAIL, and writes the game-day
attestation as a signed audit record.

## Don't

- **Don't run these in production-tenant clusters.** They are staging /
  pre-prod only.
- **Don't disable experiments** because they fire too often. The whole
  point is to surface the failure mode. If an experiment becomes flaky,
  fix the underlying behaviour or tune the threshold — never silently
  disable.
- **Don't add an experiment without a check job.** A chaos experiment
  without an assertion is just an outage.
