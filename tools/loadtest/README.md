# Load tests

`k6` scripts that drive the public API with SLO-gated thresholds. CI invokes
these in a sidecar to gate PRs on:

- `p95(/v1/pipelines)`  < 200 ms
- `p99(/v1/pipelines)`  < 500 ms
- `p95(/v1/streams)`    < 200 ms
- `p99(/v1/streams)`    < 500 ms
- gateway error rate    < 1 %

## Run locally

```bash
brew install k6
k6 run tools/loadtest/api-gateway.js
```

Against a deployed gateway:

```bash
BASE_URL=https://api.example.aegisvision.io \
TENANT=t-prod-canary \
BEARER=$(aegis-cli token --tenant t-prod-canary) \
k6 run tools/loadtest/api-gateway.js
```
