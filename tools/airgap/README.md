# Air-gapped bundle builder

Produces a single self-contained tarball that an operator can copy onto a
disconnected cluster and install without any internet egress. Per ADR-0027,
air-gapped support is day-one, not retrofit — every CI release builds the
bundle as a first-class artifact.

## What's in the bundle

```
aegisvision-airgap-<version>.tar.zst
├── manifest.json              # bundle manifest + cosign signatures
├── README.md                  # operator install guide
├── install.sh                 # one-shot bootstrap script (idempotent)
├── verify.sh                  # signature + checksum verification
├── images/                    # OCI layout — every service image
│   ├── api-gateway/
│   ├── pipeline-service/
│   └── ... (34 services)
├── charts/                    # tarballed Helm charts
│   ├── api-gateway-0.1.0.tgz
│   └── ... (37 charts)
├── manifests/                 # platform K8s manifests (Istio, Kyverno, ESO, SPIRE)
│   ├── platform/
│   └── argocd/
├── policies/                  # Kyverno + OPA policies
├── crds/                      # CRDs the platform depends on
├── sboms/                     # SPDX SBOMs per image
├── signatures/                # cosign bundle (signatures + attestations)
└── checksums.txt              # SHA-256 of every file
```

## Building

```sh
# From a CI runner with access to the registry:
./tools/airgap/build.sh --version=0.4.0 --registry=ghcr.io/hoangsonww
# Produces dist/aegisvision-airgap-0.4.0.tar.zst (~ 8 GiB typical).
```

For tests against the locally-built images:

```sh
./tools/airgap/build.sh --version=dev --registry=localhost:5000 --skip-sign
```

## Installing on the target cluster

```sh
# Extract on a workstation, copy via approved transfer mechanism (e.g.
# CD-ROM / one-way diode), and on the target host:
zstd -d aegisvision-airgap-0.4.0.tar.zst -o bundle.tar
tar -xf bundle.tar
cd aegisvision-airgap-0.4.0
./verify.sh        # checks every signature against the embedded cosign key
./install.sh \
  --registry=registry.dmz.internal \
  --kubeconfig=$HOME/.kube/airgap.cluster
```

The install script:

1. Loads every image from `images/` into the local registry via `crane push`.
2. Applies CRDs (Istio, Kyverno, ESO, SPIRE, ArgoCD).
3. Applies platform manifests (Istio config, Vault, SPIRE).
4. `helm install`s every chart in dependency order.
5. Waits for readiness on the API gateway.
6. Reports installed versions back as `installed.json`.

## Re-verification

`verify.sh` is also exposed as a Kyverno admission webhook config that
rejects any image whose digest is not in the bundle's manifest — defence
against pulling unsigned images post-install.

## Why this lives here, not in `release-please` automation

`release-please` cuts the version + CHANGELOG. The bundle is the *artifact*
of a release — built by a dedicated workflow (`release-airgap.yml`) only
when `release-please` lands a tagged release. This split keeps the
artifact build off the critical path of every PR but ties it durably to
versioned releases.
