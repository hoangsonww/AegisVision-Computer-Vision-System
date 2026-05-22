# ADR-0029: Compliance-evidence-service owns no data

**Status:** Accepted (2026-05-21)

## Context

Auditors (SOC 2, EU AI Act, ISO 27001, internal compliance) need
evidence on demand: "show me every privileged-action approval for
tenant X in Q1." The naïve implementation is a separate database that
ingests audit events + decisions and lets the auditor query it.

That implementation has three problems:

1. **Two copies of the truth.** The audit log lives in audit-service;
   gate decisions in policy-gate-service. A separate evidence DB would
   need to ingest both, and any divergence between the two is a "which
   is right" question we never want to answer.
2. **A new system of record to secure.** Each duplicate store is a
   target for the same exfiltration threats we mitigate in the
   authoritative stores. Worth it only if it earns its keep.
3. **A new system to keep in sync.** Schema drift between the
   authoritative store and the evidence DB silently breaks the audit
   query.

## Decision

`compliance-evidence-service` owns *no* data. It is a query composer
that fans out to the authoritative stores:

- audit-service → CC6.1 / CC7.3 evidence
- policy-gate-service → CC6.6 evidence
- slo-watchdog → CC7.2 evidence
- drift-detection-service → CC7.4 evidence

The service exposes:

- `GET /v1/evidence?control=...&from=...&to=...` — JSON envelope
- `GET /v1/evidence.csv?...` — CSV download (auditor preference)
- `GET /v1/evidence.jsonl?...` — streaming JSON-Lines for very large
  windows

The wire is tenant-scoped at the gateway. Each collector implements a
small `Collector` interface (`Name`, `Controls`, `Collect`) — tests
swap real upstreams for stubs.

## Consequences

- **One copy of the truth.** The authoritative service answers the
  query; the evidence service composes the response.
- **Auditor-friendly format.** CSV is the lowest-common-denominator
  for evidence packages handed to outside auditors.
- **No new data store to secure.** No ingestion lag, no schema-drift
  failure mode. The service is stateless.
- **Adding a new control means adding a Collector**, not a new table.
  The 4 production Collectors (audit, gate, slo, drift) cover CC6.1,
  CC6.6, CC7.2, CC7.3, CC7.4. Future controls (CC8.1 change-management
  evidence) add a new Collector that talks to e.g. ArgoCD's audit API.

## Rejected alternatives

- **Materialized evidence DB.** Rejected for the reasons above.
- **Direct auditor access to each upstream.** Rejected — auditors get
  a single endpoint, not 4. Also: cross-service filtering (e.g. "list
  all evidence for tenant X across all controls") is impossible
  without a composer.
- **GraphQL.** Rejected — adds dependency surface for a use case
  (per-control, time-windowed evidence pull) that does not benefit
  from GraphQL's join semantics.
