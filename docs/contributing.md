# Contributing

> **How to add a service, change a contract, ship a chart.**

This is a 35-service monorepo. The path of least resistance is to
follow the conventions; the conventions exist because each one
solves a problem that bit a prior CV platform.

---

## Before opening a PR

1. Read [`CLAUDE.md`](../CLAUDE.md), [`ARCHITECTURE.md`](../ARCHITECTURE.md),
   and the relevant ADR(s) in [`docs/adr/`](./adr/).
2. Search existing services for a similar shape — copy that.
3. Ask in the architecture WG if the change is load-bearing.

---

## Adding a service

```bash
./tools/scaffold/new-service.sh my-thing-service
```

What this generates:

- `services/my-thing-service/{cmd,internal/{config,server,service,store}}/`
- `services/my-thing-service/Dockerfile`
- `services/my-thing-service/go.mod`
- `deploy/helm/my-thing-service/` (Chart.yaml + canonical templates).
- Adds the module to `go.work`.
- Adds the chart to `deploy/argocd/applicationset.yaml`.

Then:

1. Define the API in `/proto` first. Run `task proto`.
2. Wire `pkg/platform` (logging / OTel / metrics / health / shutdown).
3. Implement handlers in `internal/server`.
4. Persist via `internal/store` (Postgres + SQLite for dev).
5. Write unit tests (`-race` clean).
6. Verify conformance: `(cd tools/conformance && go test ./...)`.
7. Verify integration: `(cd tools/integration && go test ./...)`.
8. Write the service `README.md`.

---

## Changing a protobuf contract

Contracts are the most-CODEOWNERS-reviewed part of the repo.

1. Open a PR that contains **only** the proto change.
2. CODEOWNERS triggers architecture-WG review.
3. Merged → regenerate stubs (`task proto`).
4. Then ship the handler.
5. Then ship the client.

Don't merge handler + contract in the same PR.

Buf lints (`buf lint`) and breaking-change check (`buf breaking`)
both run in CI.

---

## Shipping a Helm chart

Every chart must include:

- `Deployment` (or `StatefulSet`)
- `Service`
- `ServiceAccount`
- `AuthorizationPolicy` (Istio, ALLOW list)
- `PeerAuthentication` (mTLS STRICT)
- `NetworkPolicy` (default-deny)
- `ServiceMonitor`
- `HorizontalPodAutoscaler`
- `PodDisruptionBudget`

Plus a non-root container, requests + limits, probes, OPA policy
where applicable. The conformance test asserts all of this.

---

## Adding a bus subject

When you add a new bus subject (e.g. `my.new.subject.v1`):

1. Update `tools/integration/integration_test.go` `TestSmoke_PlatformBusSubjects`
   with the new subject + its consumer.
2. If the subject is wildcard-matched, add to `TestSmoke_PlatformWildcardSubjects`.
3. Update [`ARCHITECTURE.md`](../ARCHITECTURE.md) §"Bus architecture".
4. Update the affected service's README.

Missing any of these and CI fails on integration.

---

## Adding an LLM-calling site

Three rules:

1. **You may not.** Use `llm-gateway`. ADR-0018.
2. If you really must (rare, must be ADR-approved), use `pkg/llm` —
   never roll your own client.
3. Apply the safety layer (`pkg/llm/safety`). Never bypass.

---

## Adding an agent tool

1. Define the tool schema in `services/agent-service/internal/tools/`.
2. **Set the tier explicitly.** Tier 3 means a gate. The reviewer
   verifies the tier matches the action.
3. Add a unit test that asserts:
   - Tier 0/1 tools execute without a gate.
   - Tier 3 tools refuse without a gate.
4. Update [`docs/agents.md`](./agents.md) §"The four risk tiers".

---

## Adding a chart that needs Vault

1. Add the ExternalSecret template under `deploy/helm/<svc>/templates/`.
2. Document the Vault path in the service's `README.md`.
3. Document the env var (e.g. `AEGIS_FOO_DSN_FILE`) in [`SETUP_GUIDE.md`](../SETUP_GUIDE.md).

