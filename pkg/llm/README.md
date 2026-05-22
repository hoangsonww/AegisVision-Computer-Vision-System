# pkg/llm

> **The LLM client + safety layer.** Every LLM call goes through here.

`llm-gateway` and `agent-service` import this. Nobody else makes LLM
calls — ADR-0018.

---

## Contents

| File | Purpose |
| --- | --- |
| `http_client.go` | OpenAI-compatible HTTP client. |
| `fake.go` | Deterministic fake for dev/test. |
| `safety.go` | Sanitiser + PII redactor + refusal threshold. |
| `safety_test.go` | Safety unit tests (prompt-injection corpus). |
| `types.go` | Request / Response / Choice shapes. |

---

## Safety layer

```go
clean := safety.Sanitize(input)
redacted := safety.RedactPII(clean)
resp := client.ChatCompletion(ctx, redacted)
score := safety.Score(resp)
if score < threshold { return ErrSafetyRefused }
```

This is the canonical order. Departures from it have to be
ADR-approved.

The integration smoke test
`tools/integration/TestSmoke_LLMSafety_RefusesInjection` asserts that
prompt-injection inputs are refused.

---

## Anti-patterns

- **Don't bypass safety.** No "this prompt is trusted" shortcuts.
- **Don't roll your own HTTP client.** Use this one.
- **Don't call backends directly.** Go through `llm-gateway`.
