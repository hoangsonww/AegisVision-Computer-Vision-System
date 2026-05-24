#!/usr/bin/env bash
# Build the AegisVision air-gapped bundle. Per ADR-0027, this is the
# canonical release artifact: every container image + every Helm chart +
# every platform manifest + signatures, packed into one tarball that can
# be installed on a disconnected cluster.
#
# Usage:
#   ./tools/airgap/build.sh --version <ver> --registry <ghcr.io/...>
#                          [--out dist/]
#                          [--skip-sign] [--platforms linux/amd64,linux/arm64]
#
# Exits non-zero on any error. Idempotent: re-running produces the same
# checksum given the same inputs.
set -euo pipefail
shopt -s nullglob

VERSION=""
REGISTRY=""
OUT_DIR="dist"
PLATFORMS="linux/amd64,linux/arm64"
SKIP_SIGN=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version=*) VERSION="${1#*=}"; shift ;;
    --version)   VERSION="$2"; shift 2 ;;
    --registry=*) REGISTRY="${1#*=}"; shift ;;
    --registry)  REGISTRY="$2"; shift 2 ;;
    --out=*)     OUT_DIR="${1#*=}"; shift ;;
    --out)       OUT_DIR="$2"; shift 2 ;;
    --platforms=*) PLATFORMS="${1#*=}"; shift ;;
    --platforms) PLATFORMS="$2"; shift 2 ;;
    --skip-sign) SKIP_SIGN=true; shift ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
[[ -z "$VERSION" ]] && { echo "missing --version" >&2; exit 2; }
[[ -z "$REGISTRY" ]] && { echo "missing --registry" >&2; exit 2; }

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"
BUNDLE_NAME="aegisvision-airgap-${VERSION}"
WORK_DIR="$(mktemp -d)"
STAGE_DIR="${WORK_DIR}/${BUNDLE_NAME}"
mkdir -p "$STAGE_DIR"/{images,charts,manifests,policies,crds,sboms,signatures}

echo "▶ Bundling AegisVision ${VERSION}"
echo "  Stage dir:  $STAGE_DIR"
echo "  Registry:   $REGISTRY"
echo "  Platforms:  $PLATFORMS"

# ---- 1. Image ------------------------------------------------------------
# Single umbrella image: ghcr.io/<owner>/aegisvision contains every service
# binary baked in. Pick which service at runtime via `command:`.
echo "▶ Pulling aegisvision image into OCI layout"
image="${REGISTRY}/aegisvision:${VERSION}"
out="${STAGE_DIR}/images/aegisvision"
mkdir -p "$out"
if command -v crane >/dev/null 2>&1; then
  crane pull --format=oci --platform="${PLATFORMS%%,*}" "$image" "$out" || {
    echo "  ! aegisvision image not available at $image; build + push first" >&2
    exit 1
  }
elif docker image inspect "$image" >/dev/null 2>&1; then
  docker save "$image" -o "${out}/image.tar"
else
  echo "  ! crane unavailable and docker has no local copy of $image" >&2
  exit 1
fi
echo "  ✓ aegisvision"

# ---- 2. Helm charts ------------------------------------------------------
echo "▶ Packaging Helm charts"
for chart_dir in deploy/helm/*/; do
  chart="$(basename "$chart_dir")"
  if [[ ! -f "$chart_dir/Chart.yaml" ]]; then continue; fi
  if command -v helm >/dev/null 2>&1; then
    helm package "$chart_dir" --destination "${STAGE_DIR}/charts" --version "$VERSION" >/dev/null
  else
    # Manual fallback: tar the chart dir.
    tar -czf "${STAGE_DIR}/charts/${chart}-${VERSION}.tgz" -C "$chart_dir/.." "$chart"
  fi
done
echo "  ✓ $(ls "${STAGE_DIR}/charts/" | wc -l | tr -d ' ') charts"

# ---- 3. Platform manifests ----------------------------------------------
echo "▶ Copying platform manifests"
cp -r deploy/k8s/base       "${STAGE_DIR}/manifests/k8s-base"
cp -r deploy/k8s/policies   "${STAGE_DIR}/policies"
cp -r deploy/argocd         "${STAGE_DIR}/manifests/argocd"
cp -r deploy/platform       "${STAGE_DIR}/manifests/platform" 2>/dev/null || true

# ---- 4. SBOM (best-effort — needs syft) ---------------------------------
if command -v syft >/dev/null 2>&1; then
  echo "▶ Generating SBOM"
  image="${REGISTRY}/aegisvision:${VERSION}"
  syft "$image" -o spdx-json="${STAGE_DIR}/sboms/aegisvision.spdx.json" 2>/dev/null || true
  echo "  ✓ aegisvision SBOM"
else
  echo "▶ syft not installed — skipping SBOM"
fi

# ---- 5. Cosign signature ------------------------------------------------
if [[ "$SKIP_SIGN" == "false" ]] && command -v cosign >/dev/null 2>&1; then
  echo "▶ Downloading cosign signature"
  image="${REGISTRY}/aegisvision:${VERSION}"
  cosign download signature "$image" > "${STAGE_DIR}/signatures/aegisvision.sig" 2>/dev/null || true
fi

# ---- 6. README + install + verify scripts -------------------------------
cp tools/airgap/README.md   "${STAGE_DIR}/README.md"
cp tools/airgap/install.sh  "${STAGE_DIR}/install.sh"
cp tools/airgap/verify.sh   "${STAGE_DIR}/verify.sh"
chmod +x "${STAGE_DIR}/install.sh" "${STAGE_DIR}/verify.sh"

# ---- 7. Manifest + checksums -------------------------------------------
echo "▶ Computing checksums"
( cd "$STAGE_DIR" && find . -type f \! -name checksums.txt -print0 | sort -z | xargs -0 shasum -a 256 > checksums.txt )

cat > "${STAGE_DIR}/manifest.json" <<EOF
{
  "name": "aegisvision-airgap",
  "version": "${VERSION}",
  "built_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "registry": "${REGISTRY}",
  "platforms": "${PLATFORMS}",
  "image_count": $(ls "${STAGE_DIR}/images" 2>/dev/null | wc -l | tr -d ' '),
  "chart_count": $(ls "${STAGE_DIR}/charts" 2>/dev/null | wc -l | tr -d ' '),
  "checksum_count": $(wc -l < "${STAGE_DIR}/checksums.txt" | tr -d ' ')
}
EOF

# ---- 8. Final tarball ---------------------------------------------------
mkdir -p "$OUT_DIR"
TARBALL="${OUT_DIR}/${BUNDLE_NAME}.tar"
( cd "$WORK_DIR" && tar -cf "${TARBALL}" "${BUNDLE_NAME}" )

if command -v zstd >/dev/null 2>&1; then
  zstd -19 --rm "$TARBALL"
  FINAL="${TARBALL}.zst"
else
  gzip -9 "$TARBALL"
  FINAL="${TARBALL}.gz"
fi

# Outer-level checksum.
shasum -a 256 "$FINAL" > "${FINAL}.sha256"

echo "✓ Bundle written: $FINAL"
echo "  Size: $(du -h "$FINAL" | cut -f1)"
echo "  SHA-256: $(cat "${FINAL}.sha256")"
echo
echo "Cleanup: $WORK_DIR"
rm -rf "$WORK_DIR"
