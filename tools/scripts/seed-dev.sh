#!/usr/bin/env bash
# seed-dev.sh — populate a freshly-bootstrapped AegisVision deployment
# with a demo tenant, project, pipeline, and stream so adopters can see
# the platform working end-to-end.
#
# Targets either the local walking-skeleton (default — http://localhost:8080)
# or a remote cluster (set AEGIS_API_BASE).
#
# Usage:
#   ./tools/scripts/seed-dev.sh                            # local walking-skeleton
#   AEGIS_API_BASE=https://api.example.com ./seed-dev.sh   # remote cluster
#
# Idempotent — re-running is safe because every create call includes a
# stable Idempotency-Key.

set -eu

API_BASE="${AEGIS_API_BASE:-http://localhost:8080}"
TENANT_ID="${AEGIS_DEMO_TENANT:-t-demo}"

curl_json() {
  curl -sS --fail \
    -H "X-Tenant-Id: $TENANT_ID" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: seed-$2" \
    -X "$1" \
    "$API_BASE$3" \
    --data "$4"
}

echo "Seeding AegisVision demo at $API_BASE (tenant=$TENANT_ID)"
echo "----------------------------------------------------------"

echo "[1/4] Creating demo pipeline..."
curl_json POST pipeline-demo /v1/pipelines '{
  "name": "demo-dwell",
  "project_id": "p1",
  "spec": {
    "operators": [
      {"name": "ingest",  "kind": "ingest"},
      {"name": "sampler", "kind": "sampler", "config": {"fps": 5}},
      {"name": "detect",  "kind": "detect",  "config": {"model": "synthetic-detector", "version": "1"}},
      {"name": "tracker", "kind": "tracker", "config": {"algorithm": "iou-greedy"}},
      {"name": "rule",    "kind": "rule",    "config": {"predicate": "dwell", "class": "person", "min_ms": 5000}},
      {"name": "emit",    "kind": "emit"}
    ]
  }
}' >/dev/null || true
echo "  ok"

echo "[2/4] Registering demo model..."
curl_json POST model-demo /v1/models '{
  "name": "synthetic-detector",
  "triton_name": "synthetic-detector",
  "version": "1",
  "reference_distribution": {"person": 1.0}
}' >/dev/null || true
echo "  ok"

echo "[3/4] Creating demo stream..."
curl_json POST stream-dock-1 /v1/streams '{
  "name": "dock-1",
  "project_id": "p1",
  "protocol": "file",
  "url": "synthetic://noop",
  "pipeline_id": "p-walking"
}' >/dev/null || true
echo "  ok"

echo "[4/4] Subscribing to the SSE feed for 10 seconds..."
echo "  (you should see at least one KIND_DWELL event)"
echo
timeout 10 curl -sN \
  -H "X-Tenant-Id: $TENANT_ID" \
  "$API_BASE/v1/events:stream?stream_id=stream-dock-1" || true

echo
echo "Demo seeded. Browse http://localhost:8090/ (production console) or"
echo "http://localhost:8080/console/ (walking-skeleton console) to see"
echo "the resources you just created."
