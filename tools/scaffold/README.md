# tools/scaffold

> **New-service scaffolder.** Generates the canonical service layout.

```bash
go run ./cmd/scaffold --name my-thing-service
```

What it generates:

- `services/my-thing-service/cmd/my-thing-service/main.go`
- `services/my-thing-service/internal/{config,server,service,store}/`
- `services/my-thing-service/Dockerfile`
- `services/my-thing-service/go.mod`
- `deploy/helm/my-thing-service/` (Chart.yaml + canonical templates)
- Adds `./services/my-thing-service` to `go.work`
- Adds the chart to `deploy/argocd/applicationset.yaml`

You still need to:

1. Define the proto contract in `/proto/aegisvision/<domain>/v1/`.
2. Run `task proto`.
3. Wire `pkg/platform` in `main.go`.
4. Implement the handler.

See [`ARCHITECTURE.md`](../../ARCHITECTURE.md) and
[`services/README.md`](../../services/README.md) for the conventions
every service must follow.
