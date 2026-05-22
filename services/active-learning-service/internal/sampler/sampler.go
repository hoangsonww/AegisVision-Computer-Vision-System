// Package sampler implements uncertainty + diversity sampling.
//
// Uncertainty:
//
//	least_confidence  — score = 1 - max_prob; high = uncertain
//	margin            — score = 1 - (p1 - p2); high = uncertain
//	entropy           — score = -sum(p log p), normalized 0..1
//
// Diversity bucketing: once a class hits the per-class daily budget the
// sampler stops accepting more of that class for the day. This prevents
// the "all-faces-no-objects" failure mode that wrecks downstream training.
package sampler

import (
	"math"
	"sort"

	"github.com/aegisvision/pkg/intelligence"
)

// Score computes the uncertainty score under the requested strategy. p must
// sum to ~1; we don't normalize for the caller because the upstream
// detection serializer already does (and a sub-1 sum is a signal that
// something is broken upstream).
func Score(strategy intelligence.UncertaintyStrategy, p []float64) float64 {
	if len(p) == 0 {
		return 0
	}
	switch strategy {
	case intelligence.UncertaintyMargin:
		// p1 - p2 of sorted descending.
		s := append([]float64(nil), p...)
		sort.Sort(sort.Reverse(sort.Float64Slice(s)))
		if len(s) == 1 {
			return 1 - s[0]
		}
		return 1 - (s[0] - s[1])
	case intelligence.UncertaintyEntropy:
		var H float64
		for _, x := range p {
			if x <= 0 {
				continue
			}
			H -= x * math.Log(x)
		}
		// Normalize to [0,1] using log(N) — H_max = log(N).
		Hmax := math.Log(float64(len(p)))
		if Hmax == 0 {
			return 0
		}
		return H / Hmax
	default:
		// least-confidence (also the fallback).
		maxp := 0.0
		for _, x := range p {
			if x > maxp {
				maxp = x
			}
		}
		return 1 - maxp
	}
}

// Decision is the per-event sampler verdict.
type Decision struct {
	// Score from the chosen strategy.
	Score float64
	// True if we accept this sample for queueing.
	Accept bool
	// Reason for rejection (empty when accepted).
	Reason string
}

// Budgeter tracks per-class daily counts.
type Budgeter struct {
	perClass map[string]int
	total    int
}

func NewBudgeter() *Budgeter { return &Budgeter{perClass: map[string]int{}} }

// Try increments and returns true if accepted under the policy.
func (b *Budgeter) Try(class string, policy intelligence.LearningPolicy) (Decision, bool) {
	if policy.DailyBudget > 0 && b.total >= policy.DailyBudget {
		return Decision{Reason: "daily_budget_exhausted"}, false
	}
	// Per-class soft cap: divide daily budget by number of diversity classes;
	// 0 means "no diversity cap".
	if len(policy.DiversityClasses) > 0 && policy.DailyBudget > 0 {
		perCap := policy.DailyBudget / max1(len(policy.DiversityClasses))
		if b.perClass[class] >= perCap {
			return Decision{Reason: "per_class_cap"}, false
		}
	}
	b.perClass[class]++
	b.total++
	return Decision{Accept: true}, true
}

// InBand returns true if score is in the policy's interesting band. A 0/0
// band is treated as "accept anything strictly > 0".
func InBand(score float64, policy intelligence.LearningPolicy) bool {
	lo, hi := policy.MinUncertainty, policy.MaxUncertainty
	if lo == 0 && hi == 0 {
		return score > 0
	}
	return score >= lo && score <= hi
}

func max1(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}
