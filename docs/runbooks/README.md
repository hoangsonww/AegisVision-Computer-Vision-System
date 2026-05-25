# Runbooks

Operational guides for the AegisVision platform.

| Runbook | Purpose |
| --- | --- |
| [oncall.md](oncall.md) | First-90-seconds triage; what to look at when paged. |
| [incident-response.md](incident-response.md) | Severity levels, IC process, postmortem norms. |
| [dr.md](dr.md) | Disaster recovery procedures. RPO/RTO targets. |
| [triton.md](triton.md) | NVIDIA Triton Inference Server — failure modes + remediations. |
| [canary-rollback.md](canary-rollback.md) | When the canary controller rolls back a model promotion. |
| [drift-spike.md](drift-spike.md) | Investigation playbook for a class-distribution drift alarm. |
| [agent-incident.md](agent-incident.md) | When the bounded-autonomy agent behaves unexpectedly. |
| [chaos-game-day.md](chaos-game-day.md) | Quarterly chaos game-day procedure. |

Add new runbooks here when a class of incident has happened twice and we want
to make sure the third time is faster than the first.
