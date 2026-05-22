# ADR-0018: LLM/VLM gateway uniformity

**Status:** Accepted (2026-05-21)

## Context

Phase 4 brings agents, RAG, NL query, and active learning. All of these
need a language model (and one of them, the redaction-classifier-from-text
path, needs a vision-language model). We have a choice between:

1. Each service speaks directly to whichever backend we want for that
   workload — Triton+TRT-LLM for production, vLLM for prototyping, hosted
   APIs for breaking-glass.
2. One internal gateway speaks a uniform OpenAI-compatible vocabulary;
   every caller targets the gateway.

## Decision

We adopt option 2. The platform exposes a single internal endpoint
`llm-gateway.aegisvision.svc:8400` that speaks OpenAI-compatible JSON for
chat, embeddings, and a small vision/describe extension. Callers depend
only on the gateway; backend swaps happen at the gateway, not at the
callers.

This gives us:

- **Uniform safety layer.** The gateway always runs the prompt-injection
  sanitizer + PII redactor (ADR-0021), so no service can opt out by
  forgetting to wire it.
- **Uniform accounting.** Token usage is counted once, in the gateway,
  and exported as `aegis_llm_tokens_total{tenant, model, kind}`. This is
  the input to per-tenant cost-accounting.
- **Uniform rate limiting.** Per-tenant LLM quotas live alongside other
  quotas in tenant-service and are enforced via
  `pkg/platform/middleware.RateLimit` at the gateway.
- **Backend portability.** OpenAI-compatible is the universal shim; any
  backend that speaks it (vLLM, TGI, Triton+TRT-LLM, Bedrock+shim, hosted
  OpenAI) is interchangeable.
- **Air-gapped fallback.** When the air-gapped bundle ships, the gateway
  is the only thing that needs an offline backend wired in.

## Consequences

- **One additional hop in the request path.** ~3–5 ms in-cluster overhead;
  acceptable given we cap LLM calls at ~5–10/min/agent.
- **The "fake" backend is dev-only.** Outside dev/test, the gateway
  refuses to start without an `AEGIS_LLM_BACKEND_URL`. Synthetic responses
  must never serve production tenant traffic.
- **Image bytes never reach the gateway.** Vision inputs are claim-check
  URNs (ADR-0008); the gateway dereferences and feeds the image to the
  VLM backend, but the URN — not the bytes — is what crosses the network.

## Rejected alternatives

- **Per-service direct integration.** Rejected — it forces every Phase-4
  service to reimplement the safety layer and accounting.
- **Sidecar per pod.** Rejected — introduces N+1 deployments and makes
  backend swaps a fleet operation.
