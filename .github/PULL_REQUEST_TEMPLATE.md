<!--
Thanks for contributing to AegisVision! A few notes before you submit:

- One topic per PR.
- New behaviour needs tests.
- New bus subjects must be added to tools/integration/integration_test.go.
- New services need a README.
- Architectural changes need an ADR in docs/adr/.

See CONTRIBUTING.md for the full guide.
-->

## Summary

<!-- 1–3 bullets describing what changed and why. Focus on the "why". -->

-
-
-

## Type of change

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation only
- [ ] Refactor (no functional change)
- [ ] Chore (build, dependency, tooling)
- [ ] ADR (architectural decision record)

## Affected areas

<!-- Tick all that apply. -->

- [ ] proto/ (contract change — CODEOWNERS will review)
- [ ] pkg/ (shared library)
- [ ] services/ (a service implementation)
- [ ] deploy/ (Helm / k8s / chaos / terraform)
- [ ] tools/ (CI / test / scaffolding)
- [ ] docs/ (docs / ADRs / runbooks)
- [ ] .github/ (CI / templates)

## Checklist

- [ ] I read [`CONTRIBUTING.md`](../CONTRIBUTING.md) and [`docs/contributing.md`](../docs/contributing.md).
- [ ] I checked the relevant ADR(s) in [`docs/adr/`](../docs/adr/).
- [ ] `task vet` is clean.
- [ ] `task test` is green.
- [ ] `(cd tools/conformance && go test -count=1 ./...)` is green.
- [ ] `(cd tools/integration && go test -race -count=1 ./...)` is green.
- [ ] I added tests for new behaviour.
- [ ] I added/updated the relevant README(s).
- [ ] I added an ADR for any load-bearing architectural change.
- [ ] I added a CHANGELOG entry for user-visible changes.
- [ ] No secrets are committed.

## Test plan

<!-- How did you verify this works? Include commands + observed outputs where useful. -->

## Screenshots / output (if UI / observability)

<!-- Paste here. -->

## Related issues / ADRs

<!-- Use "Closes #N" / "Refs ADR-0023". -->
