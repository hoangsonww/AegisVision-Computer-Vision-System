# On-call runbook — AegisVision

You're paged. Where to start, in priority order.

## 1. Triage (first 90s)

1. Open the **AegisVision / Platform Overview** Grafana dashboard.
   - p95 glass-to-event > 300ms? → §3 Latency
   - error rate > 1% on api-gateway? → §4 API errors
   - `aegis_gpu_free_vram_bytes` < 1 GiB across all inference GPUs? → §5 GPU pressure
2. Check the pager source. If it's a synthetic check, jump to §6 Synthetic.
3. If multiple alerts fired in <30s, treat as an incident and start a war room.

## 2. The contact tree

| Role | Primary | Escalation |
| --- | --- | --- |
| Platform on-call | rotating PagerDuty | Son Nguyen — hoangson091104@gmail.com |
| Streaming on-call | rotating PagerDuty | Son Nguyen — hoangson091104@gmail.com |
| Security on-call | rotating PagerDuty | Son Nguyen — hoangson091104@gmail.com |

> The author is the escalation contact for every role.
> See [`GOVERNANCE.md`](../../GOVERNANCE.md).

## 3. Latency p95 > 300ms

1. Pull up the **glass-to-event** Grafana panel; identify which leg is slow.
2. If `aegis_dataplane_operator_seconds{operator="detect"}` is the culprit:
   - Check `aegis_gpu_free_vram_bytes` and the GPU scheduler `/v1/state`.
   - If a model was promoted in the last hour, roll back the `inference-router`
     deployment (Argo: `argocd app rollback inference-router`).
3. If `aegis_request_duration_seconds{service="api-gateway"}` is the culprit:
   - Check `pipeline-service`/`stream-manager` p95 — gRPC tail latency
     usually means database saturation. `kubectl exec` into the
     postgres pod and run `pg_stat_activity`.
4. If NATS lag — check `aegis_realtime_dropped_total`. Slow consumers can
   produce false alarms; scale `realtime-hub` and re-check.

## 4. API errors > 1%

1. `kubectl logs -n aegis-system -l app.kubernetes.io/name=api-gateway --tail 200`
2. Look for `authz` denials — a misconfigured OPA policy can deny everything.
3. If Postgres errors: failover Postgres (Patroni: `patronictl failover`).
4. If upstream-unavailable: bounce the unhealthy upstream's deployment.

## 5. GPU pressure

1. `curl http://gpu-scheduler.aegis-system.svc:8200/v1/state | jq` —
   look for GPUs with `freeVRAM < 1GiB` and recent reservations.
2. If a model is consuming much more VRAM than its declared CostProfile:
   a) ban it from new placements (`POST /v1/release` for every reservation),
   b) re-profile the model (ADR-0003) — the model's declared CostProfile
      is wrong and needs correction before re-promotion.
3. The reservation ledger is canonical — `nvidia-smi free memory` is NOT
   what the scheduler uses, and it will be misleading. Don't trust it.

## 6. Synthetic check failed

1. `curl -i https://api.aegisvision.example/v1/pipelines -H "X-Tenant-Id: synthetic"`
2. If 503: check ingress + istiod logs.
3. If 401: rotate the synthetic JWT (`kubectl exec` into the synthetic test
   client and `aegis-cli token --tenant synthetic`).

## 7. Data plane stopped

1. Symptom: `aegis_dataplane_frames_processed_total` is flat.
2. `kubectl logs -n aegis-streaming -l app.kubernetes.io/name=dataplane-runner --tail 200`.
3. Common causes: NATS unreachable; ring claim-check exhausted; ingest source
   unreachable. Each surfaces a distinct log line.
4. Last resort: `kubectl rollout restart -n aegis-streaming deploy/dataplane-runner`.

## 8. Compliance event

If the alert is `audit_records_anomaly`:
- Do NOT delete anything in the audit-service database. The audit log is
  evidence (ADR-0014).
- Page the project author (hoangson091104@gmail.com) immediately.
- Snapshot the audit-service Postgres before any further action.
