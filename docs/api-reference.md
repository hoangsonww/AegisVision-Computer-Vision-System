# API Reference

> **Public REST API.** Versioned, opaque IDs, RFC 9457 errors, cursor
> pagination, idempotency.

All endpoints below live under `/v1/`. Authentication is **Bearer
JWT** (OIDC). Tenants are scoped via the `tenant_id` JWT claim and an
optional `X-Aegis-Tenant` header (rejected if mismatched).

Errors are **RFC 9457 `application/problem+json`**:

```json
{
  "type": "https://aegisvision.example.com/problems/idempotency-conflict",
  "title": "Idempotency-Key conflict",
  "status": 409,
  "detail": "Different body under same key.",
  "instance": "/v1/streams",
  "trace_id": "00-abcd-..."
}
```

---

## Conventions

- Mutating methods (`POST`, `PATCH`, `DELETE`) accept `Idempotency-Key`. Replays within 24h return the cached response byte-identically.
- List endpoints use opaque cursor pagination via `?page_token=...&page_size=...`.
- All IDs are **opaque strings**: `pl_*`, `str_*`, `mdl_*`, `ds_*`, `tn_*`, `g_*`.

---

## Health

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/health` | GET | Liveness. |
| `/v1/ready` | GET | Readiness (depends on `stream-manager`, `event-service`). |

---

## Pipelines

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/pipelines` | GET, POST | List, create. |
| `/v1/pipelines/{id}` | GET, PATCH, DELETE | Get, update, delete. |
| `/v1/pipelines/{id}/revisions` | GET, POST | List + cut revision. |
| `/v1/pipelines/{id}/revisions/{ver}` | GET | Read revision. |
| `/v1/pipelines/{id}:promote-revision` | POST | Promote a revision to current. |

POST `/v1/pipelines`:

```json
{
  "name":"dock-pipeline",
  "project_id":"proj_xxx",
  "dag": [
    {"operator":"ingest","params":{}},
    {"operator":"sampler","params":{"rate":5}},
    {"operator":"detect","params":{"model":"mdl_person_v1"}},
    {"operator":"tracker","params":{"algorithm":"bytetrack"}},
    {"operator":"rule","params":{"name":"dwell","class":"person","zone":"...","min_ms":5000}},
    {"operator":"emit","params":{}}
  ]
}
```

---

## Streams

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/streams` | GET, POST | List, create. |
| `/v1/streams/{id}` | GET, PATCH, DELETE | Get, update, delete. |
| `/v1/streams/{id}:pause` | POST | Pause. |
| `/v1/streams/{id}:resume` | POST | Resume. |

POST `/v1/streams`:

```json
{
  "name":"dock-1",
  "project_id":"proj_xxx",
  "protocol":"rtsp",
  "url":"rtsp://camera-7/stream",
  "pipeline_id":"pl_abc",
  "pipeline_revision_id":"rev_xyz",
  "tags":{"site":"warehouse"}
}
```

---

## Models

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/models` | GET, POST | List, register. |
| `/v1/models/{id}` | GET, PATCH | Get, update. |
| `/v1/models/{id}/versions` | GET, POST | List, add version. |
| `/v1/models/{id}:promote` | POST | (Gated.) Promote a candidate. |

---

## Datasets

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/datasets` | GET, POST | List, create. |
| `/v1/datasets/{id}` | GET, PATCH | Get, update. |
| `/v1/datasets/{id}/versions` | GET, POST | List, cut version. |
| `/v1/datasets/{id}/versions/{ver}/samples` | GET, POST | List, add samples. |

---

## Annotations

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/label-policies` | GET, POST | List, create. |
| `/v1/label-policies/{id}/revisions` | GET, POST | List, cut revision. |
| `/v1/annotations` | GET, POST | List, create. |
| `/v1/annotations/{id}` | GET, PATCH, DELETE | Resource. |

---

## Training jobs

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/training-jobs` | GET, POST | List, start. |
| `/v1/training-jobs/{id}` | GET | Read. |
| `/v1/training-jobs/{id}:cancel` | POST | Cancel. |

---

## Media

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/media/clips` | GET, POST | List, request a clip. |
| `/v1/media/clips/{id}` | GET | Get signed download URL. |
| `/v1/media/recordings` | GET | List recordings. |
| `/v1/media/retention-policies` | GET, POST | List, set retention. |

---

## Events

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/events` | GET | Cursor-paginated. |
| `/v1/events/{id}` | GET | Get one. |
| `/v1/events:stream` | GET (SSE) | Realtime feed. |

---

## Tenants + projects

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/tenants` | GET, POST | List, create. |
| `/v1/tenants/{id}` | GET, PATCH, DELETE | Resource (DELETE = crypto-shred). |
| `/v1/tenants/{id}/projects` | GET, POST | List, create. |
| `/v1/tenants/{id}/members` | GET, POST | List, add. |

---

## Agents

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/agents/sessions` | POST | Open a session. |
| `/v1/agents/sessions/{id}` | GET, DELETE | Inspect, close. |
| `/v1/agents/sessions/{id}/messages` | POST | Send a message. |

---

## Gates

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/gates` | GET | List pending. |
| `/v1/gates/{id}` | GET | Inspect. |
| `/v1/gates/{id}:approve` | POST | Approve. |
| `/v1/gates/{id}:deny` | POST | Deny. |

---

## Canary plans

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/canary-plans` | GET, POST | List, submit. |
| `/v1/canary-plans/{id}` | GET | Inspect. |
| `/v1/canary-plans/{id}:cancel` | POST | Cancel. |

---

## Knowledge

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/knowledge:query` | POST | Cited snippet retrieval. |
| `/v1/knowledge:ingest` | POST | Re-ingest a path. |

---

## NLQ

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/nlq:parse` | POST | Natural language → structured query. |

---

## LLM (internal gateway)

OpenAI-compatible. Callers use any OpenAI SDK.

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/chat/completions` | POST | Chat. |
| `/v1/embeddings` | POST | Embeddings. |
| `/v1/moderations` | POST | Moderation. |

---

## Metering

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/metering/usage` | GET | Per-tenant usage rollup. |
| `/v1/metering/invoices` | GET | Period invoices. |

---

## Compliance evidence

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/evidence:bundle` | POST | Produce a signed evidence bundle. |
| `/v1/controls` | GET | List available controls. |

---

## Drift

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/drift/runs` | GET, POST | List, manual trigger. |
| `/v1/drift/runs/{id}` | GET | Read. |

---

## Rules

| Path | Method | Purpose |
| --- | --- | --- |
| `/v1/rules` | GET, POST | List, create. |
| `/v1/rules/{id}` | GET, PATCH, DELETE | Resource. |
| `/v1/rules:evaluate` | POST | Evaluate against events. |

---

## Console

Two surfaces exist:

| Surface | Path | Notes |
| --- | --- | --- |
| Walking-skeleton (Phase 1) | `GET /console/` on `api-gateway` | Served when `AEGIS_CONSOLE_DIR` is set. Minimal HTML+JS for the 5-service skeleton demo. |
| Production console (Phase 7) | Separate `services/console/` Next.js app, served on its own host (e.g. `console.<your-domain>`) | Exposes every endpoint in this reference as a usable UI page. See [`console.md`](./console.md). |

Both consume **only the public REST endpoints documented above**. The
console is, in effect, the largest and best-tested client of this API
reference.

---

## See also

- [`concepts.md`](./concepts.md) — what these resources mean.
- [`console.md`](./console.md) — the production UI built on this API.
- [`/proto/aegisvision/`](../proto/aegisvision/) — protobuf contracts.
