# deploy/helm/console

> **Helm chart for the production Next.js console.**

Deploys a 2-replica (autoscaling to 8), HPA-bound, PDB-protected,
ServiceMonitor-scraped, mTLS-STRICT, OPA-AuthZ-gated, default-deny
NetworkPolicy, non-root read-only-rootfs Next.js standalone server.

---

## Install

```bash
helm install console deploy/helm/console -n aegis-system \
  --set apiBaseUrl=http://api-gateway.aegis-system.svc.cluster.local:8080
```

Or via ArgoCD — `deploy/argocd/applicationset.yaml` reconciles this chart
automatically.

---

## Values

| Key | Default | Purpose |
| --- | --- | --- |
| `image.repository` | `ghcr.io/hoangsonww/console` | Image name. |
| `image.tag` | `0.1.0` | |
| `replicaCount` | `2` | |
| `service.port` | `80` | ClusterIP port. |
| `service.containerPort` | `8090` | Pod port. |
| `service.metricsPort` | `9090` | Scrape port. |
| `apiBaseUrl` | `http://api-gateway.aegis-system.svc.cluster.local:8080` | Browser-side URL (`NEXT_PUBLIC_AEGIS_API_BASE`). |
| `hpa.enabled` | `true` | |
| `hpa.minReplicas` | `2` | |
| `hpa.maxReplicas` | `8` | |
| `hpa.cpuUtilization` | `70` | Target CPU %. |
| `pdb.enabled` | `true` | |
| `pdb.minAvailable` | `1` | |
| `serviceMonitor.enabled` | `true` | |
| `serviceMonitor.interval` | `30s` | |
| `serviceMonitor.path` | `/api/metrics` | |
| `securityContext.runAsNonRoot` | `true` | |
| `securityContext.runAsUser` | `1001` | |
| `containerSecurityContext.readOnlyRootFilesystem` | `true` | |
| `containerSecurityContext.capabilities.drop` | `[ALL]` | |

---

## Templates

The chart renders the 9 resources that the conformance test asserts
every chart must include:

| File | Kind |
| --- | --- |
| `deployment.yaml` | `Deployment` |
| `service.yaml` | `Service` |
| `serviceaccount.yaml` | `ServiceAccount` |
| `peerauthentication.yaml` | `PeerAuthentication` (mTLS STRICT) |
| `authorizationpolicy.yaml` | `AuthorizationPolicy` (ALLOW from api-gateway + istio-ingress + prometheus) |
| `networkpolicy.yaml` | `NetworkPolicy` (default-deny + explicit allows) |
| `servicemonitor.yaml` | `ServiceMonitor` |
| `hpa.yaml` | `HorizontalPodAutoscaler` |
| `poddisruptionbudget.yaml` | `PodDisruptionBudget` |

---

## Behind a public ingress

Front the chart with an Istio Gateway + VirtualService (or your usual
ingress). The console talks to `api-gateway` over the cluster network —
`apiBaseUrl` should be the in-cluster Service DNS, not the external host.

Typical hostname split:

- `console.aegisvision.example.com` → `console.Service:80`
- `api.aegisvision.example.com` → `api-gateway.Service:8080`

Browser JS is happy with same-origin or properly CORS-wired
cross-origin. `api-gateway` ships with CORS-permitting middleware for
the console origin.

---

## Per-tenant theming

The console reads its tenant from the OIDC `tenant_id` claim (set by
`auth-proxy`). For visual customization (logo / colors / banner),
override the values at chart install:

```bash
helm install console deploy/helm/console -n aegis-system \
  --set image.tag=v0.5.0-theme-acme \
  --set apiBaseUrl=https://api.aegisvision.example.com
```

Or fork the source under `services/console/` and bake your brand assets
into the image.

---

## See also

- [`../../../services/console/`](../../../services/console) — source.
- [`../../../docs/console.md`](../../../docs/console.md) — full console docs.
- [`../../README.md`](../../README.md) — every chart in the deploy tier.
