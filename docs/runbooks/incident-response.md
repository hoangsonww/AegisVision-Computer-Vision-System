# Incident response runbook

## Severity levels

| Sev | Definition | Pager | Public status page |
| --- | --- | --- | --- |
| **SEV-1** | Glass-to-event broken for ≥ 1 tenant for ≥ 5 min | Yes | Yes, immediate |
| **SEV-2** | Glass-to-event degraded for ≥ 1 tenant for ≥ 10 min | Yes | Yes, within 15 min |
| **SEV-3** | Single service degraded, no tenant impact | Yes (business hours) | Optional |
| **SEV-4** | Internal-only impact | No | No |

## SEV-1 / SEV-2 process

1. **Page**: the on-call sees the alert in PagerDuty.
2. **Acknowledge** within 5 minutes. Open a `#incident-<date>` Slack channel.
3. **Incident commander**: the first responder is IC by default unless they
   hand off. The IC's job is to coordinate, not to fix.
4. **Comms**: post a public status page entry within 15 min for SEV-1, 30
   min for SEV-2.
5. **Mitigation first**: roll back, scale up, divert traffic. Root-cause
   analysis happens AFTER customer impact is contained.
6. **Postmortem** within 5 business days. Use the template in
   `docs/runbooks/postmortem-template.md`.

## Communication norms

- All actions taken on production go in the channel as soon as they happen.
  "I'm restarting the gateway" — yes, write it.
- No solo heroics. If you're about to do something destructive, write it
  first and pause 30 seconds for objections.
- The IC says "all clear" when the alert clears AND a synthetic test
  confirms recovery.

## Post-incident

- Postmortem is blameless. Look for missing safety nets, not missing humans.
- Action items get filed as issues with deadlines, owners, and links from
  the postmortem doc.
- Pattern review: if the same component has been on fire 3 times in 30
  days, schedule a deeper review.
