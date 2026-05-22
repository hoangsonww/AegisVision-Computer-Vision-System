package shadow_test

import (
	"context"
	"testing"
	"time"

	"github.com/aegisvision/services/shadow-inference-service/internal/shadow"
)

type stubInvoker struct {
	predicted string
}

func (s stubInvoker) Invoke(_ context.Context, _, _ string) (shadow.BaselineResult, error) {
	return shadow.BaselineResult{Predicted: s.predicted, Confidence: 0.9}, nil
}

func TestEngine_AgreementCountsAsSuccess(t *testing.T) {
	e := shadow.NewEngine(stubInvoker{predicted: "person"})
	e.WindowDur = 500 * time.Millisecond
	e.SetActive([]shadow.Active{{
		PlanID: "p-1", TenantID: "", CandidateModel: "yolo-v9",
		CandidatePercent: 100, // always pick the candidate path in this test
	}})
	for i := 0; i < 10; i++ {
		urn := "frame://" + time.Now().Add(time.Duration(i)*time.Microsecond).String()
		e.OnBaseline(context.Background(), shadow.BaselineResult{
			TenantID: "t-1", StreamID: "s-1", FrameURN: urn,
			Predicted: "person", Confidence: 0.95, LatencyMs: 80,
		})
		e.OnFrame(context.Background(), shadow.FrameDescriptor{
			TenantID: "t-1", StreamID: "s-1", FrameURN: urn, OccurredAt: time.Now(),
		})
	}
	// Let async invokeAndRecord finish.
	time.Sleep(200 * time.Millisecond)
	// Advance time past the window so Drain emits.
	obs := e.Drain(time.Now().Add(2 * time.Second))
	if len(obs) != 2 {
		t.Fatalf("expected 2 obs (baseline + candidate), got %d", len(obs))
	}
	totalCandidate := int64(0)
	totalBaseline := int64(0)
	for _, o := range obs {
		if o.Variant == "candidate" {
			totalCandidate = o.Successes + o.Failures
			if o.Failures != 0 {
				t.Fatalf("candidate should agree on all: %+v", o)
			}
		} else {
			totalBaseline = o.Successes + o.Failures
		}
	}
	if totalCandidate != 10 || totalBaseline != 10 {
		t.Fatalf("expected 10 each, got cand=%d base=%d", totalCandidate, totalBaseline)
	}
}

func TestEngine_DisagreementCountsAsFailure(t *testing.T) {
	e := shadow.NewEngine(stubInvoker{predicted: "vehicle"})
	e.WindowDur = 500 * time.Millisecond
	e.SetActive([]shadow.Active{{
		PlanID: "p-1", TenantID: "", CandidatePercent: 100,
	}})
	for i := 0; i < 5; i++ {
		urn := "f" + time.Now().Add(time.Duration(i)*time.Microsecond).String()
		e.OnBaseline(context.Background(), shadow.BaselineResult{
			TenantID: "t-1", StreamID: "s-1", FrameURN: urn,
			Predicted: "person",
		})
		e.OnFrame(context.Background(), shadow.FrameDescriptor{
			TenantID: "t-1", StreamID: "s-1", FrameURN: urn,
		})
	}
	time.Sleep(200 * time.Millisecond)
	obs := e.Drain(time.Now().Add(2 * time.Second))
	for _, o := range obs {
		if o.Variant == "candidate" && o.Failures != 5 {
			t.Fatalf("candidate should disagree on all 5: %+v", o)
		}
	}
}

func TestEngine_NoMatchingBaselineSkips(t *testing.T) {
	e := shadow.NewEngine(stubInvoker{predicted: "person"})
	e.SetActive([]shadow.Active{{
		PlanID: "p-1", TenantID: "", CandidatePercent: 100,
	}})
	// Frame arrives but baseline never did — should produce zero counters.
	e.OnFrame(context.Background(), shadow.FrameDescriptor{TenantID: "t-1", StreamID: "s-1", FrameURN: "no-baseline"})
	obs := e.Drain(time.Now().Add(2 * time.Minute))
	for _, o := range obs {
		if o.Successes+o.Failures != 0 {
			t.Fatalf("expected zero counts, got: %+v", o)
		}
	}
}

func TestEngine_PercentageGate(t *testing.T) {
	// Two assignments — 0% should never select, 100% always.
	e0 := shadow.NewEngine(stubInvoker{predicted: "x"})
	e0.SetActive([]shadow.Active{{PlanID: "p-0", CandidatePercent: 0}})
	e0.OnBaseline(context.Background(), shadow.BaselineResult{FrameURN: "f1", Predicted: "x"})
	e0.OnFrame(context.Background(), shadow.FrameDescriptor{StreamID: "s-1", FrameURN: "f1"})
	time.Sleep(50 * time.Millisecond)
	obs := e0.Drain(time.Now().Add(2 * time.Minute))
	for _, o := range obs {
		if o.Successes+o.Failures > 0 {
			t.Fatalf("0%% should never select: %+v", o)
		}
	}
}
