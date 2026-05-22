# Audit evidence collection scripts

Operator-side helpers an internal compliance team uses to pull a fresh
evidence package for an upcoming SOC 2 / EU AI Act / ISO 27001 review.

The actual evidence lives in `compliance-evidence-service` (per
ADR-0029); these scripts are convenience wrappers that:

1. Authenticate against the audit token vault.
2. Pull the per-control CSV / JSONL for the requested window.
3. Bundle the results into a single zip with a manifest +
   timestamped attestation.

## Contents

```
tools/audit/
├── README.md
├── pull-evidence.sh         # one-shot per-tenant per-window evidence bundle
├── policy-pack.sh           # corporate policy markdown → PDF for auditor handover
└── controls-coverage.sh     # report which SOC 2 controls have produced
                              # >= N evidence rows over the audit window
                              # (catches "control covered on paper but
                              # never triggered" gaps before the auditor does)
```

## Usage

```sh
# Pull a 3-month evidence bundle for tenant `acme`:
./tools/audit/pull-evidence.sh \
  --tenant acme \
  --from 2026-01-01 \
  --to   2026-04-01 \
  --out  /tmp/audit-acme-q1

# Quick "did the controls actually fire?" report before audit kickoff:
./tools/audit/controls-coverage.sh --tenant acme --window 90d
```
