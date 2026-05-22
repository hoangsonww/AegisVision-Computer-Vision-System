# gpu-scheduler

> **MIG-default GPU reservation ledger.** ADR-0003.

Production inference does not "share" a GPU softly. Slices are
reserved through this ledger.

`gpu-scheduler` exposes a gRPC API:

```
ReserveSlice(req) returns (Reservation);
ReleaseSlice(reservation_id);
DescribeSlices();
```

`inference-router` calls `ReserveSlice` before every Triton call; on
unavailability, the router returns 503 to its caller. **No fallback
to soft-sharing.** That is the entire point of MIG-default.

---

## Why MIG-default

A single misbehaving model on a shared GPU can blow up *every*
in-flight request — OOM, memory corruption, eviction storms. MIG
slices are hardware-isolated: an OOM on slice A is invisible to
slice B.

The chaos drill `deploy/chaos/gpu-oom-reject.yaml` injects an OOM
and asserts that the scheduler rejects the request rather than
allowing soft-share fallback.

---

## See also

- [`../inference-router/README.md`](../inference-router/README.md).
- ADR-0003.
