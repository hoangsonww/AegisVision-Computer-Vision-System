# tools/conformance

> **Helm chart shape test.** Every chart in `deploy/helm/` must
> conform to the platform contract.

This is a Go test suite that loads every chart, renders it, and
asserts the rendered manifests include:

- `Deployment` (or `StatefulSet`)
- `Service`
- `ServiceAccount`
- `AuthorizationPolicy` (Istio, ALLOW list)
- `PeerAuthentication` (mTLS STRICT)
- `NetworkPolicy` (default-deny)
- `ServiceMonitor`
- `HorizontalPodAutoscaler`
- `PodDisruptionBudget`

It also asserts:

- The container is non-root.
- Resources have requests + limits.
- Probes are defined.
- The OPA policy ConfigMap exists where applicable.
- The image reference points to a registry under `aegisvision/`.

---

## Run

```bash
(cd tools/conformance && go test -count=1 ./...)
# → ok   38 charts conformant
```

---

## Adding a new chart

The conformance test fails if a new chart is missing any of the above.
This is by design — every chart is observable and secure by default.
