# EU AI Act posture

The EU AI Act classifies remote biometric identification as **high-risk**.
This document maps the platform's controls to the relevant articles.

## Risk classification

| Capability | Risk class | Where enforced |
| --- | --- | --- |
| Object detection (vehicle, pallet, etc.) | Limited risk | n/a |
| Person detection | Limited risk | n/a |
| Face detection (no identification) | Limited risk | n/a |
| Face / gait identification | **High risk** | Gated; separately licensed |
| Biometric categorization | **Prohibited** | Disabled by default; license required |
| Real-time RBI in public spaces | **Prohibited** without legal carve-out | Disabled outside compliance mode |

## Conformity assessment

For high-risk capabilities, the platform produces a **model card** at
promotion time:

- Intended use + out-of-scope use
- Training-data lineage (dataset + version IDs)
- Performance metrics broken down by demographic group (fairness eval)
- Known limitations
- Logs of the conformity assessment

This card is stored alongside the model artifact and signed (cosign).

## Risk management system (Art. 9)

- Continuous monitoring via `aegis_inference_*` metrics + Grafana fairness
  dashboards.
- Active-learning loop closes from production drift back to training
  (annotation-service + training-orchestrator + dataset-service).
- Quarterly model reviews are scheduled tasks tracked in the platform's
  routines.

## Data governance (Art. 10)

- Each dataset_version carries a manifest in object storage; the manifest
  records data origin, consent basis, geographic distribution.
- Training pipelines refuse to consume datasets without a manifest.

## Technical documentation (Art. 11)

- `docs/adr/` provides the architecture record.
- The model card from §Conformity above provides per-model documentation.
- Audit log (audit.v1) provides operational provenance.

## Record keeping (Art. 12)

- All high-risk model invocations are logged to `audit.v1`. The retention
  is **10 years** for high-risk capabilities (configured in the audit-service
  retention policy override).
- Operator actions (start/stop pipelines, promote models, change rules) go
  through the bounded-autonomy gate (ADR-0014) which itself audits.

## Transparency (Art. 13)

- The console surfaces the model name + version on every emitted event.
- A "compliance mode" hard-disables capabilities classified as prohibited or
  restricted by jurisdiction; the per-tenant config picks the jurisdiction.

## Human oversight (Art. 14)

- ADR-0014: consequential actions require human approval via a Temporal
  signal gate. Agents may propose but cannot promote a model or change a
  safety rule.
- ADR-0017 (Phase 4) implements this gate as `policy-gate-service`. The
  gate is enforced in code (the agent loop refuses to execute tier-3
  tools) — not just in the LLM prompt. A jailbroken model cannot bypass.
- Every gate decision is published to `audit.v1` and stored append-only;
  the audit record is the evidence trail for Art. 12 (record-keeping).

## Agentic-AI specific controls (Phase 4)

The platform ships agents that can issue tool calls. Each tool has a
risk tier (read / advise / reversible / consequential — see ADR-0017).
Tier-3 tools are gated. We treat agents as themselves AI systems under
the Act:

- **Transparency (Art. 13).** Every agent response includes the model
  used, the safety signals, and citations into the knowledge corpus
  (ADR-0020). Users see what the agent did, not just what it said.
- **Robustness against prompt injection (Art. 15).** ADR-0021 details
  the defense-in-depth layers (sanitizer → PII redactor → translator
  hardening → rate limit → gate).
- **No autonomous deployment changes.** The agent runtime cannot deploy
  a model, change a retention policy, or alter any other production
  setting without a human's signature on a gate. This is enforced at
  the code layer in `pkg/agent` (tier check) and at the service layer
  in `policy-gate-service` (state machine).
- **Logging & retention.** Every agent step (plan / tool_call /
  tool_result / gate / final) is persisted by `agent-service` in
  per-tenant storage and forwarded to `audit.v1`. Retention follows the
  same tenant policy as other audit data.

## Accuracy, robustness, cybersecurity (Art. 15)

- Profile-gated promotion (ADR-0003): a model with no measured cost +
  accuracy profile cannot reach STABLE.
- The supply chain is cosign-signed + SLSA-provenanced + dependabot-tracked
  at every CI run.
- Kyverno admission policies enforce non-root, drop-ALL caps, no privileged.

## Incident reporting (Art. 73)

The SEV-1/SEV-2 process in `docs/runbooks/incident-response.md` triggers a
mandatory report to relevant authorities within 15 days for incidents that
involve high-risk capabilities. The audit log is the evidence record.
