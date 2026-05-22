package sampler_test

import (
	"math"
	"testing"

	"github.com/aegisvision/pkg/intelligence"
	"github.com/aegisvision/services/active-learning-service/internal/sampler"
)

func TestScore_LeastConfidence(t *testing.T) {
	got := sampler.Score(intelligence.UncertaintyLeastConfidence, []float64{0.7, 0.2, 0.1})
	if math.Abs(got-0.3) > 1e-6 {
		t.Fatalf("got %f", got)
	}
}

func TestScore_Margin(t *testing.T) {
	got := sampler.Score(intelligence.UncertaintyMargin, []float64{0.55, 0.40, 0.05})
	if math.Abs(got-(1-0.15)) > 1e-6 {
		t.Fatalf("got %f", got)
	}
}

func TestScore_Entropy(t *testing.T) {
	// Uniform distribution should hit normalized entropy = 1.
	uniform := []float64{0.25, 0.25, 0.25, 0.25}
	if got := sampler.Score(intelligence.UncertaintyEntropy, uniform); math.Abs(got-1) > 1e-6 {
		t.Fatalf("uniform entropy: %f", got)
	}
	// Delta distribution should be 0.
	if got := sampler.Score(intelligence.UncertaintyEntropy, []float64{1, 0, 0, 0}); got != 0 {
		t.Fatalf("delta entropy: %f", got)
	}
}

func TestBudgeter_RespectsCaps(t *testing.T) {
	b := sampler.NewBudgeter()
	policy := intelligence.LearningPolicy{
		DailyBudget: 4, DiversityClasses: []string{"person", "vehicle"},
	}
	// 2 per class allowed (4/2).
	for i := 0; i < 2; i++ {
		_, ok := b.Try("person", policy)
		if !ok {
			t.Fatalf("person try %d should accept", i)
		}
	}
	if _, ok := b.Try("person", policy); ok {
		t.Fatal("expected per_class_cap on 3rd person")
	}
	for i := 0; i < 2; i++ {
		_, ok := b.Try("vehicle", policy)
		if !ok {
			t.Fatalf("vehicle try %d should accept", i)
		}
	}
	if _, ok := b.Try("vehicle", policy); ok {
		t.Fatal("expected daily_budget_exhausted on 5th total")
	}
}

func TestInBand(t *testing.T) {
	p := intelligence.LearningPolicy{MinUncertainty: 0.2, MaxUncertainty: 0.7}
	if !sampler.InBand(0.5, p) {
		t.Fatal("0.5 should be in band")
	}
	if sampler.InBand(0.1, p) || sampler.InBand(0.9, p) {
		t.Fatal("0.1/0.9 should be out of band")
	}
}
