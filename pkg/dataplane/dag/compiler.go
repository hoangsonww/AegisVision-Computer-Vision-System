// Package dag compiles a Pipeline DAG (the YAML/JSON form documented in
// the architecture doc §20.3) into an ordered chain of operators that the
// dataplane runtime can execute.
//
// The compiler is intentionally simple: it does a topological sort, rejects
// cycles, validates that every node refers to a registered operator factory,
// and returns either an []dataplane.Operator chain (for the linear-chain
// runtime today) or a Plan (for the fan-out runtime we'll switch to in a
// later phase). For the walking skeleton we use the linear path; the DAG
// data model is the same.
package dag

import (
	"errors"
	"fmt"
	"sort"
)

// NodeSpec is one node in a user-authored Pipeline.spec.nodes[] entry.
type NodeSpec struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Params map[string]any `json:"params"`
	After  []string       `json:"after,omitempty"` // upstream node IDs
}

// PipelineSpec is the compiler's input.
type PipelineSpec struct {
	ID    string     `json:"id"`
	Nodes []NodeSpec `json:"nodes"`
}

// Plan is the compiler's output. Stages run in order; nodes within a stage
// run in parallel. The walking-skeleton runtime collapses stages into a
// single chain; the parallel runtime consumes Stages directly.
type Plan struct {
	Stages [][]NodeSpec
}

// Errors that the compiler surfaces.
var (
	ErrEmpty         = errors.New("dag: empty pipeline")
	ErrDuplicateID   = errors.New("dag: duplicate node id")
	ErrUnknownAfter  = errors.New("dag: node references unknown upstream")
	ErrCycle         = errors.New("dag: cycle detected")
	ErrUnknownType   = errors.New("dag: unknown operator type")
)

// Registry maps operator type names to factory descriptors. The dataplane
// runtime registers its built-in types (ingest, sampler, model.infer,
// tracker, rule.spatial, rule.temporal, rule.composite, sink.event, ...)
// before compiling.
type Registry struct {
	known map[string]struct{}
}

func NewRegistry(types ...string) *Registry {
	r := &Registry{known: make(map[string]struct{}, len(types))}
	for _, t := range types {
		r.known[t] = struct{}{}
	}
	return r
}

func (r *Registry) Add(t string) { r.known[t] = struct{}{} }
func (r *Registry) Has(t string) bool {
	_, ok := r.known[t]
	return ok
}

// Compile validates the spec and produces a Plan.
func Compile(spec PipelineSpec, reg *Registry) (Plan, error) {
	if len(spec.Nodes) == 0 {
		return Plan{}, ErrEmpty
	}

	// 1. Build the node index.
	byID := make(map[string]*NodeSpec, len(spec.Nodes))
	for i := range spec.Nodes {
		n := &spec.Nodes[i]
		if n.ID == "" {
			return Plan{}, fmt.Errorf("dag: node missing id")
		}
		if _, exists := byID[n.ID]; exists {
			return Plan{}, fmt.Errorf("%w: %s", ErrDuplicateID, n.ID)
		}
		byID[n.ID] = n
	}

	// 2. Validate references.
	for _, n := range byID {
		if reg != nil && !reg.Has(n.Type) {
			return Plan{}, fmt.Errorf("%w: node %s type %q", ErrUnknownType, n.ID, n.Type)
		}
		for _, up := range n.After {
			if _, ok := byID[up]; !ok {
				return Plan{}, fmt.Errorf("%w: node %s references %q", ErrUnknownAfter, n.ID, up)
			}
		}
	}

	// 3. Kahn topological sort. Stages = layers of nodes whose upstream are
	//    all already scheduled.
	indeg := make(map[string]int, len(byID))
	children := make(map[string][]string, len(byID))
	for _, n := range byID {
		indeg[n.ID] = len(n.After)
		for _, up := range n.After {
			children[up] = append(children[up], n.ID)
		}
	}
	var ready []string
	for id, d := range indeg {
		if d == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	var plan Plan
	scheduled := 0
	for len(ready) > 0 {
		stage := make([]NodeSpec, 0, len(ready))
		for _, id := range ready {
			stage = append(stage, *byID[id])
		}
		plan.Stages = append(plan.Stages, stage)
		scheduled += len(stage)
		var next []string
		for _, id := range ready {
			for _, c := range children[id] {
				indeg[c]--
				if indeg[c] == 0 {
					next = append(next, c)
				}
			}
		}
		sort.Strings(next)
		ready = next
	}
	if scheduled != len(byID) {
		return Plan{}, ErrCycle
	}
	return plan, nil
}

// LinearOrder returns the nodes in a single sequence respecting the
// topological order — exactly what the walking-skeleton linear runtime
// expects. Returns ErrCycle on a cyclic DAG.
func (p Plan) LinearOrder() []NodeSpec {
	var out []NodeSpec
	for _, stage := range p.Stages {
		out = append(out, stage...)
	}
	return out
}
