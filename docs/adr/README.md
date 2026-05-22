# Architecture Decision Records

The decisions that govern this repository.

| ADR | Title |
| --- | --- |
| 0001 | [Hard separation of control plane and data plane](0001-two-plane-separation.md) |
| 0002 | [ClickHouse firehose, PostgreSQL metadata](0002-clickhouse-firehose.md) |
| 0003 | [MIG default for production inference](0003-mig-default.md) |
| 0007 | [Protobuf everywhere, Buf-managed](0007-protobuf-buf.md) |
| 0008 | [Claim-check for frames](0008-claim-check-for-frames.md) |
| 0014 | [Bounded autonomy with human gates](0014-bounded-autonomy.md) |
| 0016 | [Walking skeleton first](0016-walking-skeleton-first.md) |
| 0017 | [Bounded-autonomy implementation](0017-bounded-autonomy-implementation.md) |
| 0018 | [LLM/VLM gateway uniformity](0018-llm-gateway-uniformity.md) |
| 0019 | [Active learning loop](0019-active-learning-loop.md) |
| 0020 | [RAG over plain LLM](0020-rag-over-plain-llm.md) |
| 0021 | [Prompt-injection defense in depth](0021-prompt-injection-defense.md) |
| 0022 | [Continuous autonomy — scheduled agents](0022-continuous-autonomy.md) |
| 0023 | [Canary regression + auto-rollback](0023-canary-regression-rollback.md) |
| 0024 | [Shadow inference architecture](0024-shadow-inference.md) |
| 0025 | [Drift + SLO signals](0025-drift-and-slo-signals.md) |
| 0026 | [Predictive prefetch](0026-predictive-prefetch.md) |
| 0027 | [Air-gapped bundle as day-one artifact](0027-air-gapped-bundle.md) |
| 0028 | [Chaos engineering as readiness gate](0028-chaos-as-readiness-gate.md) |
| 0029 | [Compliance-evidence-service owns no data](0029-compliance-evidence-service.md) |
| 0030 | [Release engineering — release-please + signed bundles](0030-release-engineering.md) |

The full set of 16 ADRs lives in the Notion architecture doc; this folder
mirrors the load-bearing decisions for the building blocks shipped in this
commit. Additional ADRs will be added to this directory as their decisions
land in code.
