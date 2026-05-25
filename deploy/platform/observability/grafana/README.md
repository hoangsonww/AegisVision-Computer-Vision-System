# Grafana dashboards

The AegisVision template expects a Grafana instance with dashboards for
the operational signals adopters care about. Dashboards are
adopter-specific (your panel layout, your alert thresholds), so the
template does not pin a JSON — instead this file documents the panels
the platform's runbooks reference, so adopters can recreate them on
their own Grafana.

## Recommended dashboards

### Glass-to-event SLO
Source: api-gateway, stream-manager, event-service, dataplane-runner.
- Glass-to-event latency (p50 / p95 / p99) — RED metric histogram.
- Streams in steady-state count (`aegis_stream_active`).
- Events emitted per minute by tenant.
- Multi-window SLO burn-rate (1h / 6h / 24h).

### NVIDIA Triton inference
Source: Triton's own `:8002/metrics`.
- Inference rate (`rate(nv_inference_request_success[1m])`) per model.
- Inference failure rate (`rate(nv_inference_request_failure[1m])`).
- Queue duration (`triton:inference_queue_duration_us:p95`,
  `:p99` — pre-computed by `triton-rules.yaml`).
- Compute infer duration (`triton:inference_compute_infer_duration_us:p95`).
- Response cache hit ratio (`triton:inference_response_cache_hit_ratio`).
- Pending request count (`nv_inference_pending_request_count`).
- GPU utilization (`nv_gpu_utilization`).
- GPU memory used (`nv_gpu_memory_used_bytes`).
- Power draw (`nv_gpu_power_usage`) — cost-accounting.

### Bus subjects
Source: NATS + Kafka exporters + `aegis_bus_publish_total`,
`aegis_bus_consume_total`, `aegis_bus_redelivery_total`.
- Per-subject publish + consume rate.
- Redelivery rate (an indicator of consumer lag or stuck messages).
- Bus connection state.

### Bounded autonomy
Source: agent-service + policy-gate-service + audit-service.
- Active agent sessions count.
- Tier-3 gate requests / minute.
- Median time-to-resolve a tier-3 gate.
- Per-tenant LLM token spend (`aegis_llm_tokens_total`).

### Cost
Source: cost-accounting + metering-service.
- GPU-seconds by tenant.
- Inference calls by tenant (cache_hit vs miss).
- Storage cost by tenant.

## Provisioning

The kube-prometheus-stack chart can auto-provision dashboards from
ConfigMaps labelled `grafana_dashboard: "1"`. When you check your
dashboards into git, generate a ConfigMap per dashboard and apply via
your normal GitOps path.

## Reference

- [Triton metrics](https://docs.nvidia.com/deeplearning/triton-inference-server/user-guide/docs/user_guide/metrics.html)
- [`triton-rules.yaml`](../prometheus/triton-rules.yaml) — the
  recording + alerting rules the dashboards reference.
- [`docs/triton.md`](../../../../docs/triton.md) — the operating
  manual.
