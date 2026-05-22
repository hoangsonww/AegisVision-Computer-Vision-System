# Data flow

> **End-to-end flow diagrams for every path data takes through the
> platform.**

If you remember nothing else: **frames never go on the bus**. The bus
carries claim-check URNs. ADR-0008.

---

## 1. Glass-to-event (the hot path)

This is the path that defines the platform's latency budget. The
walking-skeleton p95 is 2.7 ms on local hardware; the documented
production target is 300 ms.

```mermaid
sequenceDiagram
    autonumber
    participant CAM as Camera
    participant DR as dataplane-runner
    participant CC as claim-check
    participant IR as inference-router
    participant TRT as Triton + MIG
    participant NATS as NATS
    participant ES as event-service
    participant CH as ClickHouse
    participant AG as api-gateway
    actor U as User

    CAM->>DR: RTSP frame
    DR->>CC: PUT frame_urn = cc://t-7/s-1/seq-N
    DR->>DR: sampler (down-sample)
    DR->>IR: Infer(frame_urn, model)
    IR->>TRT: detect
    TRT-->>IR: detections
    IR->>NATS: inference.completed.v1
    IR-->>DR: detections
    DR->>DR: tracker, rule
    alt rule trips
        DR->>NATS: events.v1
        NATS->>ES: deliver
        ES->>CH: insert
        ES->>AG: SSE push (chunked)
        AG->>U: SSE event
    end
```

**What touches imagery bytes:** camera, dataplane-runner, claim-check,
inference-router → Triton. Everyone else operates on URNs.

---

## 2. Inference fan-out

```mermaid
flowchart LR
    IR[inference-router] -->|inference.completed.v1| NATS
    NATS --> MET[metering-service]
    NATS --> DD[drift-detection-service]
    NATS --> CA[cost-accounting]
    NATS --> AL[active-learning-service]

    IR -->|inference.baseline.v1| NATS2[NATS]
    NATS2 --> SI[shadow-inference-service]

    IR -->|inference.outcome.v1| NATS3[NATS]
    NATS3 --> CC[canary-controller]
```

Each `Infer` call emits up to three bus events. Integration smoke
asserts all three have publishers + consumers.

---

## 3. Bounded-autonomy gate round-trip

```mermaid
sequenceDiagram
    autonumber
    actor U as User
    participant AS as agent-service
    participant LG as llm-gateway
    participant KS as knowledge-service
    participant PG as policy-gate-service
    participant NATS as NATS
    actor A as Approver

    U->>AS: "promote my-model-v2"
    AS->>LG: chat completion
    LG-->>AS: tool=promote_model (tier 3)
    AS->>KS: query_knowledge (for citation)
    KS-->>AS: snippets
    AS->>PG: RequestGate(promote_model, args, citations)
    PG-->>AS: gate_id
    AS-->>U: "pending approval (gate_id)"
    PG->>A: notify
    A->>PG: approve
    PG->>NATS: gate.resolved.<gate_id>
    NATS->>AS: deliver
    AS->>AS: resume (auto)
    AS-->>U: "Promoted my-model-v2."
```

---

## 4. Training → registry → canary → promotion

```mermaid
flowchart TB
    AS[annotation-service] --> DS[dataset-service]
    DS --> TO[training-orchestrator]
    TO --> KF[Kubeflow / Ray]
    KF --> ART[artifact]
    ART --> MR[model-registry]
    MR --> CC[canary-controller]
    CC -->|recommend promote| PG[policy-gate-service]
    PG --> MR
    MR -->|promotion event| NATS
    NATS --> IR[inference-router<br/>traffic flip]
```

---

## 5. Tenant offboarding (crypto-shredding)

```mermaid
sequenceDiagram
    autonumber
    actor A as Admin
    participant TS as tenant-service
    participant AUD as audit-service
    participant V as Vault transit
    participant NATS as NATS

    A->>TS: DELETE /v1/tenants/{id}
    TS->>AUD: append delete_tenant (fail-closed)
    AUD-->>TS: ok
    TS->>V: delete transit/keys/aegis-tenant-{id}
    V-->>TS: ok
    TS->>NATS: tenant.deleted.v1
    NATS->>OBS[multiple consumers]
    OBS->>OBS: drop indexes / GC caches
```

After this point the tenant's encrypted bytes — in ClickHouse,
Postgres, object store, **including backups** — are unreadable.

---

## 6. Edge → core outbox sync

```mermaid
flowchart LR
    subgraph edge (k3s)
        CAM[camera] --> DR[dataplane-runner]
        DR --> ES[event-service<br/>edge]
        ES --> EG[edge-gateway]
        EG --> OUT[(local outbox)]
    end
    EG -->|periodic sync gRPC| ESC[event-service<br/>core]
    ESC --> CH[(ClickHouse core)]
```

The outbox commits locally first. On partition, edge keeps producing
events; sync drains on recovery.

---

## 7. Drift → SLO → page

```mermaid
flowchart LR
    IR[inference-router] -->|inference.completed.v1| NATS
    NATS --> DD[drift-detection-service]
    DD -->|drift.alert.v1| NATS2
    NATS2 --> SLO[slo-watchdog]
    SLO -->|burn-rate exceeded| ALERT[alertmanager]
    ALERT --> OC[oncall]
```

---

## 8. Predictive prefetch

```mermaid
flowchart LR
    IR[inference-router] -->|inference.completed.v1| NATS
    NATS --> PF[prefetch-service]
    PF --> GRID[(7×24 EMA grid)]
    PF -->|10-min horizon| WARM[warm-up dispatch]
    WARM --> TRT[Triton]
```

---

## What never appears on these diagrams

- **No image bytes on the bus.** Always URNs.
- **No per-frame Postgres calls.** Per-frame work is stateless.
- **No raw LLM calls.** Always through `llm-gateway`.
- **No second agent runtime.** All sessions through `agent-service`.

If you find yourself drawing one of those, that's an ADR-tracked
change — not a feature.

---

## See also

- [`ARCHITECTURE.md`](../ARCHITECTURE.md) — the full architectural view.
- [`concepts.md`](./concepts.md) — what the resources mean.
- [`adr/`](./adr/) — the load-bearing decisions.
