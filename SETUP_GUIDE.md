# AegisVision Setup Guide

> **End-to-end setup, from "just cloned the repo" to "10,000 streams in
> production on an air-gapped cluster."**

This guide is the canonical reference for deploying AegisVision. It covers
the three deployment paths (local dev, online cluster, air-gapped) plus
prerequisites, configuration, verification, and a focused troubleshooting
section. If something doesn't work, the troubleshooting section is at the
bottom — search it first.

---

## Table of contents

1. [Prerequisites](#prerequisites)
2. [Path A — Local dev (walking skeleton)](#path-a--local-dev-walking-skeleton)
3. [Path B — Local dev (full platform with dependencies)](#path-b--local-dev-full-platform-with-dependencies)
4. [Path C — Online cluster (ArgoCD + Helm)](#path-c--online-cluster-argocd--helm)
5. [Path D — Air-gapped install](#path-d--air-gapped-install)
6. [Path E — Edge install (k3s)](#path-e--edge-install-k3s)
7. [Post-install verification](#post-install-verification)
8. [Day-2 operations](#day-2-operations)
9. [Environment variables reference](#environment-variables-reference)
10. [Troubleshooting](#troubleshooting)
11. [Uninstall](#uninstall)

---

## Prerequisites

### Local dev minimum

| Tool | Version | Why |
| --- | --- | --- |
| Go | 1.26+ | All services. |
| Task | 3.36+ | Task runner. `brew install go-task`. |
| Buf | 1.34+ | Protobuf codegen. `brew install bufbuild/buf/buf`. |
| protoc-gen-go | 1.36+ | gRPC stubs. Installed by `task bootstrap`. |
| protoc-gen-go-grpc | 1.5+ | gRPC stubs. Installed by `task bootstrap`. |
| Docker | 25+ | Optional; for chart conformance images. |
| jq | any | Test helpers. |
| curl | any | Walking skeleton verification. |

```bash
brew install go-task bufbuild/buf/buf go jq
git clone https://github.com/hoangsonww/AegisVision-Computer-Vision-System.git
cd AegisVision-Computer-Vision-System
task bootstrap
```

`task bootstrap` does:

- Installs `protoc-gen-go` and `protoc-gen-go-grpc`.
- Runs `task proto` to generate protobuf bindings.
- Runs `task build` to verify all 49 modules compile.

If `task bootstrap` succeeds, you have a working developer environment.

### Online cluster prerequisites

| Component | Version | Notes |
| --- | --- | --- |
| Kubernetes | 1.27+ | EKS / GKE / AKS / Rancher / on-prem all work. |
| Istio | 1.22+ | Ambient mesh (no sidecars). |
| ArgoCD | 2.10+ | GitOps reconciliation. |
| Kyverno | 1.11+ | Admission policies, including cosign signature verify. |
| External Secrets Operator | 0.10+ | All credentials via Vault → ESO. |
| SPIRE | 1.9+ | Workload identity (SPIFFE IDs). |
| Vault | 1.17+ | Transit keys + secrets engine. Per-tenant transit keys. |
| Patroni Postgres | PG 16 | HA via Spilo / Zalando operator (or your own). |
| Redis Sentinel | 7+ | Rate limit + idempotency cache. |
| ClickHouse Operator | 0.23+ | Event firehose (3×2 replicated). |
| NATS JetStream | 2.10+ | Low-latency bus. 3 replicas. |
| Kafka | 3.7+ (KRaft) | Durable bus. 3 brokers. Strimzi / MSK. |
| NVIDIA GPU Operator | 24+ | If serving with Triton on real GPUs. |
| Triton Inference Server | 2.50+ | If serving real models. |

Cosign + Syft are required at the *build* environment but not on the cluster.

### Air-gapped prerequisites

| Component | Notes |
| --- | --- |
| Internal container registry | Harbor / ECR / GitLab Registry / generic OCI. |
| Internal Helm registry | Same registry usually; OCI Helm. |
| `cosign` | Verify bundle signatures. |
| `zstd` | Decompress the bundle. |

---

## Path A — Local dev (walking skeleton)

This is the **fastest** way to see the platform work end-to-end. Five
services + an embedded NATS, no databases, no GPUs, no external
dependencies, no Docker, no Kubernetes. From `task bootstrap` to a
real event arriving on an SSE feed in **under five minutes**.

```bash
# Terminal 1 — event-service embeds NATS in dev mode and prints the URL.
AEGIS_EMBED_NATS=true task run:event-service
# It logs: "embedded NATS listening on nats://127.0.0.1:54231"

# Export the URL for the rest:
export AEGIS_NATS_URL=nats://127.0.0.1:54231

# Terminal 2 — control plane for pipelines.
task run:pipeline-service

# Terminal 3 — control plane for streams.
AEGIS_NATS_URL=$AEGIS_NATS_URL task run:stream-manager

# Terminal 4 — the data plane operator runtime.
AEGIS_NATS_URL=$AEGIS_NATS_URL task run:dataplane-runner

# Terminal 5 — the public REST + SSE entrypoint + browser console.
AEGIS_STREAM_MANAGER_ADDR=localhost:9092 \
AEGIS_EVENT_SERVICE_URL=http://localhost:8090 \
AEGIS_CONSOLE_DIR=$(pwd)/services/api-gateway/console \
  task run:api-gateway

# Terminal 6 — subscribe to the SSE feed for tenant t-demo and stream
# stream-dock-1. Keep this running.
curl -N -H 'X-Tenant-Id: t-demo' \
  'http://localhost:8080/v1/events:stream?stream_id=stream-dock-1'

# Terminal 7 — create the stream.
curl -X POST -H 'X-Tenant-Id: t-demo' -H 'Idempotency-Key: 1' \
  -H 'Content-Type: application/json' \
  -d '{"name":"dock-1","project_id":"p1","protocol":"file","url":"file:///x","pipeline_id":"p-walking"}' \
  http://localhost:8080/v1/streams
```

Within **~5 seconds**, Terminal 6 prints an SSE event like:

```
event: detection
data: {"kind":"KIND_DWELL","severity":"SEVERITY_HIGH","stream_id":"stream-dock-1",
       "tenant_id":"t-demo","payload":{"class":"person","dwell_ms":5000},
       "trace_id":"...","ts":"..."}
```

**That's the platform working end-to-end.** REST → gRPC → bus → operator
DAG → rule → bus → SSE.

For the **walking-skeleton minimal browser console** (vanilla HTML+JS,
the 5-service demo UI), open <http://localhost:8080/console/> after
Terminal 5 starts. For the **production Next.js console** with every
page wired, see Path B's console section below.

---

## Path B — Local dev (full platform with dependencies)

If you want to exercise the *whole* stack locally (intelligence tier
+ adaptive autonomy + operations & compliance services included),
you'll need real backing services. The fastest way is `docker-compose`:

```bash
# Spin up Postgres, Redis, NATS, MinIO, ClickHouse single-node.
cd deploy/dev
docker compose up -d

# Apply migrations.
task migrate

# Start the full stack (one terminal per service):
task run:llm-gateway          # talks to a configured upstream
task run:agent-service        # uses llm-gateway + policy-gate-service
task run:policy-gate-service
task run:knowledge-service    # auto-ingests docs/ on first run
task run:nlq-service
task run:active-learning-service
task run:canary-controller
task run:shadow-inference-service
task run:drift-detection-service
task run:slo-watchdog
task run:prefetch-service
task run:autonomy-orchestrator
task run:compliance-evidence-service
task run:cost-accounting
task run:metering-service
task run:inference-router     # connects to a Triton URL or uses synthetic
task run:model-registry
task run:dataset-service
task run:annotation-service
task run:training-orchestrator
task run:media-service
task run:tenant-service
task run:audit-service
task run:notification-service
task run:realtime-hub
task run:rule-engine
task run:gpu-scheduler
task run:edge-gateway
task run:semantic-search
```

You'll need to set `AEGIS_LLM_BACKEND_URL` for `llm-gateway` to point at
your model backend (vLLM, hosted OpenAI, Anthropic via the OpenAI-shim,
Bedrock with shim, or Triton+TRT-LLM). In *dev* mode the fake echo
backend works out of the box.

```bash
# Dev — uses fake echo client.
AEGIS_ENV=dev task run:llm-gateway

# Prod-shape — refuses to start without a real backend.
AEGIS_ENV=production \
AEGIS_LLM_BACKEND_URL=https://vllm.internal/v1 \
AEGIS_LLM_BACKEND_API_KEY_SECRET=/run/secrets/llm-api-key \
  task run:llm-gateway
```

This is honest by design — production-shape config refuses unsafe
defaults.

### Production Next.js console (browser UI)

The walking-skeleton HTML console at `/console/` is the minimal vanilla
HTML+JS demo for the 5-service skeleton. The **production UI** lives in
[`services/console/`](./services/console) and exposes every public REST
endpoint as a polished page.

```bash
cd services/console
npm install                              # one-time
NEXT_PUBLIC_AEGIS_API_BASE=http://localhost:8080 npm run dev
# → http://localhost:8090
```

<p align="center">
  <a href="./docs/console.md">
    <img src="./docs/img/console.png" alt="AegisVision operator console — Mission control" width="820"/>
  </a>
  <br/>
  <sub><em>What http://localhost:8090 looks like once <code>api-gateway</code> is up — Mission control with live SSE, tier-3 gate inbox, canary / drift / SLO triptych. Full doc at <a href="./docs/console.md"><code>docs/console.md</code></a>.</em></sub>
</p>

Open <http://localhost:8090>. Top-right has a tenant switcher — use
`t-demo` to match the walking-skeleton's events. The console covers
**33 routes**: dashboard with live SSE feed, streams + pipelines + models
+ datasets + annotations + training + media + rules CRUD, events search +
live, agent chat with citations + tier-3 gate inbox, canary decision
board, drift heatmap, SLO burn-rate, prefetch grid, knowledge RAG, NLQ,
active-learning queue, semantic search, tenants + projects + members,
cost dashboards, compliance evidence bundles, append-only audit viewer.

---

## Path C — Online cluster (ArgoCD + Helm)

This is how you deploy to a real Kubernetes cluster.

### Step 1 — bootstrap platform tier

```bash
# Namespaces, priority classes, baseline policies.
kubectl apply -f deploy/k8s/base
kubectl apply -f deploy/k8s/policies     # Kyverno admission
kubectl apply -f deploy/k8s/quotas       # per-namespace ResourceQuotas
```

Install platform tier (use your own Helm charts or our manifests):

```bash
# Istio Ambient
istioctl install -f deploy/platform/istio/profile.yaml

# Cert-manager, SPIRE, ESO, Vault — apply your usual way.
kubectl apply -f deploy/platform/spire/
kubectl apply -f deploy/platform/external-secrets/
kubectl apply -f deploy/platform/vault/

# Observability (OTel collector, Prometheus, Loki, Tempo, Grafana)
kubectl apply -f deploy/platform/observability/
```

### Step 2 — bootstrap data tier

```bash
# Patroni Postgres
kubectl apply -f deploy/platform/data/postgres/

# Redis Sentinel
kubectl apply -f deploy/platform/data/redis/

# ClickHouse Operator + cluster
kubectl apply -f deploy/platform/data/clickhouse/

# NATS JetStream + Kafka
kubectl apply -f deploy/platform/data/nats/
kubectl apply -f deploy/platform/data/kafka/

# Wait for everything to be Ready.
kubectl get pods -n aegis-data -w
```

### Step 3 — bootstrap ArgoCD + ApplicationSet

```bash
# Install ArgoCD (your usual way).
kubectl apply -f deploy/platform/argocd/

# Apply the AegisVision ApplicationSet.
kubectl apply -f deploy/argocd/applicationset.yaml
```

ArgoCD will now reconcile every chart in `deploy/helm/`. First sync
takes 5–10 minutes.

### Step 4 — wire secrets

Each service that needs credentials (Postgres DSN, Vault token, LLM
backend API key) has an `ExternalSecret` template referencing a Vault
path. Populate Vault:

```bash
vault kv put secret/aegisvision/postgres/api-gateway dsn=...
vault kv put secret/aegisvision/llm-gateway/backend api_key=...
vault kv put secret/aegisvision/cursor key=$(openssl rand -base64 48)
# (… see deploy/platform/external-secrets/templates/ for the full list)
```

### Step 5 — verify

```bash
# All deployments Ready.
kubectl get deploy -n aegis-system

# All charts conformance-clean.
(cd tools/conformance && go test -count=1 ./...)

# Smoke test API.
kubectl port-forward -n aegis-system svc/api-gateway 8080:80
curl http://localhost:8080/v1/health
```

### Step 6 — load real models

```bash
# Upload your model to Triton's S3 (or local) model store.
kubectl -n aegis-gpu exec -ti triton-0 -- tritonserver --model-control-mode=explicit --load-model my-model

# Register the model in model-registry.
curl -X POST -H 'X-Tenant-Id: t-1' -H 'Content-Type: application/json' \
  -d '{"name":"my-model","triton_name":"my-model","version":"v1"}' \
  http://localhost:8080/v1/models
```

---

## Path D — Air-gapped install

For DMZ / classified / regulated environments. **No cluster needs internet
access.**

### Step 1 — build the bundle (on a CI runner with internet)

```bash
./tools/airgap/build.sh --version 0.5.0 \
  --registry-out ghcr.io/hoangsonww \
  --output dist/aegisvision-airgap-0.5.0.tar.zst
```

The bundle contains:

- All container images (one per service), tagged + signed (cosign keyless).
- All Helm charts (`deploy/helm/`).
- All Kubernetes manifests (`deploy/k8s/`, `deploy/platform/`, `deploy/argocd/`).
- SBOM for every image (Syft, SPDX format).
- SLSA v1 provenance for the bundle.
- Cosign signatures + transparency log entries.
- The `install.sh` and `verify.sh` scripts.

Bundle size: ~6–8 GiB (depending on Triton + model size).

### Step 2 — transfer

Move `dist/aegisvision-airgap-0.5.0.tar.zst` to your target environment
via your approved transfer mechanism (e.g. SneakerNet, secure data
diode, registered USB).

### Step 3 — verify on target

```bash
zstd -d aegisvision-airgap-0.5.0.tar.zst -o bundle.tar
tar -xf bundle.tar
cd aegisvision-airgap-0.5.0
./verify.sh
```

`verify.sh` checks:

- bundle checksum.
- cosign signature on the bundle.
- SLSA provenance is valid + from the expected workflow.
- Every per-image signature.

### Step 4 — install on target

```bash
./install.sh \
  --registry=registry.dmz.internal \
  --kubeconfig=$HOME/.kube/airgap.cluster \
  --vault=https://vault.dmz.internal \
  --postgres-dsn=postgres://... \
  --skip-platform-tier=false
```

`install.sh` does:

- Pushes every image to your internal registry under the namespace you
  specify.
- Loads every Helm chart into ArgoCD (or applies via `helm` if no
  ArgoCD).
- Applies platform tier (Istio + ESO + SPIRE + Vault + observability),
  unless `--skip-platform-tier` is set.
- Configures Kyverno admission policy to verify images against the
  bundled cosign key.

Total install time on a 6-node cluster: **~30 minutes** including
platform-tier bootstrap.

### Step 5 — verify air-gapped install

```bash
./install.sh --verify-only
# (or:)
(cd tools/conformance && go test -count=1 ./...)
```

---

## Path E — Edge install (k3s)

A reduced-operator profile for on-prem / vehicle / mobile / aerospace.

```bash
# On the edge box (e.g. NVIDIA Jetson AGX Orin):
curl -sfL https://get.k3s.io | sh -

# Apply the edge profile.
helm install aegis-edge deploy/helm/edge-profile -n aegis-edge --create-namespace

# Wire outbox sync to the core cluster.
kubectl -n aegis-edge create secret generic core-sync \
  --from-literal=core-grpc=core.example.com:443 \
  --from-literal=core-token=$(cat /run/secrets/edge-token)
```

The edge profile runs:

- `stream-manager` (reduced).
- `dataplane-runner` (full).
- `event-service` (with outbox-to-core).
- `inference-router` (with local Triton).
- `edge-gateway` (does the outbox sync to core).

LLM / agent / canary / drift remain on the core cluster.

---

## Post-install verification

Always run these after a fresh install.

```bash
# 1) Conformance test on every Helm chart.
(cd tools/conformance && go test -count=1 ./...)

# 2) Integration smoke (against any cluster: local kind, dev, staging, prod).
(cd tools/integration && go test -race -count=1 ./...)

# 3) Walking-skeleton in-cluster.
kubectl port-forward -n aegis-system svc/api-gateway 8080:80 &
curl -H 'X-Tenant-Id: t-demo' http://localhost:8080/v1/health
curl -H 'X-Tenant-Id: t-demo' http://localhost:8080/v1/streams

# 4) SLO load test.
k6 run tools/loadtest/slo-gate.js
# Fails the build if p95 > target.

# 5) Optional — chaos drill.
kubectl apply -f deploy/chaos/nats-pod-kill.yaml
# (… observe NATS recovers within ~10 s)
```

---

## Day-2 operations

### Backups

```bash
# Daily Postgres backup (already wired via WAL-G — verify).
kubectl -n aegis-data exec -ti postgres-0 -- wal-g backup-list

# Daily ClickHouse backup (already wired via clickhouse-backup — verify).
kubectl -n aegis-data exec -ti clickhouse-0-0 -- clickhouse-backup list
```

### Quarterly DR drills

```bash
./tools/dr-drills/run-quarterly.sh
```

This runs every restore script + chaos experiment in series, generating
a signed attestation in `dist/dr-drills/<date>.json`. The compliance
auditor accepts this as evidence for SOC 2 CC8.1.

### Tenant onboarding

```bash
# Create the tenant.
curl -X POST -H 'Authorization: Bearer $TOKEN' \
  -H 'Content-Type: application/json' \
  -d '{"name":"acme","plan":"enterprise"}' \
  https://api.aegisvision.example.com/v1/tenants

# Vault transit key + KMS key + per-tenant namespace are created automatically
# by tenant-service (see services/tenant-service for details).
```

### Tenant offboarding (crypto-shredding)

```bash
# Destroying the tenant's Vault transit key renders all encrypted bytes
# (current + backups + claim-check store + ClickHouse + Postgres) unreadable.
vault delete transit/keys/aegis-tenant-acme

# Mark the tenant deleted in metadata.
curl -X DELETE -H 'Authorization: Bearer $TOKEN' \
  https://api.aegisvision.example.com/v1/tenants/acme
```

### Model promotion (gated)

```bash
# Submit a canary plan.
curl -X POST -H 'X-Tenant-Id: t-1' -H 'Content-Type: application/json' \
  -d '{
    "candidate_model":"my-model-v2",
    "baseline_model":"my-model-v1",
    "traffic_pct": 5,
    "min_samples": 1000,
    "wilson_alpha": 0.05
  }' \
  https://api.aegisvision.example.com/v1/canary-plans

# Canary controller decides promote/rollback automatically.
# If promote → policy-gate-service requests human approval.
# Approver hits the UI; approval routes back via gate.resolved.<id>;
# model-registry flips traffic to 100%.
```

---

## Environment variables reference

### Universal (every service via `pkg/platform/config`)

| Var | Purpose | Default |
| --- | --- | --- |
| `AEGIS_ENV` | `dev` / `test` / `staging` / `production`. Production-shape services refuse to start with unsafe defaults. | `dev` |
| `AEGIS_LOG_LEVEL` | `debug` / `info` / `warn` / `error`. | `info` |
| `AEGIS_OTEL_ENDPOINT` | OTLP collector. | empty (no traces) |
| `AEGIS_OTEL_SAMPLE_RATE` | Trace sample fraction (0..1). | `0.01` |
| `AEGIS_PORT_HTTP` | HTTP listen port. | per-service default |
| `AEGIS_PORT_GRPC` | gRPC listen port. | per-service default |
| `AEGIS_PORT_METRICS` | Prom metrics port. | per-service default |
| `AEGIS_TENANT_REQUIRED` | Reject requests with no tenant. | `true` outside dev |

### api-gateway

| Var | Purpose | Required in prod |
| --- | --- | --- |
| `AEGIS_OPA_ENDPOINT` | OPA sidecar AuthZ. | **yes** (panics otherwise) |
| `AEGIS_JWKS_URL` | IdP JWKS endpoint. | **yes** |
| `AEGIS_JWT_ISSUER` | JWT `iss` claim. | **yes** |
| `AEGIS_CURSOR_KEY` | HMAC key for cursors (≥32 bytes). | **yes** |
| `AEGIS_STREAM_MANAGER_ADDR` | gRPC target for stream-manager. | **yes** |
| `AEGIS_EVENT_SERVICE_URL` | event-service base URL (for SSE proxy). | **yes** |
| `AEGIS_CONSOLE_DIR` | Path to console assets. | optional |

### llm-gateway

| Var | Purpose | Required in prod |
| --- | --- | --- |
| `AEGIS_LLM_BACKEND_URL` | OpenAI-compatible upstream. | **yes** (panics otherwise) |
| `AEGIS_LLM_BACKEND_API_KEY_SECRET` | File path to API key. | **yes** |
| `AEGIS_LLM_REFUSAL_THRESHOLD` | Safety score (0..1). | optional, default `0.8` |
| `AEGIS_LLM_PII_REDACT` | `true` / `false`. | default `true` |
| `AEGIS_LLM_RATE_PER_TENANT_RPM` | Per-tenant RPM cap. | default `60` |

### agent-service

| Var | Purpose | Required in prod |
| --- | --- | --- |
| `AEGIS_LLM_GATEWAY_URL` | llm-gateway base URL. | **yes** |
| `AEGIS_POLICY_GATE_URL` | policy-gate-service base URL. | **yes** |
| `AEGIS_KNOWLEDGE_URL` | knowledge-service base URL. | **yes** |
| `AEGIS_NATS_URL` | For `gate.resolved.>` auto-resume. | **yes** |

### inference-router

| Var | Purpose | Required in prod |
| --- | --- | --- |
| `AEGIS_TRITON_URL` | Triton inference server URL. | **yes** |
| `AEGIS_NATS_URL` | For `inference.*` publishes. | **yes** |
| `AEGIS_GPU_SCHEDULER_URL` | GPU reservation ledger. | **yes** |

### bus consumers / producers (all)

| Var | Purpose |
| --- | --- |
| `AEGIS_NATS_URL` | NATS connection. |
| `AEGIS_KAFKA_BROKERS` | Kafka brokers (comma-separated). |
| `AEGIS_BUS_MODE` | `nats` / `kafka` / `dual`. Default `dual` in prod. |

### console (Next.js)

| Var | Purpose | Required |
| --- | --- | --- |
| `NEXT_PUBLIC_AEGIS_API_BASE` | api-gateway base URL the browser hits. Defaults to `http://localhost:8080`. Set at *build time* — Next.js inlines `NEXT_PUBLIC_*` into the bundle. | yes for prod |
| `NODE_ENV` | `production` in prod (the standalone server reads this). | yes |
| `PORT` | Listen port. Default `8090`. | no |
| `HOSTNAME` | Bind host. Default `0.0.0.0`. | no |

---

## Troubleshooting

### `task bootstrap` fails on `protoc-gen-go: command not found`

```bash
# Ensure GOPATH/bin is on PATH.
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.zshrc
source ~/.zshrc
task bootstrap
```

### Walking skeleton: SSE feed never receives an event

1. Check `event-service` logs — did it consume `events.v1`?
2. Check `dataplane-runner` logs — did it publish `events.v1`?
3. Check `stream-manager` logs — did it dispatch `operator.control`?
4. Check `AEGIS_NATS_URL` is the *same* across all five terminals.

```bash
# Print all bus subjects with consumers + redelivery counts.
nats stream report -s $AEGIS_NATS_URL
```

### `api-gateway` panics on startup with `auth.AllowAll must not be used outside dev/test`

`AEGIS_OPA_ENDPOINT` is unset. The platform refuses unsafe defaults in
production. Set it (or set `AEGIS_ENV=dev`).

### `llm-gateway` panics with `AEGIS_LLM_BACKEND_URL is required`

Same pattern — refuse unsafe defaults. Set it.

### `agent-service` agent waits forever on a tier-3 tool

It's waiting for `gate.resolved.<request-id>`. Check:

```bash
# Is policy-gate-service publishing the resolution?
nats sub -s $AEGIS_NATS_URL 'gate.resolved.>'

# Did the human actually approve?
curl http://localhost:8410/v1/gates
```

### ArgoCD shows `OutOfSync` on every chart

```bash
# Most common cause: ESO can't reach Vault.
kubectl -n aegis-platform logs deploy/external-secrets

# Re-sync everything.
argocd app sync aegis-platform --prune
```

### `inference-router` returns 503

`gpu-scheduler` has no reservations. Either:

```bash
# Free up a MIG slice:
kubectl -n aegis-gpu describe gpunode <node>

# Or, in dev, force the synthetic detector:
AEGIS_TRITON_URL=synthetic://localhost task run:inference-router
```

### Helm conformance test fails for a chart

The chart is missing one of: PeerAuthentication, AuthorizationPolicy,
NetworkPolicy, ServiceMonitor, HPA, PDB, ServiceAccount. The test
output names which.

### Air-gapped install fails: cosign signature mismatch

The bundle was tampered with in transit, or the public key in your
cluster doesn't match the build's keyless signer. Re-download + re-run
`./verify.sh`.

### `task test` fails on `pkg/platform` race detector

```bash
# This is usually parallel build cache contention; retry once.
task test
```

---

## Uninstall

### Local dev

Ctrl-C all five terminals. No state is persisted in dev mode.

### Online cluster

```bash
# Remove the ApplicationSet → ArgoCD removes every chart.
kubectl delete -f deploy/argocd/applicationset.yaml

# Remove platform tier.
kubectl delete -f deploy/platform/

# Remove namespaces.
kubectl delete -f deploy/k8s/base
```

### Air-gapped

```bash
./install.sh --uninstall --kubeconfig=$HOME/.kube/airgap.cluster
```

This removes the ApplicationSet + every chart but leaves your Vault
+ Postgres + ClickHouse + NATS + Kafka intact. To remove those, delete
them via your usual infrastructure-as-code workflow.

---

## Where next

- [`README.md`](./README.md) — what AegisVision is.
- [`ARCHITECTURE.md`](./ARCHITECTURE.md) — how it's structured.
- [`docs/runbooks/`](./docs/runbooks/) — operating it.
- [`docs/compliance/`](./docs/compliance/) — evidence for SOC 2 / EU AI
  Act / GDPR auditors.
- Each service's `README.md` — operate that service in detail.
