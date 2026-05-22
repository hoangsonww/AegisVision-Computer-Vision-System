package predictor_test

import (
	"testing"
	"time"

	"github.com/aegisvision/services/prefetch-service/internal/predictor"
)

func TestPredictor_ConvergesViaEMA(t *testing.T) {
	p := predictor.New(0.5)
	now := time.Date(2026, 5, 21, 14, 0, 0, 0, time.UTC) // Thursday 14:00
	p.SetNow(func() time.Time { return now })
	k := predictor.Key{Tenant: "t-1", Model: "yolo-v9"}
	// First observation pushes EMA to 0.5*1000 = 500.
	p.Observe(now, k, 1000)
	pred := p.Predict(0, 5)
	if len(pred) != 1 || pred[0].Expected < 400 || pred[0].Expected > 600 {
		t.Fatalf("EMA after 1 obs: %+v", pred)
	}
	// More observations should converge toward 1000.
	for i := 0; i < 20; i++ {
		p.Observe(now, k, 1000)
	}
	pred = p.Predict(0, 5)
	if pred[0].Expected < 950 {
		t.Fatalf("EMA after 20 obs: %f", pred[0].Expected)
	}
}

func TestPredictor_HorizonLandsInCorrectCell(t *testing.T) {
	p := predictor.New(1.0) // pure replace
	now := time.Date(2026, 5, 21, 14, 0, 0, 0, time.UTC) // Thursday 14:00
	p.SetNow(func() time.Time { return now })
	hot := predictor.Key{Tenant: "rush", Model: "yolo"}
	cool := predictor.Key{Tenant: "quiet", Model: "yolo"}
	// 14:00 is hot.
	p.Observe(now, hot, 5000)
	// 15:00 is cool.
	p.Observe(now.Add(1*time.Hour), cool, 1)

	// Predicting at horizon=0 (still 14:00) should rank hot first.
	hotPred := p.Predict(0, 5)
	if len(hotPred) == 0 || hotPred[0].Key.Tenant != "rush" {
		t.Fatalf("expected rush first: %+v", hotPred)
	}
	// Predicting at horizon=1h (15:00) should rank cool first.
	coolPred := p.Predict(1*time.Hour, 5)
	if len(coolPred) == 0 || coolPred[0].Key.Tenant != "quiet" {
		t.Fatalf("expected quiet first: %+v", coolPred)
	}
}

func TestPredictor_TopNCap(t *testing.T) {
	p := predictor.New(1.0)
	now := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	p.SetNow(func() time.Time { return now })
	for i := 0; i < 20; i++ {
		p.Observe(now, predictor.Key{Tenant: "t", Model: string(rune('a' + i))}, float64(i+1))
	}
	out := p.Predict(0, 5)
	if len(out) != 5 {
		t.Fatalf("expected 5, got %d", len(out))
	}
	// Sorted descending by expected.
	for i := 1; i < len(out); i++ {
		if out[i].Expected > out[i-1].Expected {
			t.Fatalf("not sorted desc: %+v", out)
		}
	}
}
