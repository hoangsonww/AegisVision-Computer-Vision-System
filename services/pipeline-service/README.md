# pipeline-service

> **Owns pipeline DAGs and their revisions.** A pipeline is a recipe — the
> ordered set of operators (ingest, detect, track, rule, emit) that runs
> against a stream.

`pipeline-service` is the smallest and most-canonical control-plane
service. It is the reference implementation: when you add a new service,
you copy this one's shape.

It exposes a gRPC API for CRUD-with-revisions over `Pipeline` resources,
backed by Postgres. Streams pick a pipeline by ID; the data plane reads
the pipeline DAG via `stream-manager` and dispatches operator-control
messages accordingly.

---

## What lives in a Pipeline

```protobuf
message Pipeline {
  string id = 1;                   // opaque, e.g. "pl_abc123"
  string tenant_id = 2;
  string project_id = 3;
  string name = 4;
  string description = 5;
  repeated OperatorSpec dag = 6;   // ordered operator chain
  string current_revision_id = 7;
  google.protobuf.Timestamp created_at = 8;
  google.protobuf.Timestamp updated_at = 9;
  string etag = 10;
}

message OperatorSpec {
  string operator = 1;             // "ingest" | "sampler" | "detect" |
                                   // "tracker" | "rule" | "emit"
  google.protobuf.Struct params = 2;
}

message PipelineRevision {
  string id = 1;
  string pipeline_id = 2;
  repeated OperatorSpec dag = 3;
  string created_by = 4;
  google.protobuf.Timestamp created_at = 5;
}
```

Pipelines are **immutable per revision**. A "change" creates a new
`PipelineRevision`. The `current_revision_id` is what the data plane
sees.

---

## gRPC API

```
ListPipelines(ListPipelinesRequest) returns (ListPipelinesResponse);
GetPipeline(GetPipelineRequest) returns (Pipeline);
CreatePipeline(CreatePipelineRequest) returns (Pipeline);
UpdatePipeline(UpdatePipelineRequest) returns (Pipeline);
DeletePipeline(DeletePipelineRequest) returns (google.protobuf.Empty);
ListRevisions(ListRevisionsRequest) returns (ListRevisionsResponse);
GetRevision(GetRevisionRequest) returns (PipelineRevision);
PromoteRevision(PromoteRevisionRequest) returns (Pipeline);
```

All methods are idempotent via `Idempotency-Key` (gRPC metadata). All
list methods use cursor pagination.

---

## Internal architecture

```mermaid
flowchart LR
    AG[api-gateway] -->|gRPC| PS[pipeline-service]
    PS --> SVC[internal/service]
    SVC --> STORE[internal/store<br/>Postgres]
    PS --> AUD[audit-service]
    PS --> EVT[bus<br/>pipeline.changed.v1]
```

- `internal/server` — gRPC handlers, one file per resource.
- `internal/service` — business logic. Pure functions.
- `internal/store` — Postgres access. Sqlite for dev.
- Publishes `pipeline.changed.v1` for downstream interested parties
  (`stream-manager` warms cache).

---

## Data shape

Two tables, both partitioned by `tenant_id`:

```sql
CREATE TABLE pipelines (
  id                  TEXT PRIMARY KEY,
  tenant_id           TEXT NOT NULL,
  project_id          TEXT NOT NULL,
  name                TEXT NOT NULL,
  description         TEXT,
  current_revision_id TEXT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL,
  etag                TEXT NOT NULL
);

CREATE TABLE pipeline_revisions (
  id          TEXT PRIMARY KEY,
  pipeline_id TEXT NOT NULL REFERENCES pipelines(id),
  dag         JSONB NOT NULL,
  created_by  TEXT NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL
);
```

Indexes lead with `(tenant_id, …)` so all queries are tenant-scoped.

---

## Run locally

```bash
# Uses SQLite in dev.
task run:pipeline-service

# Or against real Postgres:
AEGIS_PG_DSN=postgres://aegis:aegis@localhost:5432/aegis task run:pipeline-service
```

---

## Configuration

| Var | Purpose | Default |
| --- | --- | --- |
| `AEGIS_PG_DSN` | Postgres DSN. | sqlite in dev |
| `AEGIS_NATS_URL` | For `pipeline.changed.v1`. | empty in dev |
| `AEGIS_PORT_GRPC` | gRPC listen. | `:9091` |
| `AEGIS_PORT_HTTP` | Health/metrics. | `:9090` |

---

## Metrics

- `aegis_pipeline_requests_total{tenant,method,status}` — rate.
- `aegis_pipeline_request_duration_seconds{tenant,method}` — duration.
- `aegis_pipeline_revisions_total{tenant}` — running count.
- `aegis_pipeline_changes_published_total` — bus emit count.

---

## Failure modes

| Failure | Effect | Mitigation |
| --- | --- | --- |
| Postgres unavailable | gRPC 503 | Retry with backoff; alarm. |
| NATS unavailable | bus event drops | Local outbox + re-publish on recovery. |
| Concurrent update | 409 etag mismatch | Client retries. |
| Invalid DAG | 400 | Validation in `internal/service`. |

---

## See also

- [`../stream-manager/README.md`](../stream-manager/README.md) — the consumer of pipeline DAGs.
- [`/proto/aegisvision/control/v1/pipelines.proto`](../../proto/aegisvision/control/v1/) — gRPC contract.
- [`ARCHITECTURE.md`](../../ARCHITECTURE.md) — pipelines in the control plane.
