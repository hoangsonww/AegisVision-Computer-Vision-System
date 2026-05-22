# edge-gateway

> **k3s-friendly outbox sync to core.**

`edge-gateway` lives on the edge (k3s, NVIDIA Jetson AGX, on-prem
rack). It runs a reduced data plane locally and syncs events to the
core cluster via an outbox.

```mermaid
flowchart LR
    subgraph Edge (k3s)
        CAM[camera] --> DR[dataplane-runner<br/>edge profile]
        DR --> EG[edge-gateway]
        EG --> OUTBOX[(local outbox)]
    end
    EG --> CORE[(core cluster)]
```

---

## Outbox

Local SQLite (or BoltDB) holds pending events. The sync loop flushes
to the core's `event-service` via gRPC over mTLS. On reconnect after
outage, it drains in order.

This is the standard outbox pattern: writes commit locally first,
sync follows.

---

## Configuration

| Var | Purpose |
| --- | --- |
| `AEGIS_CORE_GRPC` | Core cluster gRPC endpoint. |
| `AEGIS_CORE_TOKEN_FILE` | Bearer token for sync. |
| `AEGIS_OUTBOX_DIR` | Local outbox storage. |
| `AEGIS_OUTBOX_MAX_BYTES` | Hard cap; refuse to accept when full. |

---

## See also

- [`deploy/helm/edge-profile/README.md`](../../deploy/helm/edge-profile/README.md).
- [`SETUP_GUIDE.md`](../../SETUP_GUIDE.md) — Path E (edge install).
