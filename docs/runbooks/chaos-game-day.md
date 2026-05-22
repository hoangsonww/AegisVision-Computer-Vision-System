# Runbook: Quarterly chaos game-day

**Last updated:** 2026-05-21
**Owner:** reliability + ml-reliability

Per ADR-0028, chaos engineering is a production-readiness gate. This is
the runbook the on-call rotation owns for the quarterly game-day drill.
Cadence: first Tuesday of each quarter, 10:00 PT, 2-hour window.

## Why we do this

A chaos experiment that never runs is documentation. A chaos experiment
that runs once a year, fails, and we then fix the underlying issue is
training. The point is to keep the **fail-mode response** in muscle
memory, not to discover unknown unknowns (those come from incidents).

## Pre-flight (T-1 day)

```sh
# Verify chaos-mesh is healthy in the staging cluster.
kubectl --context staging -n chaos-mesh get pods

# Tail the platform's alertmanager — we expect specific alerts to fire
# during the drill and want to verify they reach the right channels.
kubectl --context staging -n monitoring port-forward svc/alertmanager 9093 &

# Pre-pull the chaos-runner image so the drill isn't blocked on pulls.
kubectl --context staging -n aegis-chaos run prewarm --image=ghcr.io/hoangsonww/aegisvision-chaos-runner:latest --restart=Never -- /bin/true
```

## The drill (T+0)

For each experiment below, in order:

1. Apply the manifest.
2. Watch the corresponding check job.
3. Record PASS / FAIL in the spreadsheet.
4. If FAIL — file an incident, do NOT proceed to the next experiment.
   We want the drill to surface the issue, not paper over multiple
   compounding failures.

| # | Experiment | Expected window | ADR |
| - | ---------- | --------------- | --- |
| 1 | `nats-pod-kill.yaml` | 3 min | ADR-0005 |
| 2 | `clickhouse-rolling-restart.yaml` | 6 min | ADR-0002 |
| 3 | `gpu-oom-reject.yaml` | 2 min | ADR-0003 |
| 4 | `llm-gateway-timeout.yaml` | 8 min | ADR-0017/0018 |
| 5 | `policy-gate-down.yaml` | 4 min | ADR-0014/0017 |
| 6 | `postgres-failover.yaml` | 3 min | ADR-0002 |
| 7 | `kafka-broker-loss.yaml` | 5 min | ADR-0002 |
| 8 | `az-partition.yaml` | 5 min | n/a (HA expectation) |
| 9 | `triton-model-evict.yaml` | 4 min | ADR-0003 |
| 10 | `webhook-receiver-5xx.yaml` | 7 min | n/a (notification reliability) |

```sh
# Apply each, in sequence. Each check job emits PASS/FAIL on completion.
for exp in deploy/chaos/*.yaml; do
  echo "▶ $exp"
  kubectl --context staging apply -f "$exp"
  job="chaos-check-$(basename "$exp" .yaml)"
  kubectl --context staging -n aegis-chaos wait --for=condition=complete --timeout=15m "job/$job"
  kubectl --context staging -n aegis-chaos logs "job/$job" | tail -5
  read -p "Continue? [y/N] " ans
  [ "$ans" = "y" ] || break
done
```

## Post-flight (T+2h)

1. Sign + upload the game-day attestation as an audit record:
   ```sh
   ./tools/dr-drills/run-game-day.sh --upload-attestation
   ```
   The attestation lists each experiment, its PASS/FAIL status, the
   on-caller who ran it, and the cluster context. It lands in
   audit-service under the `chaos.gameday.v1` category, retained for the
   SOC 2 audit (CC7.4).
2. Open a retrospective issue. Even all-PASS runs get a retro — what was
   the slowest recovery, what alert pages were missed, what dashboards
   need work.
3. Schedule the next quarter's drill.

## Common failure modes (and what to do)

| Symptom | Likely cause | Action |
| ------- | ------------ | ------ |
| `nats-pod-kill` reports queue still draining at T+120s | The forwarder's tick is set too long, or core NATS DNS hadn't propagated. | Bump the tick (`AEGIS_FORWARD_TICK`) to 1s on edge-gateway; check core DNS TTL. |
| `policy-gate-down` reports completed status | Agent's tool list still has tier-3 actions that route around the gate. | Audit `services/agent-service/internal/tools/tools.go` for any tool with `RiskTier: llm.RiskTierConsequential` that has a synthetic `Run` rather than refusing. |
| `postgres-failover` budget exceeded (>5 failures) | pgx driver pool wasn't configured for retry on connection drop. | Set `pool_max_conn_lifetime_jitter` and reduce idle conn lifetime; verify Patroni etcd lock TTL. |
| `kafka-broker-loss` did not recover | Consumer group rebalance stuck. | `kafka-consumer-groups.sh --describe` to inspect, and bump session timeout. |
| `triton-model-evict` post latency >> baseline | inference-router's pool only had one replica per model. | Add at least 2 replicas per warmed model. |

## Don't

- **Don't run game-day in production-tenant clusters.** Staging only.
- **Don't disable the check jobs** because they fire too often. Fix the
  underlying behaviour.
- **Don't skip the retrospective** even when every experiment passed. The
  point is the practice, not the score.