---

## Adding a metric

- Name it `aegis_<service>_<metric>_<unit>`.
- Always include `service` as a label.
- **Never put unbounded user input in a label.**
- Use the canonical histogram buckets from `pkg/platform/metrics`.

---

## Adding an ADR

1. Copy `docs/adr/_template.md` (if it exists) or model on an existing ADR.
2. Number sequentially.
3. Title: `<verb> <noun>`.
4. Sections: Context, Decision, Consequences, Status.
5. Update [`README.md`](../README.md) and [`ARCHITECTURE.md`](../ARCHITECTURE.md) tables.

---

## Extending the console

The Next.js console at [`services/console/`](../services/console)
follows a simple recipe:

1. **Define types** in `src/lib/types.ts`.
2. **Add API method** in `src/lib/api.ts` (use `request<T>(method, path, body, opts)`).
3. **Add hook** in `src/lib/hooks.ts` (use `useTenantOpts()` for tenant).
4. **Create page** under `src/app/<route>/page.tsx`.
5. **Add to sidebar** in `src/components/Sidebar.tsx`.
6. **Reuse primitives** — `PageHeader`, `Card`, `DataTable`, `Modal`,
   `Button`, `Pill`, `EmptyState`, `LoadingState`, `ErrorState`,
   `JsonViewer`, `Input`, `Textarea`, `SearchInput`, `MetricCard`.
7. CI runs `npm run typecheck && npm run lint && npm run build` —
   green is the bar.

The console **must not** introduce bypass paths. Where the platform
refuses (tier-3 without gate, audit failure, etc.), the UI surfaces the
refusal — never a "force" flag.

See [`console.md`](./console.md) for the full design.

---

## What we don't do

- **Drive-by formatting.** Format your code; don't reformat someone else's.
- **Half-finished implementations.** Ship complete features.
- **Backwards-compat shims for code that hasn't shipped.** Just change the code.
- **Doc-only PRs without context.** A doc change should be paired with the code change it documents, or have its own commit message explaining why.
- **Disabling tests.** Fix the underlying issue.

---

## Code style

- `gofmt` + `go vet` — enforced in CI.
- Linter: `golangci-lint`. Config in `.golangci.yml`.
- No emojis in code, no emojis in commit messages.
- No comments restating the obvious. Comment **why**, never **what**.
- Test names: `Test<Function>_<Condition>_<Outcome>`.
- File names: `snake_case.go`.
- Errors: from `pkg/platform/problem`. Never raw strings.

---

## Commits + branches

- Branch from `main`. PRs target `main`.
- Conventional commits not required, but appreciated: `feat:`, `fix:`, `chore:`, `refactor:`, `docs:`.
- Sign your commits.
- Squash on merge.

---

## Dependency updates (Dependabot)

Dependabot **version updates are disabled by default**. The full config ships
as a template at
[`.github/dependabot.yml.example`](../.github/dependabot.yml.example).
Dependabot only reads `.github/dependabot.yml`, so while the config keeps its
`.example` suffix it is invisible to Dependabot — no scheduled runs, no
version-update PRs. (This is more reliable than `open-pull-requests-limit: 0`,
which leaves Dependabot running on schedule and only caps the PR count.)

**To enable version updates** (e.g. after forking this template):

```bash
mv .github/dependabot.yml.example .github/dependabot.yml
git commit -am "ci: enable Dependabot version updates"
```

The template covers Go modules, GitHub Actions, and the service Dockerfiles.
Tune each ecosystem with `open-pull-requests-limit` (`10` = normal, `0` =
pause that ecosystem) or delete its block.

CVE/security alerts are **unaffected** either way — they're governed by repo
Settings (Security → Code security), not this file.

While version updates are off, bump dependencies manually as part of the
change that needs them (`go get -u ./... && go mod tidy`, then `task test`).

---

## Where next

- [`testing.md`](./testing.md).
- [`api-reference.md`](./api-reference.md).
- [`CLAUDE.md`](../CLAUDE.md) — Claude Code working notes.
- [`adr/`](./adr/).
