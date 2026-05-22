# pkg/agent

> **Bounded-autonomy agent runtime + tool router + gate hook.**
> ADR-0014, ADR-0017.

`agent-service` (and `autonomy-orchestrator` indirectly) imports this.
Nobody grows a second agent runtime. ADR-0022.

---

## API

```go
type Agent struct {
    LLM     llm.Client
    Tools   ToolRegistry
    Gate    GateClient    // routes tier-3 through policy-gate-service
    Audit   audit.Client
    Knowledge knowledge.Client
}

func (a *Agent) Run(ctx context.Context, req Request) (Result, error)
func (a *Agent) Resume(ctx context.Context, requestID string, toolResult any) (Result, error)
```

`Run` returns either:

- A final answer (with citations), or
- A `PendingGate{ID: ...}` if the agent chose a tier-3 tool.

`Resume` continues a paused session after the gate resolves.

---

## Tiered tool refusal

The runtime refuses tier-3 tools without a gate **in code**:

```go
if tool.Tier == Tier3 && req.GateID == "" {
    return Result{}, ErrTier3NeedsGate
}
```

You cannot prompt your way past this.

---

## Citation enforcement

If a tool result requires citation (knowledge query, NLQ structured
query) and the agent's final answer doesn't include the citation, the
runtime surfaces an error. ADR-0020.

---

## Anti-patterns

- **Don't add a separate agent runtime.** Open a session against `agent-service`.
- **Don't bypass the gate hook.** That's the entire point of bounded autonomy.
- **Don't cite training data.** Cite knowledge-service snippets.
