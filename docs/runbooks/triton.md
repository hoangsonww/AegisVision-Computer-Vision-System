# Runbook: NVIDIA Triton Inference Server

**Owner:** infra-gpu / ml-reliability

This runbook covers the operational failure modes of the NVIDIA Triton
Inference Server pool that powers the AegisVision GPU hot path. For the
architecture and configuration reference, see
[`docs/triton.md`](../triton.md).

---

## Quick reference

| Component | Where |
| --- | --- |
| Helm chart | [`deploy/helm/triton/`](../../deploy/helm/triton) |
| Architecture | [`docs/triton.md`](../triton.md) |
| Chaos experiment | [`deploy/chaos/triton-model-evict.yaml`](../../deploy/chaos/triton-model-evict.yaml) |
| Service in cluster | `triton.aegis-inference.svc.cluster.local` |
| HTTP / gRPC / metrics ports | `8000` / `8001` / `8002` |

Common commands:

```bash
# Pod state
kubectl -n aegis-inference get pods -l app.kubernetes.io/name=triton

# Per-model state
kubectl -n aegis-inference port-forward svc/triton 8000:8000
curl localhost:8000/v2/repository/index
curl localhost:8000/v2/models/<name>/stats

# Live metrics
curl localhost:8002/metrics | grep nv_inference_queue_duration_us

# Logs
kubectl -n aegis-inference logs -l app.kubernetes.io/name=triton --tail=200
```

---

## Failure mode 1 — `nv_inference_request_failure` > 0

**Page severity:** P1.

### Triage

1. Identify the failing model + version:
   ```bash
   curl -s localhost:8002/metrics | grep nv_inference_request_failure
   ```
2. Pull the recent error logs:
   ```bash
   kubectl -n aegis-inference logs -l app.kubernetes.io/name=triton \
     --tail=500 | grep -E 'ERROR|failed|exception'
   ```
3. Check the model's per-stat endpoint:
   ```bash
   curl localhost:8000/v2/models/<name>/stats | jq .
   ```

### Common causes

- **TensorRT engine vs GPU SKU mismatch** — `CUDNN_STATUS_VERSION_MISMATCH`,
  `Internal error: engine plan file is not compatible`. Re-compile the
  engine with `trtexec` on the target SKU. The air-gap bundle includes
  the source ONNX/PyTorch artifact and a build script — use that.
- **Out-of-memory on the MIG slice** — `out of memory` in Triton logs;
  pod restarts. Right-size the MIG profile in `values.yaml` (move from
  `1g.10gb` → `2g.20gb`) or reduce `instance_group.count` in the
  model `config.pbtxt`.
- **Strict-config rejection** — `Internal: invalid configuration for
  model <name>`. Fix the `config.pbtxt` and re-upload.
- **Backend missing** — `failed to load backend`. The backend was
  trimmed from `values.yaml triton.backends`. Re-add it.

### Mitigation

- Unload the offending model to stop the failure stream:
  ```bash
  curl -X POST localhost:8000/v2/repository/models/<name>/unload
  ```
- File the fix and re-load via `model-registry` once corrected.

---

## Failure mode 2 — Queue duration spike

**Symptom:** `nv_inference_queue_duration_us` p95 climbs above the SLO
target (default 5 ms). The KEDA HPA may already be scaling out.

### Triage

```bash
# Per-model breakdown
curl -s localhost:8002/metrics | grep nv_inference_queue_duration_us

# Saturation indicator
curl -s localhost:8002/metrics | grep nv_inference_pending_request_count

# Pod count
kubectl -n aegis-inference get pods -l app.kubernetes.io/name=triton
```

### Common causes

- **Burst traffic exceeds HPA scale-out lag.** Check the HPA events:
  ```bash
  kubectl -n aegis-inference describe scaledobject triton
  ```
- **Dynamic batcher misconfigured.** If `preferred_batch_size` is set
  too low for the actual concurrent load, the batcher fragments
  requests. Tune `dynamic_batching.preferred_batch_size` upward.
- **A noisy tenant.** Per-tenant rate-limit at the `inference-router`;
  if a tenant is repeatedly tripping the inner backstop they need a
  larger per-tenant quota or their own pool.
- **GPU saturated.** `nv_gpu_utilization` > 0.95 sustained means we're
  out of compute; scale out or move the model to a larger MIG slice.

### Mitigation

1. Raise `autoscaling.maxReplicas` if the HPA is pinned at the ceiling.
2. Tune the dynamic batcher (per-model `config.pbtxt`).
3. If a single tenant is responsible, isolate them to their own pool
   via `pipeline-service`.
4. Worst case: temporarily reduce `prefetch-service` warm-ups to free
   capacity.

---

## Failure mode 3 — Model load storm

**Symptom:** Many models in `LOADING` state at once; pod CPU spikes;
some loads fail with `Timeout`.

### Triage

```bash
curl -s localhost:8000/v2/repository/index | jq '.[] | select(.state=="LOADING")'
```

### Causes

This is supposed to be impossible by construction:
`--model-control-mode=explicit` plus
`model-registry`'s own load-rate limiter plus per-pod
`modelLoadThreadCount` should prevent it.

If you see this in production:

