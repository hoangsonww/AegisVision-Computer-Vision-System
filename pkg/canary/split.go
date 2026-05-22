// Package canary implements traffic-splitting + statistical regression
// detection for the AegisVision canary controller (ADR-0023).
//
// Two responsibilities:
//
//  1. Deterministic traffic-split. Given (tenant, stream/request-key,
//     candidate-percent), Variant() returns "baseline" or "candidate" such
//     that the same key always lands on the same variant — a single
//     stream's detections are not interleaved between models, which would
//     break tracking continuity.
//
//  2. Statistical regression detection. Given an observation window for
//     baseline and candidate (successes, failures, latency p95), decide
//     whether the candidate is *worse* with confidence.
//
//     We use a one-sided proportion test based on Wilson confidence
//     intervals — pragmatic, parameter-free, and avoids the false-positive
//     storms that naive p-value tests produce. Combined with a hard
//     minimum-sample-size gate.
package canary

import (
	"hash/fnv"
	"math"
)

// Variant returns "baseline" or "candidate" for the given splitKey and
// percentage of traffic that should go to the candidate (0..100).
//
// We use FNV-1a so two services on different hosts derive the same
// variant for the same splitKey. The deterministic mapping means a
// running pipeline does not flip between models mid-session.
func Variant(splitKey string, candidatePercent int) string {
	if candidatePercent <= 0 {
		return "baseline"
	}
	if candidatePercent >= 100 {
		return "candidate"
	}
	h := fnv.New32a()
	h.Write([]byte(splitKey))
	bucket := int(h.Sum32() % 100)
	if bucket < candidatePercent {
		return "candidate"
	}
	return "baseline"
}

// Observation is the input to the regression detector. Counts are within
// a single observation window for one variant.
type Observation struct {
	Successes int64
	Failures  int64
	// Latency p95 in milliseconds (best-effort; 0 disables latency check).
	P95LatencyMs int64
}

// RegressionInput packages baseline + candidate observations along with
// the plan's tolerance parameters.
type RegressionInput struct {
	Baseline           Observation
	Candidate          Observation
	MaxProportionDelta float64 // e.g. 0.005 (= 0.5 percentage points)
	MaxLatencyP95Ms    int     // e.g. 50 ms
	MinSamples         int64
}

// Verdict from the regression detector.
type Verdict struct {
	// Is the candidate good enough to advance?
	Healthy bool
	// Free-form reason — useful for the controller's audit trail.
	Reason string
	// Observed success rates.
	BaselineRate  float64
	CandidateRate float64
	// 95% lower bound of the candidate success rate (Wilson).
	CandidateLowerBound float64
}

// AssessRegression returns whether the candidate appears non-worse than
// the baseline within the plan's tolerance. False means *the candidate is
// likely worse*; callers should treat that as a rollback signal.
//
// Logic:
//
//  1. Insist on enough samples in BOTH variants. Below the floor the
//     verdict is healthy=false with reason "insufficient_samples" — the
//     controller's holding loop should keep waiting, not advance.
//  2. Compute success-rate point estimates.
//  3. If candidate's *lower* 95% Wilson bound is more than
//     MaxProportionDelta below the baseline's point estimate, regression.
//  4. If MaxLatencyP95Ms > 0 and candidate's p95 latency is more than
//     that delta above baseline's, regression.
func AssessRegression(in RegressionInput) Verdict {
	bn := in.Baseline.Successes + in.Baseline.Failures
	cn := in.Candidate.Successes + in.Candidate.Failures
	if bn < in.MinSamples || cn < in.MinSamples {
		return Verdict{Healthy: false, Reason: "insufficient_samples"}
	}
	br := safeRate(in.Baseline.Successes, bn)
	cr := safeRate(in.Candidate.Successes, cn)
	lo := wilsonLowerBound(in.Candidate.Successes, cn)
	v := Verdict{BaselineRate: br, CandidateRate: cr, CandidateLowerBound: lo}
	if br-lo > in.MaxProportionDelta {
		v.Healthy = false
		v.Reason = "proportion_regression"
		return v
	}
	if in.MaxLatencyP95Ms > 0 && in.Candidate.P95LatencyMs > 0 && in.Baseline.P95LatencyMs > 0 {
		if in.Candidate.P95LatencyMs-in.Baseline.P95LatencyMs > int64(in.MaxLatencyP95Ms) {
			v.Healthy = false
			v.Reason = "latency_regression"
			return v
		}
	}
	v.Healthy = true
	v.Reason = "non_inferior"
	return v
}

func safeRate(success, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total)
}

// wilsonLowerBound returns the lower bound of the 95% Wilson score
// confidence interval for a success proportion. This is the standard
// "lower bound of Wilson score" used in (e.g.) Reddit's hot-rank — it
// behaves sensibly with small n where the normal approximation breaks.
//
// References:
//   Wilson, E.B. (1927). "Probable Inference, the Law of Succession,
//   and Statistical Inference."
func wilsonLowerBound(success, total int64) float64 {
	if total <= 0 {
		return 0
	}
	const z = 1.96 // 95% two-sided; lower bound is the lower of the two.
	n := float64(total)
	p := float64(success) / n
	z2 := z * z
	denom := 1 + z2/n
	center := p + z2/(2*n)
	margin := z * math.Sqrt((p*(1-p)+z2/(4*n))/n)
	lb := (center - margin) / denom
	if lb < 0 {
		lb = 0
	}
	return lb
}
