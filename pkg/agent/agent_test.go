package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aegisvision/pkg/agent"
	"github.com/aegisvision/pkg/llm"
)

// A read-only tool that the agent should auto-execute.
type readTool struct{ called int }

func (t *readTool) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:                 "search_detections",
		Description:          "Search detections",
		ParametersSchemaJSON: `{"type":"object","properties":{"q":{"type":"string"}}}`,
		RiskTier:             llm.RiskTierRead,
	}
}

func (t *readTool) Run(_ context.Context, _ json.RawMessage) (any, error) {
	t.called++
	return map[string]any{"hits": []map[string]string{{"class": "person"}}}, nil
}

// A consequential tool that the agent must NOT auto-execute.
type promoteTool struct{ called int }

func (t *promoteTool) Schema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:                 "promote_model",
		Description:          "Promote a model from canary to primary",
		ParametersSchemaJSON: `{"type":"object","properties":{"model_id":{"type":"string"}}}`,
		RiskTier:             llm.RiskTierConsequential,
	}
}

func (t *promoteTool) Run(_ context.Context, _ json.RawMessage) (any, error) {
	t.called++
	return nil, nil
}

func TestAgent_ReadToolThenFinal(t *testing.T) {
	fc := llm.NewFakeClient()
	fc.ScriptToolCall("search_detections", map[string]string{"q": "person"})
	fc.ScriptFinal("Found 1 detection.")

	reg := agent.NewRegistry()
	rt := &readTool{}
	reg.Register(rt)
	a := agent.New(fc, reg, agent.NewMemoryGateway(), agent.NewMemorySink(), agent.Config{}, "t-1", "s-1")

	out, err := a.Run(context.Background(), "find any person detections in the last hour")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "completed" {
		t.Fatalf("status: %s", out.Status)
	}
	if rt.called != 1 {
		t.Fatalf("read tool called %d times", rt.called)
	}
	if out.Final == "" {
		t.Fatal("no final content")
	}
}

func TestAgent_ConsequentialToolOpensGate(t *testing.T) {
	fc := llm.NewFakeClient()
	fc.ScriptToolCall("promote_model", map[string]string{"model_id": "yolo-v9"})

	reg := agent.NewRegistry()
	pt := &promoteTool{}
	reg.Register(pt)
	gw := agent.NewMemoryGateway()
	a := agent.New(fc, reg, gw, agent.NewMemorySink(), agent.Config{}, "t-1", "s-1")

	out, err := a.Run(context.Background(), "promote yolo-v9 to primary")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "awaiting_gate" {
		t.Fatalf("expected awaiting_gate, got %s", out.Status)
	}
	if pt.called != 0 {
		t.Fatalf("consequential tool was auto-executed: %d calls", pt.called)
	}
	if out.GateID == "" {
		t.Fatal("no gate id")
	}
}

func TestAgent_RefusesUnknownTool(t *testing.T) {
	fc := llm.NewFakeClient()
	fc.ScriptToolCall("destroy_everything", map[string]string{})
	reg := agent.NewRegistry()
	a := agent.New(fc, reg, agent.NewMemoryGateway(), agent.NewMemorySink(), agent.Config{}, "t-1", "s-1")
	out, err := a.Run(context.Background(), "do harm")
	if err == nil {
		t.Fatal("expected error on unknown tool")
	}
	if out.Status != "failed" {
		t.Fatalf("status: %s", out.Status)
	}
}

func TestAgent_MaxStepsBound(t *testing.T) {
	// Script tool-call → tool-call → ... so the loop never reaches final.
	fc := llm.NewFakeClient()
	for i := 0; i < 50; i++ {
		fc.ScriptToolCall("search_detections", map[string]string{"q": "x"})
	}
	reg := agent.NewRegistry()
	reg.Register(&readTool{})
	a := agent.New(fc, reg, agent.NewMemoryGateway(), agent.NewMemorySink(), agent.Config{MaxSteps: 6}, "t-1", "s-1")
	_, err := a.Run(context.Background(), "loop forever")
	if err == nil {
		t.Fatal("expected failure on max steps")
	}
}

func TestAgent_ResumeAfterGate(t *testing.T) {
	fc := llm.NewFakeClient()
	fc.ScriptToolCall("promote_model", map[string]string{"model_id": "yolo-v9"})

	reg := agent.NewRegistry()
	reg.Register(&promoteTool{})
	gw := agent.NewMemoryGateway()
	a := agent.New(fc, reg, gw, agent.NewMemorySink(), agent.Config{}, "t-1", "s-1")
	out, _ := a.Run(context.Background(), "promote")
	if out.Status != "awaiting_gate" || out.GateID == "" {
		t.Fatalf("expected awaiting_gate, got %+v", out)
	}
	// Approver decides + the caller (agent-service) actually executes the
	// underlying action and feeds its result back via Resume.
	gw.Decide(out.GateID, "approved", "alice@example.com", "approved per RFC-X")
	fc.ScriptFinal("Model promoted successfully.")
	out2, err := a.Resume(context.Background(), out.GateID, map[string]any{"approved": true, "deployed_at": "2026-05-21T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if out2.Status != "completed" {
		t.Fatalf("resume status: %s", out2.Status)
	}
}
