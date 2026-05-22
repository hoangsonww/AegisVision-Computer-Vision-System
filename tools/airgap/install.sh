#!/usr/bin/env bash
# Install an extracted AegisVision air-gapped bundle onto the target cluster.
# Per ADR-0027, this script is idempotent — re-running on a partially-installed
# cluster only applies what's missing.
#
# Usage:
#   ./install.sh --registry <internal-registry> --kubeconfig <path>
#               [--skip-images] [--skip-charts] [--dry-run]
#
# Expects to be run from the extracted bundle root (the directory containing
# manifest.json).
set -euo pipefail

REGISTRY=""
KUBECONFIG_PATH="${KUBECONFIG:-$HOME/.kube/config}"
SKIP_IMAGES=false
SKIP_CHARTS=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --registry=*) REGISTRY="${1#*=}"; shift ;;
    --registry)   REGISTRY="$2"; shift 2 ;;
    --kubeconfig=*) KUBECONFIG_PATH="${1#*=}"; shift ;;
    --kubeconfig) KUBECONFIG_PATH="$2"; shift 2 ;;
    --skip-images) SKIP_IMAGES=true; shift ;;
    --skip-charts) SKIP_CHARTS=true; shift ;;
    --dry-run)     DRY_RUN=true; shift ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done
[[ -z "$REGISTRY" ]] && { echo "missing --registry" >&2; exit 2; }
[[ ! -f "manifest.json" ]] && { echo "no manifest.json — run from bundle root" >&2; exit 2; }
export KUBECONFIG="$KUBECONFIG_PATH"

VERSION="$(grep -oE '"version": *"[^"]+"' manifest.json | head -1 | cut -d'"' -f4)"
echo "▶ Installing AegisVision $VERSION → $REGISTRY"

run() {
  if [[ "$DRY_RUN" == "true" ]]; then echo "  [dry-run] $*"; else "$@"; fi
}

# ---- 1. Verify bundle integrity ----------------------------------------
echo "▶ Verifying bundle"
./verify.sh

# ---- 2. Push images into the in-cluster / DMZ registry -----------------
if [[ "$SKIP_IMAGES" == "false" ]]; then
  echo "▶ Loading images into $REGISTRY"
  for img_dir in images/*/; do
    svc="$(basename "$img_dir")"
    if [[ -d "$img_dir/blobs" ]]; then
      # OCI layout — push via crane.
      run crane push "$img_dir" "${REGISTRY}/${svc}:${VERSION}"
    elif [[ -f "$img_dir/image.tar" ]]; then
      # docker save tar — load + retag + push.
      run docker load -i "$img_dir/image.tar"
      run docker tag "${REGISTRY}/${svc}:${VERSION}" "${REGISTRY}/${svc}:${VERSION}"
      run docker push "${REGISTRY}/${svc}:${VERSION}"
    fi
    echo "  ✓ $svc"
  done
fi

# ---- 3. Apply CRDs ------------------------------------------------------
if [[ -d crds ]]; then
  echo "▶ Applying CRDs"
  run kubectl apply -f crds/
fi

# ---- 4. Apply platform manifests (Istio, Vault, SPIRE, ESO, ArgoCD) ----
if [[ -d manifests/platform ]]; then
  echo "▶ Applying platform manifests"
  run kubectl apply -f manifests/k8s-base/
  run kubectl apply -f policies/
  run kubectl apply -R -f manifests/platform/
fi

# ---- 5. helm install each service chart --------------------------------
if [[ "$SKIP_CHARTS" == "false" ]]; then
  echo "▶ Installing Helm charts"
  for chart in charts/*.tgz; do
    name="$(basename "$chart" | sed -E 's/-[0-9].*$//')"
    # nats and edge-profile are platform infrastructure charts; install them first.
    if [[ "$name" == "nats" || "$name" == "edge-profile" ]]; then continue; fi
    run helm upgrade --install --namespace aegis-system --create-namespace \
      --set image.repository="${REGISTRY}/${name}" \
      --set image.tag="$VERSION" \
      "$name" "$chart"
  done
fi

# ---- 6. Wait for readiness ---------------------------------------------
if [[ "$DRY_RUN" == "false" ]]; then
  echo "▶ Waiting for api-gateway readiness"
  kubectl -n aegis-system rollout status deploy/api-gateway --timeout=5m
fi

# ---- 7. Record installation footprint -----------------------------------
mkdir -p installed
kubectl -n aegis-system get deploy -o jsonpath='{range .items[*]}{.metadata.name}={.spec.template.spec.containers[0].image}{"\n"}{end}' > installed/services.txt 2>/dev/null || true
cat > installed/run.json <<EOF
{
  "version": "$VERSION",
  "registry": "$REGISTRY",
  "installed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "kubeconfig": "$KUBECONFIG_PATH"
}
EOF

echo "✓ install complete — see installed/run.json"
