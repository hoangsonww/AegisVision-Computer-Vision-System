package dag_test

import (
	"testing"

	"github.com/aegisvision/pkg/dataplane/dag"
)

func TestLoadJSON_Compiles(t *testing.T) {
	body := []byte(`{
        "id": "p",
        "nodes": [
          {"id": "src", "type": "source"},
          {"id": "det", "type": "model.infer", "after": ["src"]},
          {"id": "out", "type": "sink.event", "after": ["det"]}
        ]
    }`)
	spec, err := dag.LoadJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	if spec.ID != "p" || len(spec.Nodes) != 3 {
		t.Fatalf("spec: %+v", spec)
	}
	plan, err := dag.Compile(spec, dag.NewRegistry("source", "model.infer", "sink.event"))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Stages) != 3 {
		t.Fatalf("stages: %d", len(plan.Stages))
	}
}

func TestLoadYAML_Subset(t *testing.T) {
	yaml := `
id: dock-watch
nodes:
  - id: src
    type: source
  - id: det
    type: model.infer
    after: [src]
  - id: out
    type: sink.event
    after: [det]
`
	spec, err := dag.LoadYAML([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if spec.ID != "dock-watch" {
		t.Fatalf("id: %q", spec.ID)
	}
	if len(spec.Nodes) != 3 {
		t.Fatalf("nodes: %d", len(spec.Nodes))
	}
	if _, err := dag.Compile(spec, dag.NewRegistry("source", "model.infer", "sink.event")); err != nil {
		t.Fatalf("compile: %v", err)
	}
}