- Check the `modelLoadThreadCount` value in the chart — should be 4.
- Check `model-registry` logs for evidence of an unbounded retry loop.
- Confirm Triton is not in `poll` mode (verify `--model-control-mode=`
  in `kubectl describe pod`).

### Mitigation

- Pause `model-registry` reconciles temporarily:
  ```bash
  kubectl -n aegis-system scale deploy/model-registry --replicas=0
  ```
- Wait for the in-flight loads to drain (`/v2/repository/index`).
- Restart `model-registry` with the fix.

---

## Failure mode 4 — Response-cache thrash

**Symptom:** `nv_inference_response_cache_miss_count` >>
`nv_inference_response_cache_hit_count`. Cache is contributing only
overhead.

### Triage

```bash
curl -s localhost:8002/metrics | grep -E 'response_cache_(hit|miss)_count'
```

### Cause

A high-cardinality input (per-frame detector, embedding model on
unique text) is being cached. Cache is unsafe for these models.

### Mitigation

Edit the model's `config.pbtxt`:

```pbtxt
response_cache {
  enable: false
}
```

Re-upload and reload via `model-registry`.

---

## Failure mode 5 — Cold-start latency spike

**Symptom:** First request after a model load has
`compute_input_duration_us` 10–100× the warm value.

### Cause

The model loaded without warm-up, or warm-up was insufficient. Triton
captures the CUDA graph on first inference; that capture is expensive.

### Mitigation

- Verify the model's `config.pbtxt` has a `model_warmup` block.
- Verify `prefetch-service` has the (tenant, model) pair in its 7×24
  EMA grid.
- For per-tenant models, increase `prefetch-service.horizonMinutes` so
  warm-ups dispatch earlier.

---

## Failure mode 6 — MIG slice exhausted

**Symptom:** New Triton pods stuck `Pending` with
`Insufficient nvidia.com/mig-1g.10gb`.

### Triage

```bash
kubectl get nodes -L nvidia.com/gpu.product -o wide
kubectl describe node <gpu-node> | grep -A5 'mig'
```

### Mitigation

- Add a GPU node (in cloud, scale the GPU node-group; on-prem, page
  facilities).
- Increase the MIG slice density on existing nodes (the GPU operator
  reconfigures live with no Triton-pod restart, but draining is wise).
- For sustained high load, re-evaluate the MIG profile (move from
  `1g.10gb` → `2g.20gb` to halve pod count and double per-pod
  throughput).

---

## Failure mode 7 — Triton pod OOMKilled

**Symptom:** Triton pods cycle on OOMKilled. `kubectl describe pod`
shows `Reason: OOMKilled` and `Last State.Reason: Error`.

### Triage

```bash
kubectl -n aegis-inference describe pod <pod>
kubectl -n aegis-inference top pod -l app.kubernetes.io/name=triton
```

### Cause

CPU memory limit (`resources.limits.memory`) too low for the loaded
model set. Triton's pinned-memory pool plus the model weights (for
non-GPU-resident weights, e.g. TensorFlow CPU) can exceed the chart
default 32 GiB.

### Mitigation

- Increase `resources.limits.memory` in `values.yaml`.
- Reduce `triton.pinnedMemoryPoolByteSize` if shared-memory I/O is not
  in use.

---

## Failure mode 8 — Rolling update causes 503s

**Symptom:** Brief 503 spike on the `inference-router` during a Triton
rollout.

### Cause

Service-mesh endpoint propagation lags behind pod readiness — the
inference-router gets a connection to a pod that's already draining.

### Mitigation

The chart's `preStop` lifecycle hook sleeps for 10 s before
`SIGTERM` to let mesh endpoints rotate. If you still see 503s:

- Increase the `preStop` sleep to 15 s.
- Verify Istio Ambient is healthy (no L4 short-circuiting bypass).
- As a fallback, scale the deployment up by one (`replicas+1`) before
  the rolling update, then back down after.

---

## Chaos drill

Run the chaos experiment that evicts a Triton model:

```bash
kubectl apply -f deploy/chaos/triton-model-evict.yaml
# Wait ~30 s; verify:
#   - prefetch-service re-warms the evicted model
#   - inference-router retries once
#   - glass-to-event SLO remains green
kubectl delete -f deploy/chaos/triton-model-evict.yaml
```

The check job at the bottom of the manifest asserts the expected
behaviour and fails the drill if the SLO budget burns more than 5 %.

---

## Escalation

| Severity | Action |
| --- | --- |
| Single model failing | Triage as above; do not page. |
| Cluster-wide 5xx > 1 % for 5 min | Page infra-gpu primary. |
| Glass-to-event SLO burns critical (multi-window) | Page ml-reliability; consider candidate rollback via `canary-controller`. |
| GPU pool unavailable | Page infra-gpu + leadership; activate DR runbook. |

---

## Related

- [`docs/triton.md`](../triton.md) — operating manual.
- [`docs/runbooks/canary-rollback.md`](./canary-rollback.md) — when a
  Triton-served candidate model regresses.
- [`docs/runbooks/drift-spike.md`](./drift-spike.md) — when class
  distribution shifts (often correlates with Triton compute regressions).
- [`docs/runbooks/incident-response.md`](./incident-response.md) —
  general incident workflow.
- [`deploy/chaos/triton-model-evict.yaml`](../../deploy/chaos/triton-model-evict.yaml) — drill manifest.
