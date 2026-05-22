# Security Policy

> **AegisVision takes security seriously.** This document describes how
> to report a vulnerability and the supported versions.

---

## Supported versions

Security patches are issued for the latest minor release. Older minors
receive fixes on a best-effort basis.

| Version | Supported |
| ------- | --------- |
| 0.5.x   | ✅        |
| 0.4.x   | ⚠️ Best-effort |
| < 0.4   | ❌        |

---

## Reporting a vulnerability

**Do not file public GitHub issues for security vulnerabilities.**

Instead, please email **hoangson091104@gmail.com** with:

- A clear description of the vulnerability.
- The affected component (service / library / chart / tool).
- Steps to reproduce.
- The impact (what an attacker could do).
- Any suggested mitigation.

You should receive an acknowledgement within **5 business days**, and
a status update within **14 business days**.

Encrypted reports are welcomed. PGP key on request via email.

---

## Disclosure timeline

The platform follows **coordinated disclosure**:

1. **0–14 days**: triage + acknowledgement.
2. **14–60 days**: investigate, develop fix, prepare release.
3. **60–90 days**: release fix, publish advisory, credit reporter
   (unless reporter prefers anonymity).
4. **90+ days**: if no fix is possible, work with the reporter on a
   disclosure plan.

---

## Hall of Fame

Reporters who file valid vulnerabilities will be credited (with their
permission) in the project's [`ACKNOWLEDGMENTS.md`](./ACKNOWLEDGMENTS.md).

---

## Security architecture

For background on the platform's defense-in-depth posture, see:

- [`docs/security.md`](./docs/security.md) — threat model + defenses.
- [`docs/compliance/`](./docs/compliance/) — SOC 2 / EU AI Act / GDPR.
- [`docs/adr/0014-bounded-autonomy.md`](./docs/adr/0014-bounded-autonomy.md).
- [`docs/adr/0017-bounded-autonomy-implementation.md`](./docs/adr/0017-bounded-autonomy-implementation.md).
- [`docs/adr/0021-prompt-injection-defense.md`](./docs/adr/0021-prompt-injection-defense.md).

---

## Out of scope

Findings that are **out of scope** for this policy:

- Missing rate limits on dev / demo deployments.
- Vulnerabilities requiring root-on-host access.
- Vulnerabilities in third-party dependencies already disclosed
  upstream (please report to the upstream project).
- Theoretical attacks without a working PoC.
- Social engineering attacks on the project author or contributors.

---

## Contact

**Son Nguyen** (author / maintainer)
- Email: <hoangson091104@gmail.com>
- GitHub: <https://github.com/hoangsonww>
- LinkedIn: <https://linkedin.com/in/hoangsonw>
- Website: <https://sonnguyenhoang.com>
