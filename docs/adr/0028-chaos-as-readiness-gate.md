# ADR-0028: Chaos engineering as production-readiness gate

**Status:** Accepted (2026-05-21)

## Context

Every ADR in this repo encodes a commitment about what the platform does
when something breaks. ADR-0005: the edge keeps working when WAN is down.
ADR-0014: agents fail-closed when the gate is unreachable. ADR-0023:
canaries roll back on regression. The architecture doc treats these as
load-bearing — but a commitment that has never been exercised in a
controlled failure is documentation, not engineering.

We need to validate that the platform exhibits the documented behaviour
*under the actual failure*, not just under the absence of the failure.

## Decision

Chaos engineering is treated as a first-class production-readiness gate,
not an exploration activity:

- **Every load-bearing failure mode has a corresponding chaos
  experiment** under `deploy/chaos/`. The set is exhaustive against the
  ADRs — adding a new ADR with an availability/resilience claim requires
  adding the experiment that asserts it.
- **Every experiment is paired with a check job** that emits a
  machine-readable PASS/FAIL after the experiment's duration. Without
  an assertion, the experiment is just an outage.
- **The quarterly game-day** (`docs/runbooks/chaos-game-day.md`) runs
  the full set against staging, on a fixed cadence, with a SOC 2 / EU
  AI Act audit attestation as the output.
- **Failure to drill is treated as a SEV-2 incident**. The cadence is
  the point — if the platform skips a quarter, on-call rotation is
  out of practice.

## Consequences

- **Adding a new resilience claim requires an experiment.** If you
  can't write the chaos experiment, you don't actually know what you
  claim — write it before the code.
- **Failed drills produce post-mortems, not silence**. Even one failed
  experiment generates a SEV-2, with a public retrospective. The
  retrospective lands in `docs/runbooks/incidents/` and feeds the next
  drill's assertions.
- **Staging must look like production.** Without that, the drill
  isn't measuring what we claim. Staging has the same multi-AZ
  topology, the same Patroni / ClickHouse Operator configs, the same
  Kyverno admission policies.
- **Drills produce evidence**. The DR drill scripts
  (`tools/dr-drills/`) and chaos check jobs each emit a signed audit
  record — auditors retrieve it via
  `compliance-evidence-service` (`/v1/evidence?control=CC9.1`).

## Rejected alternatives

- **Chaos as exploratory red-team day.** Rejected — exploratory is
  fine, but the *gate* requires deterministic experiments with
  documented assertions. Surprise doesn't compound with the audit
  trail.
- **Chaos in production with limited blast radius.** Rejected for
  now — staging fidelity is sufficient given the platform's tenant
  isolation. We may revisit for specific experiments (e.g. failing
  the LLM gateway for 5% of agent traffic) once the platform has been
  GA for a year.
- **One chaos engineer's checklist instead of YAML manifests.**
  Rejected — manifests are the source of truth + reviewable + drift
  against the resilience claim is detectable.
