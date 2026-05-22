# Runbook: Agent incident response

**Last updated:** 2026-05-21
**Owner:** platform-oncall

## Symptom catalog

### S1 — Spike in `aegis_llm_refusals_total{reason="prompt_injection_suspected"}`

A small, slow rate of refusals is healthy — operators and external clients
do occasionally type things that trip the sanitizer. A sudden order-of-
magnitude rise means one of three things:

1. **A misbehaving client** is hammering an endpoint with a prompt that
   trips the regex (often a debugging client with `Ignore all previous
   instructions` in a test fixture).
2. **An attacker** is probing the platform with injection variants.
3. **A regex over-matches** after a recent sanitizer update.

**Actions, in order:**

```
# Inspect: which tenants?
kubectl exec -n aegis-system deploy/llm-gateway -- \
  wget -qO - http://localhost:8401/metrics | \
  grep aegis_llm_refusals_total

# Inspect: which routes?
kubectl logs -n aegis-system -l app.kubernetes.io/name=llm-gateway --tail=200 | \
  grep refusal_reason

# If a single tenant: contact + temporarily lower their LLM quota
# If a single client UA: block at the edge (Istio)
# If a regex over-matches: revert the sanitizer change in pkg/llm/safety.go
```

### S2 — Agent session stuck in `awaiting_gate` for > 1h

```
# How many pending?
curl -s http://policy-gate-service.aegis-system:8410/v1/gates | jq '.items | length'

# Page the team who owns the requesting agent
curl -s http://policy-gate-service.aegis-system:8410/v1/gates | jq '.items[] | {id, tool_name, blast_radius, requested_at}'
```

Gates expire after their TTL (default: 72h) — at which point the agent
resumes with a synthetic "expired" tool result. If you need to expire
sooner:

```
curl -X POST http://policy-gate-service.aegis-system:8410/v1/gates/<id>/decision \
  -H 'Content-Type: application/json' -H 'X-Aegis-Subject: oncall@aegis' \
  -d '{"decision":"rejected","reason":"oncall-expired"}'
```

### S3 — Active-learning daily budget exhausted

```
kubectl exec -n aegis-system deploy/active-learning-service -- \
  wget -qO - http://localhost:8441/metrics | \
  grep aegis_al_samples_rejected_total
```

If `reason="daily_budget_exhausted"` dominates, the loop is healthy but
saturated. Either raise the daily budget (per-tenant) or accept the
ceiling. Do NOT bypass the budget for "just this once" — operational safeguard:
unbounded labeling burden destroys the operator team.

### S4 — Knowledge service returning empty for known docs

Likely causes (in order):

1. Reingest hasn't run since a docs change. Run:
   ```
   curl -X POST http://knowledge-service.aegis-system:8430/v1/knowledge/reingest \
     -H 'X-Aegis-Role: platform-admin'
   ```
2. Embedding model changed; old vectors are at the wrong dim. Drop +
   reingest the corpus:
   ```
   kubectl exec -n aegis-system deploy/knowledge-service -- \
     /usr/local/bin/knowledge-service --reset
   ```
3. The `q` query has no overlap with any indexed doc. Try the snippet
   verbatim.

### S5 — agent-service emits responses with `safety.refused = true`

The model's output tripped the output PII redactor or the safety layer
classified the output as unsafe. This is correct behavior — the agent has
declined to answer. If a legitimate response is being mis-classified,
inspect the `safety` field in the response payload and consider tuning
the redactor patterns in `pkg/llm/safety.go`.

## Blast-radius cheat sheet (when approving gates)

| Tool | Blast radius |
|------|--------------|
| `promote_model` | Tenant-wide; affects every pipeline using the model. **High.** |
| `override_retention` | Tenant-wide; legal/compliance implications. **High.** |
| `pause_draft_pipeline` | Single draft; reversible. **Low.** |
| `propose_*` | None — produces a draft, not a deployment. **None.** |

Refuse anything you can't justify. The default answer to "should I approve
this?" when you don't know is **no**.
