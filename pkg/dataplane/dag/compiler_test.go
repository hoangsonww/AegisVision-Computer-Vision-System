package dag_test

import (
	"errors"
	"testing"

	"github.com/aegisvision/pkg/dataplane/dag"
)

func reg() *dag.Registry {
	return dag.NewRegistry("source", "model.infer", "tracker", "rule.spatial", "sink.event")
}

func TestCompile_LinearChain(t *testing.T) {
	plan, err := dag.Compile(dag.PipelineSpec{
		ID: "p", Nodes: []dag.NodeSpec{
			{ID: "src", Type: "source"},
			{ID: "det", Type: "model.infer", After: []string{"src"}},
			{ID: "trk", Type: "tracker", After: []string{"det"}},
			{ID: "rul", Type: "rule.spatial", After: []string{"trk"}},
			{ID: "out", Type: "sink.event", After: []string{"rul"}},
		},
	}, reg())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(plan.Stages); got != 5 {
		t.Fatalf("want 5 stages, got %d", got)
	}
	ord := plan.LinearOrder()
	want := []string{"src", "det", "trk", "rul", "out"}
	for i, n := range ord {
		if n.ID != want[i] {
			t.Fatalf("at %d want %s got %s", i, want[i], n.ID)
		}
	}
}

func TestCompile_ParallelBranches(t *testing.T) {
	plan, err := dag.Compile(dag.PipelineSpec{
		ID: "p", Nodes: []dag.NodeSpec{
			{ID: "src", Type: "source"},
			{ID: "det_a", Type: "model.infer", After: []string{"src"}},
			{ID: "det_b", Type: "model.infer", After: []string{"src"}},
			{ID: "merge", Type: "sink.event", After: []string{"det_a", "det_b"}},
		},
	}, reg())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(plan.Stages); got != 3 {
		t.Fatalf("want 3 stages (src; det_a||det_b; merge), got %d", got)
	}
	if len(plan.Stages[1]) != 2 {
		t.Fatalf("want 2 nodes in parallel stage, got %d", len(plan.Stages[1]))
	}
}

func TestCompile_RejectsCycle(t *testing.T) {
	_, err := dag.Compile(dag.PipelineSpec{
		ID: "p", Nodes: []dag.NodeSpec{
			{ID: "a", Type: "source", After: []string{"b"}},
			{ID: "b", Type: "source", After: []string{"a"}},
		},
	}, reg())
	if !errors.Is(err, dag.ErrCycle) {
		t.Fatalf("want ErrCycle, got %v", err)
	}
}

func TestCompile_RejectsUnknownType(t *testing.T) {
	_, err := dag.Compile(dag.PipelineSpec{
		ID: "p", Nodes: []dag.NodeSpec{{ID: "x", Type: "nope"}},
	}, reg())
	if !errors.Is(err, dag.ErrUnknownType) {
		t.Fatalf("want ErrUnknownType, got %v", err)
	}
}

func TestCompile_RejectsDanglingReference(t *testing.T) {
	_, err := dag.Compile(dag.PipelineSpec{
		ID: "p", Nodes: []dag.NodeSpec{
			{ID: "x", Type: "source", After: []string{"ghost"}},
		},
	}, reg())
	if !errors.Is(err, dag.ErrUnknownAfter) {
		t.Fatalf("want ErrUnknownAfter, got %v", err)
	}
}
