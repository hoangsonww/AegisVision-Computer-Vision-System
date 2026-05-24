# Contributing to AegisVision

> Thanks for taking the time to consider contributing. This document is
> the short version; the long version lives in
> [`docs/contributing.md`](./docs/contributing.md).

AegisVision is authored and maintained by **Son Nguyen**
([@hoangsonww](https://github.com/hoangsonww)). Issues, discussions, and
PRs from anyone are welcome.

---

## Quick links

- **Architecture**: [`ARCHITECTURE.md`](./ARCHITECTURE.md)
- **Setup**: [`SETUP_GUIDE.md`](./SETUP_GUIDE.md)
- **Detailed contributor guide**: [`docs/contributing.md`](./docs/contributing.md)
- **Testing**: [`docs/testing.md`](./docs/testing.md)
- **Code of conduct**: [`CODE_OF_CONDUCT.md`](./CODE_OF_CONDUCT.md)
- **Security policy**: [`SECURITY.md`](./SECURITY.md)
- **Maintainer**: Son Nguyen — <hoangson091104@gmail.com>

---

## Ground rules

1. **Read the relevant ADR** before making a load-bearing change.
   Architecture is encoded in `docs/adr/`. If a change conflicts with an
   ADR, open an ADR amendment, not a feature PR.
2. **Follow the conventions.** They exist because each one solves a
   problem that bit a prior CV platform. See
   [`docs/contributing.md`](./docs/contributing.md).
3. **No proto-bypass.** New transports require a protobuf contract first
   (ADR-0007).
4. **No raw LLM calls.** Use `llm-gateway` (ADR-0018).
5. **No second agent runtime.** Open a session against `agent-service`
   (ADR-0022).
6. **No force-promote.** Promotion routes through `policy-gate-service`
   (ADR-0023).
7. **No image bytes on the bus.** Use claim-check (ADR-0008).

---

## Workflow

```bash
git clone https://github.com/hoangsonww/AegisVision-Computer-Vision-System.git
cd AegisVision-Computer-Vision-System
task bootstrap     # installs tools, generates protos, builds everything
task test          # runs unit tests with -race
```

Before opening a PR:

1. `task vet` is clean.
2. `task test` is green.
3. `(cd tools/conformance && go test -count=1 ./...)` is green.
4. `(cd tools/integration && go test -race -count=1 ./...)` is green.
5. New behaviour has tests.
6. New bus subjects are added to the integration smoke catalogue.
7. New services have a README.
8. CHANGELOG entry if user-visible.

---

## Reporting issues

Use **GitHub Issues**. Use the issue templates when one fits. For
security issues, see [`SECURITY.md`](./SECURITY.md) — please **do not**
file public issues for security reports.

---

## Pull requests

- One topic per PR.
- Conventional commits welcomed (not required): `feat:`, `fix:`,
  `chore:`, `refactor:`, `docs:`.
- Sign your commits.
- Squash on merge.

I (Son) review PRs personally. Expect feedback rather than a fast-pass
merge.

---

## License

By contributing, you agree that your contributions will be licensed
under the **Apache License, Version 2.0**. See [`LICENSE`](./LICENSE).
