# Glossary

> **Terminology used across AegisVision.** When two docs use the same
> word differently, *this* is the canonical definition.

---

| Term | Definition |
| --- | --- |
| **ADR** | Architecture Decision Record. Lives in `docs/adr/`. |
| **Active learning** | Sampling strategy that picks uncertain + diverse samples for labelling. ADR-0019. |
| **Adaptive autonomy** | The capability area covering canary, shadow, drift, SLO, prefetch, and autonomy-orchestrator. |
| **Agent** | The bounded-autonomy reasoning loop. ADR-0014/0017. |
| **Air-gapped bundle** | Single signed tarball containing every image + chart + manifest + signature. ADR-0027. |
| **Annotation** | A label attached to a sample. Owned by `annotation-service`. |
| **Audit record** | Append-only, hash-chained log entry. Mandatory for every mutating action. ADR-0014. |
| **Auto-resume** | When the agent waits on a tier-3 gate and continues when `gate.resolved.<id>` arrives. |
| **Backpressure** | Bounded channel buffers in the operator DAG; drops at the source when downstream is slow. |
| **Baseline (canary)** | The currently-promoted model. Compared against the candidate. ADR-0023. |
| **Bounded autonomy** | The platform's autonomy model: tiered tools, refusal in code for tier-3. ADR-0014. |
| **Burn rate** | SLO error-budget consumption rate. Multi-window. ADR-0025. |
| **Bus** | The event backbone. NATS + Kafka via `pkg/bus`. |
| **Candidate (canary)** | The model under evaluation. Promoted only on Wilson lower bound + gate. |
| **Canary plan** | A statistical comparison of two model versions. ADR-0023. |
| **Citation discipline** | Every platform-fact answer must cite knowledge-service snippets. ADR-0020. |
| **Console** | The production Next.js UI at `services/console/`. Exposes every public REST endpoint as a usable page. |
| **Console (walking-skeleton)** | The minimal vanilla HTML+JS demo at `services/api-gateway/console/`. Used only for the 5-service walking-skeleton demo. |
| **Claim-check** | Pattern: store bytes in object storage, send only the reference (URN) on the bus. ADR-0008. |
| **ClickHouse** | Columnar OLAP database for detections + events firehose. ADR-0002. |
| **Continuous autonomy** | Cron + signal-driven agent sessions. Opens regular agent-service sessions. ADR-0022. |
| **Control plane** | Per-event, transactional, durable services (CRUD + revisions). ADR-0001. |
| **Cosign keyless** | OIDC-based image signing. No long-lived keys. |
| **Crypto-shredding** | Destroy the tenant's Vault transit key → all encrypted bytes unreadable (incl. backups). ADR-0014. |
| **Cursor pagination** | Opaque, HMAC-signed, expiring page tokens. Never offset. |
| **Data plane** | Per-frame, stateless operator runtime. ADR-0001. |
| **Dataset / Dataset version** | Named, versioned collection of samples. |
| **Detection** | Model output on a single frame (bbox + class + confidence). |
| **DLQ** | Dead-letter queue. Where unprocessable bus messages go. |
| **Drift** | Distribution change in model outputs vs reference. ADR-0025. |
| **DualBus** | NATS + Kafka in parallel for low-latency + durable consumers. `pkg/bus/dualbus.go`. |
| **Edge** | k3s-friendly reduced operator set. `services/edge-gateway`. |
| **EMA** | Exponentially-moving average. Used in prefetch-service for the 7×24 demand grid. ADR-0026. |
| **Event** | A platform-emitted message about something that happened (detection, rule fire, etc.). |
| **Etag** | Optimistic concurrency token on every mutating resource. |
| **ExternalSecret** | ESO CRD that syncs a Vault path to a Kubernetes Secret. |
| **Fail-closed** | If a security control cannot run (e.g. audit unreachable), refuse the operation. |
| **Frame** | A single video frame. Descriptor on the bus; bytes in claim-check. |
| **Gate** | Human-in-the-loop approval for tier-3 actions. `policy-gate-service`. |
| **Glass-to-event** | End-to-end latency: pixel from camera → event in tenant's hand. p95 target 300 ms. |
| **HMAC JWT** | Symmetric-signed JWT. **Deliberately not supported.** Sharing the key defeats OIDC. |
| **Idempotency-Key** | Header on every mutating endpoint; replays cached 24h. |
| **In-cluster registry** | Where the air-gapped install pushes images. |
| **Intelligence tier** | The capability area covering LLM gateway, agent, knowledge, policy-gate, NLQ, and active-learning. |
| **Istio Ambient** | Sidecar-less mesh. Used for mTLS STRICT. |
| **JS divergence** | Jensen-Shannon. Symmetric, bounded. Default drift metric. ADR-0025. |
| **JWKS** | JSON Web Key Set. IdP's public-key endpoint for JWT verification. |
| **Kyverno** | Admission policy engine. Enforces cosign signature, PSS restricted, etc. |
| **KL divergence** | Kullback-Leibler. Asymmetric, tail-sensitive. ADR-0025. |
| **Label policy** | Schema of classes / attributes. Immutable per revision. |
| **Lineage** | Records which dataset version trained which model version. |
| **MIG** | NVIDIA Multi-Instance GPU. Hardware partitioning. ADR-0003. |
| **mTLS STRICT** | All pod-to-pod traffic must be mTLS. No plaintext. |
| **Multi-window burn-rate** | SLO alert math: fast + slow windows both exceed thresholds → page. |
| **NLQ** | Natural-language query. `nlq-service` parses to structured form. |
| **OPA** | Open Policy Agent. Per-service AuthZ. |
| **Operator (data plane)** | A processing stage in the streaming DAG (ingest, sampler, etc.). |
| **Operator (Kubernetes)** | An operator that manages a CRD (e.g. ClickHouse Operator). |
| **Outbox** | Edge-side local store; sync to core follows. |
| **Patroni** | Postgres HA. Spilo / Zalando operator. |
| **pgvector** | Postgres extension for vector search. Used by `pkg/embeddings`. |
| **Pipeline** | A recipe — ordered DAG of operators. Immutable per revision. |
| **PII redactor** | Strips emails / phones / SSNs from LLM input. `pkg/llm/safety`. |
| **policy-gate-service** | Human-in-the-loop approval gate. |
| **Prefetch** | Predictive model warm-up via 7×24 EMA grid. ADR-0026. |
| **problem+json** | RFC 9457 error format. Mandatory for all errors. |
| **Project** | Grouping inside a tenant. Members + resources. |
| **Prompt-injection defense** | Sanitiser + PII redactor + refusal threshold + tier-3 gate. ADR-0021. |
| **RAG** | Retrieval-augmented generation. ADR-0020. |
| **Reference distribution** | Class distribution at training time. Stored in `model-registry`. ADR-0025. |
| **Refusal threshold** | Safety-score cutoff in `pkg/llm/safety`. Below it → 422. |
| **Risk tier** | The four-tier agent tool risk model (0 read, 1 advisory, 2 propose, 3 gated). |
| **RPC** | Remote procedure call (gRPC). |
| **Rule** | Predicate over detections. dwell / count / line-cross / zone-enter. |
| **Same-URN comparison** | Shadow inference comparing candidate vs baseline on the same frame URN. ADR-0024. |
| **Sample** | A labeled frame in a dataset. |
| **SBOM** | Software Bill of Materials. SPDX format. Attached to every image. |
| **Shadow inference** | Run candidate model on the same frame URN as baseline. Tenant never sees candidate. ADR-0024. |
| **SLO** | Service Level Objective. Multi-window burn rate. ADR-0025. |
| **SLSA** | Supply-chain Levels for Software Artifacts. v1 provenance attached to every release. |
| **SPIFFE / SPIRE** | Workload identity. Issues SPIFFE IDs to pods. |
| **SSE** | Server-Sent Events. The realtime feed at `/v1/events:stream`. |
| **Stream** | A producer of frames (RTSP, file, WebRTC, synthetic). |
| **Tenant** | Top-level isolation boundary. One Vault transit key. |
| **Tier 3 tool** | Consequential agent tool. Refused in code without resolved gate. |
| **Trace ID** | OTel trace identifier. Propagated through every bus message. |
| **Triton** | NVIDIA inference server. Hosts model artifacts. |
| **TVD** | Total Variation Distance. Bounded, interpretable. ADR-0025. |
| **URN** | Uniform Resource Name. Claim-check identifier (`cc://tenant/stream/seq`). |
| **Vault transit** | Per-tenant encryption keys. Destruction = crypto-shredding. |
| **Walking skeleton** | The thin-but-complete end-to-end slice of the platform (ADR-0016). Five services + embedded NATS in five terminals; produces a real detection event end-to-end. |
| **Wilson lower bound** | Confidence-interval method for binomial proportions. Canary stat. ADR-0023. |
| **Zone** | A polygon region used in rule predicates. |
