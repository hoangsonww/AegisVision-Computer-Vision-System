# Using AegisVision as a template

> **The rebrand and adoption playbook.** This file is for adopters —
> individuals and organizations who want to fork AegisVision and use
> it as the backbone of their own computer-vision product.

AegisVision is published under the **Apache License 2.0** specifically
so that you can fork it, rebrand it, embed it in commercial products,
and operate it without asking anyone's permission. This document is
the playbook for doing that cleanly.

---

## Table of contents

1. [Decide what you're keeping](#decide-what-youre-keeping)
2. [Fork and rename](#fork-and-rename)
3. [Wire your NVIDIA Triton model repository](#wire-your-nvidia-triton-model-repository)
4. [Pick a deploy path](#pick-a-deploy-path)
5. [Customize the console](#customize-the-console)
6. [Pin your identity provider](#pin-your-identity-provider)
7. [Set up your secrets store](#set-up-your-secrets-store)
8. [Stand up observability](#stand-up-observability)
9. [Tune the autonomy bounds](#tune-the-autonomy-bounds)
10. [Run the compliance baseline](#run-the-compliance-baseline)
11. [Adoption checklist](#adoption-checklist)
12. [Trademark and naming](#trademark-and-naming)

---

## Decide what you're keeping

The template ships eight capability areas (see
[`README.md`](./README.md#capabilities-at-a-glance)). You probably
want all of them — they are wired together — but each is replaceable
behind a typed contract:

| Capability | Always keep | Optional / replaceable |
| --- | --- | --- |
| Foundations (`pkg/platform`, protobuf, walking skeleton) | ✓ | — |
| Glass-to-event ingest | ✓ | Swap `dataplane-runner`'s operator chain for DeepStream-native operators if needed. |
| GPU hot path (NVIDIA Triton) | ✓ (Triton is the load-bearing substrate) | Add backends as needed. |
| Multi-tenant + storage tier | ✓ | Single-tenant adopters can disable `tenant-service` quotas. |
| Intelligence tier (LLM + agent + RAG) | optional | If you don't need agentic UX, you can skip the intelligence-tier charts. |
| Adaptive autonomy (canary, drift, SLO, prefetch) | strongly recommended | Disable individual services in the Helm ApplicationSet. |
| Operations & compliance | ✓ | Trim compliance docs you don't need (e.g. EU AI Act if non-EU). |
| Production console | optional | Use the API directly; ship your own UI. |

Whatever you remove, remove it via the `deploy/argocd/applicationset.yaml`
include list — do not delete the charts from the repo until you're sure
you'll never re-enable them.

---

## Fork and rename

The product noun in this repo is **AegisVision** / **aegisvision**
(brand) and the Go module path is `github.com/aegisvision`. Renaming
takes about an hour.

### 1. Fork the repo

```bash
gh repo fork hoangsonww/AegisVision-Computer-Vision-System \
  --org your-org --remote --clone
cd AegisVision-Computer-Vision-System
git remote rename origin upstream
gh repo create your-org/your-product --private --source=. --remote=origin --push
```

### 2. Rename the Go module path

Edit `go.work` and every per-service `go.mod` to replace
`github.com/aegisvision` with your module path (e.g.
`github.com/yourorg/yourproduct`). Then update imports:

```bash
# macOS / BSD sed:
LC_ALL=C find . -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.work' \) \
  -not -path './node_modules/*' \
  -exec gsed -i 's|github.com/aegisvision|github.com/yourorg/yourproduct|g' {} +
# (Use plain sed on Linux: `sed -i ...`.)
gofmt -w .
```

### 3. Rename the brand strings

The brand appears in chart names, helm values, env-var prefixes
(`AEGIS_*`), service-account namespaces (`aegis-system`), the console
title, and the landing page. The repo ships a brand-rename Taskfile
target — run it interactively to see what would change before
applying:

```bash
task template:rebrand:dry-run \
  PRODUCT_NAME=YourProduct \
  PRODUCT_SLUG=yourproduct \
  ENV_PREFIX=YOURPROD
# Inspect the diff, then apply:
task template:rebrand \
  PRODUCT_NAME=YourProduct \
  PRODUCT_SLUG=yourproduct \
  ENV_PREFIX=YOURPROD
```

This rewrites Helm chart names, namespace defaults, env-var prefixes,
landing-page strings, and console copy. It does **not** rewrite the
ADRs — those are historical, and keeping the AegisVision attribution
satisfies the Apache 2.0 NOTICE requirement.

### 4. Update LICENSE + NOTICE

Apache 2.0 requires retaining the upstream `NOTICE` and copyright
header. Append your own copyright to `NOTICE` rather than removing
the existing entry. The upstream `LICENSE` text must stay verbatim.

### 5. Update CITATION + ACKNOWLEDGMENTS

If you publish your fork, update `CITATION.cff` to point at your
release and add your acknowledgment to the top of `ACKNOWLEDGMENTS.md`.
Keep the upstream acknowledgments intact.

---

## Wire your NVIDIA Triton model repository

NVIDIA Triton is the load-bearing GPU substrate. The template ships a
hardened Triton chart at [`deploy/helm/triton/`](./deploy/helm/triton);
adoption mostly comes down to pointing it at your model repository.

1. Create an object-storage bucket (S3, GCS, MinIO, or Ceph) for the
   repository. Example layout:

   ```
   s3://yourorg-models/triton-repo/
   ├── yolov8-person/
   │   ├── config.pbtxt
   │   └── 1/model.plan
   ├── face-blur-onnx/
   │   ├── config.pbtxt
   │   └── 1/model.onnx
   └── caption-ensemble/
       ├── config.pbtxt
       └── 1/                  # ensemble — no model file
   ```

   The full layout contract is in [`docs/triton.md`](./docs/triton.md#model-repository-layout).

2. Override the chart values for your bucket:

   ```yaml
   # values-yourorg.yaml
   modelRepository:
     uri: s3://yourorg-models/triton-repo
     region: us-east-1
     credentialsSecret: triton-s3-credentials
     failIfEmpty: false
   gpu:
     resourceName: nvidia.com/mig-1g.10gb
   ```

3. Register each model in `model-registry` via
   `POST /v1/models` so the canary controller and `prefetch-service`
   know about it. The platform takes care of the actual Triton
   `/v2/repository/.../load` call.

4. Reference your models from a `Pipeline` resource — the
   `dataplane-runner` will route inference through `inference-router`
   to Triton automatically.

---

## Pick a deploy path

[`SETUP_GUIDE.md`](./SETUP_GUIDE.md) documents four paths:

| Path | Use when |
| --- | --- |
| **A — Local walking skeleton** | Evaluating the template or developing against the platform. Five terminals, zero deps. |
| **B — Local full stack via docker-compose** | Developing against the intelligence tier + autonomy services. |
| **C — Online cluster via ArgoCD** | Production. Most adopters. |
| **D — Air-gapped install** | Regulated / classified / DMZ environments. Single signed tarball. |
| **E — Edge install (k3s)** | Camera-side deployments (e.g. NVIDIA Jetson). |

Most adopters start at A → B → C. D and E are additive on top of C.

---

## Customize the console

The Next.js console at [`services/console/`](./services/console)
exposes every public REST endpoint as a page. Customizing it is a
normal Next.js project — change `tailwind.config.ts`, the brand
colours in `src/app/layout.tsx`, the logo in `src/components/TopBar.tsx`,
and the title in `src/app/page.tsx`. The console reads
`NEXT_PUBLIC_AEGIS_API_BASE` at build time — change the variable name
and references to it if your env-var prefix differs.

If you ship your own UI on top of the API instead of the bundled
console, point `services/console/` at your own repo (disable the
chart in the ApplicationSet) and consume the API directly.

---

## Pin your identity provider

The platform expects:

- An OIDC IdP whose JWKS endpoint is reachable from the cluster.
- JWTs signed with RS256/ES256 — HMAC-signed JWTs are deliberately
  unsupported (`docs/glossary.md` "HMAC JWT").
- Tenant identity carried in a JWT claim or the `X-Tenant-Id` header
  (the latter only in dev).

Configure on `api-gateway`:

```bash
AEGIS_OPA_ENDPOINT=...
AEGIS_JWKS_URL=https://idp.example.com/.well-known/jwks.json
AEGIS_JWT_ISSUER=https://idp.example.com/
AEGIS_CURSOR_KEY=$(openssl rand -base64 48)
```

OPA policies live in your platform tier — the template does not embed
identity policies; that's a decision per-adopter.

---

## Set up your secrets store

The chart family expects External Secrets Operator. Each service has
an `ExternalSecret` template referencing a Vault path. Adopters
typically:

1. Stand up Vault (or replace with AWS Secrets Manager / GCP Secret
   Manager via ESO providers).
2. Populate the paths in `deploy/platform/external-secrets/`.
3. Let ESO sync into Kubernetes Secrets that the chart references.

The full path list is in
[`SETUP_GUIDE.md`](./SETUP_GUIDE.md#step-4--wire-secrets).

---

## Stand up observability

The platform expects:

- An OTLP collector for traces (`OTEL_EXPORTER_OTLP_ENDPOINT`).
- A Prometheus that scrapes every ServiceMonitor in the chart family.
- Grafana with the Triton, glass-to-event, and SLO dashboards.

Reference dashboards live in
`deploy/platform/observability/grafana/`. The Triton-specific alert
rules are at
[`deploy/platform/observability/prometheus/triton-rules.yaml`](./deploy/platform/observability/prometheus/triton-rules.yaml)
and reference the metric names documented in
[`docs/triton.md`](./docs/triton.md#metrics-and-slos).

---

## Tune the autonomy bounds

The agent has a fixed tool catalogue with risk tiers. Each tier-3
tool routes through `policy-gate-service`. Adopters typically:

- Add their own tier-0/1/2 tools by registering them in
  `services/agent-service/internal/tools/tools.go`.
- **Never** add a tier-3 tool that bypasses the gate. The refusal is
  in code, not in the prompt (ADR-0014/0017).
- Configure approver groups in `policy-gate-service` so the right
  humans see tier-3 requests.

---

## Run the compliance baseline

The template ships compliance evidence packages for SOC 2, EU AI Act,
GDPR DPIA, and a pen-test scope at
[`docs/compliance/`](./docs/compliance/). Adopters typically:

1. Walk the SOC 2 control mapping with their own auditor; check the
   evidence collection procedure works against your control set.
2. Run `compliance-evidence-service` against your live cluster to
   confirm evidence packets compose end-to-end.
3. Schedule a real pen-test against the scope in
   `docs/compliance/pentest-scope.md`.
4. Run the quarterly DR drill (`tools/dr-drills/run-quarterly.sh`).

The platform is *ready for* a real audit — adopters bring the audit
window.

---

## Adoption checklist

Before the first production deploy:

- [ ] Module path renamed; all `go.mod` files updated; `go build ./...`
      passes from every module root.
- [ ] Brand strings rewritten via `task template:rebrand`; landing page
      reflects your product.
- [ ] LICENSE retained; NOTICE updated with your copyright; CITATION
      updated.
- [ ] Triton model repository created; one model registered end-to-end;
      a synthetic stream fires a detection event in `event-service`.
- [ ] OIDC IdP wired into `api-gateway`; OPA policies authored.
- [ ] Vault / Secrets manager wired into ESO; every required secret
      populated.
- [ ] Prometheus + Grafana + OTel collector standing up; Triton
      dashboards green.
- [ ] Helm conformance test (`tools/conformance`) passes against your
      chart overlays.
- [ ] Cross-service integration smoke (`tools/integration`) passes
      against a real cluster.
- [ ] At least one chaos experiment from `deploy/chaos/` runs without
      tripping the SLO budget.
- [ ] DR drill (`tools/dr-drills/`) green at least once.
- [ ] SOC 2 / EU AI Act / GDPR docs reviewed against your real
      controls; gaps filed as tickets.

---

## Trademark and naming

"AegisVision" as a name and logo are held by the upstream maintainer.
You are free to fork under Apache 2.0 — the license explicitly grants
that — but please rename your downstream product. Continuing to call
a heavily customized fork "AegisVision" creates confusion for adopters
and is discouraged. Pick your own name; you don't owe anyone
attribution beyond the LICENSE + NOTICE requirements.

If you want to attribute the upstream template in your product
("powered by AegisVision" or "based on the AegisVision template")
that's welcome — please link to the upstream repository.

---

## Questions

Open a [GitHub Discussion](https://github.com/hoangsonww/AegisVision-Computer-Vision-System/discussions)
under "Adopters" or contact the maintainer:

- Email: <hoangson091104@gmail.com>
- GitHub: <https://github.com/hoangsonww>

---

## Related

- [`README.md`](./README.md) — the project overview.
- [`ARCHITECTURE.md`](./ARCHITECTURE.md) — the architecture deep dive.
- [`SETUP_GUIDE.md`](./SETUP_GUIDE.md) — all deploy paths.
- [`docs/triton.md`](./docs/triton.md) — the NVIDIA Triton operating manual.
- [`docs/adr/`](./docs/adr/) — the architectural rules to respect when
  customizing.
- [`GOVERNANCE.md`](./GOVERNANCE.md) — how the upstream is run.
- [`CONTRIBUTING.md`](./CONTRIBUTING.md) — how to upstream changes.
