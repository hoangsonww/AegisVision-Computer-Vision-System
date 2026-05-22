# ADR-0027: Air-gapped bundle as a day-one CI artifact

**Status:** Accepted (2026-05-21)

## Context

The platform must run in environments without internet egress — defense
contractors, classified networks, regulated medical clusters. Every
*"we'll figure out the air-gap story later"* product I've watched ship
ends the same way: a frantic 6-month effort 18 months post-launch to
re-engineer the install path. That effort always exposes coupling we
didn't realize we had to public mirror infrastructure — Sigstore Rekor,
GitHub Packages, the Helm chart museum, the model registry's HuggingFace
mirror.

We treat air-gap as a release format from day one. Every CI release
produces both the conventional Helm + image artifacts AND a single
self-contained tarball.

## Decision

The air-gapped bundle is a versioned, signed tarball produced by
`tools/airgap/build.sh` and attached to every GitHub release by
`.github/workflows/release.yml`. Its contents are exhaustive:

- Every service container image (OCI layout via `crane`, multi-arch).
- Every Helm chart, packaged.
- Every K8s manifest the platform requires (Istio, Vault, ESO, SPIRE,
  ArgoCD, Kyverno).
- All CRDs.
- Every SBOM (SPDX-JSON via syft).
- Every cosign signature for the contents.
- A SHA-256 manifest covering every file in the tarball.
- A `manifest.json` describing the bundle (version, registry, platforms,
  built-at).
- An `install.sh` that idempotently pushes images to the target
  registry + applies manifests + helm-installs every chart.
- A `verify.sh` that validates the bundle's chain-of-custody using only
  the embedded public key + locally-installed cosign.

The bundle is signed as a single blob with cosign (keyless OIDC in CI,
key-based for releases off CI). The release attaches the `.cosign.bundle`
sidecar so operators can verify offline.

## Consequences

- **Every CI release ships a self-contained installable.** No "Phase 6
  retrofit" effort.
- **Bundle size is non-trivial.** Typical bundle: ~6–8 GiB compressed
  with zstd-19. Acceptable for a once-per-release artifact; not a hot
  path.
- **Image registries are interchangeable.** The bundle's `install.sh`
  pushes to whatever registry the target cluster reads from
  (`localhost:5000`, Harbor, ECR private). The platform's pod specs use
  `image.repository` templated via Helm values; the install script sets
  it.
- **Re-bundling without re-signing breaks chain-of-custody.** The
  `verify.sh` script warns but doesn't fail in this case — operators
  who legitimately re-bundle (e.g. to add a tenant-specific config)
  accept the residual risk explicitly.
- **Bundle reproducibility.** Given the same git SHA + Go module SUMs +
  signed images, the bundle's outer checksum is deterministic. This is
  table stakes for an audit-able supply chain.

## Rejected alternatives

- **Tarball of just images.** Rejected — operators then have to source
  charts and manifests separately, defeating the point.
- **Helm chart museum + `helm pull --untar`.** Rejected — works only
  with network egress to the museum.
- **A custom installer binary.** Rejected — shell scripts are auditable
  by hand; a Go binary is one more thing to sign and one more thing for
  the operator's security review to inspect.
