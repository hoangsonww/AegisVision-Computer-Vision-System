# ADR-0030: Release engineering — release-please + signed air-gap bundles

**Status:** Accepted (2026-05-21)

## Context

A 35-service monorepo with a multi-arch container build per service
needs an opinionated release model. The non-obvious choice is whether
each service ships independently or the platform ships as a single
versioned set.

We chose **single versioned set**. The platform's invariants are
cross-service — the bus subject contract, the proto contract, the
agent risk-tier contract — and divergent service versions in
production would silently undermine those invariants.

## Decision

- **Single platform version.** `0.1.0`, `0.2.0`, … `0.5.0`. Every
  service image and every Helm chart in a release share the same
  version tag.
- **Semantic Versioning + Conventional Commits.** Commits on `main`
  follow Conventional Commits (`feat:`, `fix:`, `perf:`, `security:`,
  `docs:`, …). [release-please](https://github.com/googleapis/release-please)
  cuts the next version + the CHANGELOG entry based on commit history
  (`feat` → minor bump, `fix` → patch, breaking-change footer →
  major).
- **Release-please opens a release PR.** When merged, it creates a
  GitHub release at the tag.
- **Air-gap bundle + image promotion follow the release.** The
  `release.yml` workflow's `airgap-bundle` and `promote-images` jobs
  run only when `release-please` reports `release_created == true`.
- **Pre-GA semver semantics.** Until `1.0.0`, minor bumps may include
  breaking changes per semver §4. The first GA release is `1.0.0`;
  beyond that, breaking changes require a major.

## Consequences

- **Every release is a SOC 2-friendly atomic change.** The release
  notes are derived from PR titles, themselves derived from commit
  type — no manual changelog drift.
- **Image tag = chart tag = bundle version.** Operators install
  `0.5.0` and know everything is `0.5.0`. Drift across components is
  not even representable in this scheme.
- **Multi-arch image promotion is automatic.** The `ci-${SHA}` tag
  set by CI is the source of truth; `release.yml` re-tags it to
  `0.5.0` and `latest` via `crane tag` — no rebuild, so the digest
  (and therefore signature) is preserved.
- **CHANGELOG.md is generated, not hand-edited.** Operators trust
  the changelog because it cannot drift from the merged commits.

## Rejected alternatives

- **Per-service versioning.** Rejected — too many invariants are
  shared across services. We'd spend more time chasing combinatorial
  compatibility matrices than we'd save in per-service deploy
  flexibility.
- **Manual CHANGELOG.** Rejected — drift is the default state of
  hand-edited changelogs.
- **Calendar versioning (2026.05.21).** Rejected — semver carries
  semantic information (breaking? feature? fix?). CalVer hides that.
