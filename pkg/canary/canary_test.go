package canary_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/aegisvision/pkg/canary"
	"github.com/aegisvision/pkg/intelligence"
)

func TestVariant_DeterministicSplit(t *testing.T) {
	keys := []string{"stream-a", "stream-b", "stream-c", "stream-d", "stream-e"}
	for _, k := range keys {
		v1 := canary.Variant(k, 10)
		v2 := canary.Variant(k, 10)
		if v1 != v2 {
			t.Fatalf("non-deterministic split for %s: %s vs %s", k, v1, v2)
		}
	}
}

func TestVariant_PercentageBoundaries(t *testing.T) {
	// 0% → always baseline.
	for i := 0; i < 50; i++ {
		if canary.Variant(fmt.Sprintf("k-%d", i), 0) != "baseline" {
			t.Fatal("0% should always be baseline")
		}
	}
	// 100% → always candidate.
	for i := 0; i < 50; i++ {
		if canary.Variant(fmt.Sprintf("k-%d", i), 100) != "candidate" {
			t.Fatal("100% should always be candidate")
		}
	}
}

func TestVariant_SplitApproximatesPercentage(t *testing.T) {
	const N = 10000
	const pct = 25
	candidate := 0
	for i := 0; i < N; i++ {
		if canary.Variant(fmt.Sprintf("key-%d", i), pct) == "candidate" {
			candidate++
		}
	}
	ratio := float64(candidate) / N
	if ratio < 0.22 || ratio > 0.28 {
		t.Fatalf("split ratio off: %f", ratio)
	}
}

func TestAssessRegression_FlagsRealRegression(t *testing.T) {
	// Baseline: 9800 / 10000 = 98% success.
	// Candidate: 9500 / 10000 = 95% success — clearly worse.
	v := canary.AssessRegression(canary.RegressionInput{
		Baseline:           canary.Observation{Successes: 9800, Failures: 200},
		Candidate:          canary.Observation{Successes: 9500, Failures: 500},
		MaxProportionDelta: 0.005,
		MinSamples:         1000,
	})
	if v.Healthy {
		t.Fatalf("expected regression, got: %+v", v)
	}
	if v.Reason != "proportion_regression" {
		t.Fatalf("reason: %s", v.Reason)
	}
}

func TestAssessRegression_AcceptsNonInferior(t *testing.T) {
	// Both ~98%. The Wilson lower bound for candidate is ~0.976; baseline
	// point estimate is 0.98; delta 0.004 < 0.005 — should be healthy.
	v := canary.AssessRegression(canary.RegressionInput{
		Baseline:           canary.Observation{Successes: 9800, Failures: 200},
		Candidate:          canary.Observation{Successes: 9790, Failures: 210},
		MaxProportionDelta: 0.005,
		MinSamples:         1000,
	})
	if !v.Healthy {
		t.Fatalf("expected healthy, got: %+v", v)
	}
}

func TestAssessRegression_InsufficientSamples(t *testing.T) {
	v := canary.AssessRegression(canary.RegressionInput{
		Baseline:           canary.Observation{Successes: 50, Failures: 0},
		Candidate:          canary.Observation{Successes: 5, Failures: 5},
		MaxProportionDelta: 0.005,
		MinSamples:         1000,
	})
	if v.Healthy || v.Reason != "insufficient_samples" {
		t.Fatalf("expected insufficient_samples, got: %+v", v)
	}
}

func TestAssessRegression_LatencyRegression(t *testing.T) {
	v := canary.AssessRegression(canary.RegressionInput{
		Baseline:           canary.Observation{Successes: 9800, Failures: 200, P95LatencyMs: 100},
		Candidate:          canary.Observation{Successes: 9790, Failures: 210, P95LatencyMs: 250},
		MaxProportionDelta: 0.01,
		MaxLatencyP95Ms:    50,
		MinSamples:         1000,
	})
	if v.Healthy || v.Reason != "latency_regression" {
		t.Fatalf("expected latency_regression, got: %+v", v)
	}
}

func TestNext_AdvancesWhenHealthyAndHoldElapsed(t *testing.T) {
	plan := intelligence.CanaryPlan{
		Ladder: []intelligence.CanaryStep{
			{CandidatePercent: 5, HoldSeconds: 60, MinSamples: 100},
			{CandidatePercent: 25, HoldSeconds: 60, MinSamples: 100},
		},
		CurrentStep: 0,
		Status:      intelligence.CanaryStatusRamping,
		LastStepAt:  time.Now().Add(-2 * time.Minute),
	}
	d := canary.Next(plan, canary.Verdict{Healthy: true, Reason: "non_inferior"}, time.Now())
	if d.NewStatus != intelligence.CanaryStatusRamping || d.NewStep != 1 {
		t.Fatalf("decision: %+v", d)
	}
}

func TestNext_RollsBackOnRegression(t *testing.T) {
	plan := intelligence.CanaryPlan{
		Ladder:      []intelligence.CanaryStep{{CandidatePercent: 25, HoldSeconds: 60, MinSamples: 100}},
		CurrentStep: 0,
		Status:      intelligence.CanaryStatusRamping,
		LastStepAt:  time.Now(),
	}
	d := canary.Next(plan, canary.Verdict{Healthy: false, Reason: "proportion_regression"}, time.Now())
	if d.NewStatus != intelligence.CanaryStatusRolledBack {
		t.Fatalf("expected rollback, got: %+v", d)
	}
}

func TestNext_GatesOnAutoPromote(t *testing.T) {
	plan := intelligence.CanaryPlan{
		Ladder: []intelligence.CanaryStep{
			{CandidatePercent: 50, HoldSeconds: 30, MinSamples: 100},
		},
		CurrentStep: 0, AutoPromote: true,
		Status:     intelligence.CanaryStatusRamping,
		LastStepAt: time.Now().Add(-1 * time.Minute),
	}
	d := canary.Next(plan, canary.Verdict{Healthy: true, Reason: "non_inferior"}, time.Now())
	if d.NewStatus != intelligence.CanaryStatusPromoted || !d.GateRequired {
		t.Fatalf("expected gated promote, got: %+v", d)
	}
}
