// Divergence functions used by drift-detection-service. KL, JS, and TVD
// over discrete class distributions.
//
// References:
//   Kullback, S.; Leibler, R. A. (1951). On information and sufficiency.
//   Lin, J. (1991). Divergence measures based on the Shannon entropy.
package autonomy

import (
	"math"

	"github.com/aegisvision/pkg/intelligence"
)

// Normalize returns a copy of dist with values scaled so the values sum
// to 1. Zero-mass keys are pruned. A zero-sum input returns an empty
// map (callers should treat that as "no signal").
func Normalize(d map[string]float64) map[string]float64 {
	sum := 0.0
	for _, v := range d {
		if v > 0 {
			sum += v
		}
	}
	if sum == 0 {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(d))
	for k, v := range d {
		if v > 0 {
			out[k] = v / sum
		}
	}
	return out
}

// keyspace returns the union of keys with the smoothing pad applied.
func smooth(a, b map[string]float64, eps float64) (pa, pb map[string]float64) {
	keys := map[string]struct{}{}
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	pa = make(map[string]float64, len(keys))
	pb = make(map[string]float64, len(keys))
	for k := range keys {
		pa[k] = a[k] + eps
		pb[k] = b[k] + eps
	}
	return Normalize(pa), Normalize(pb)
}

// KL returns the Kullback-Leibler divergence D(p || q). With epsilon
// smoothing to avoid log(0) blow-ups.
func KL(p, q map[string]float64) float64 {
	pp, qq := smooth(p, q, 1e-9)
	var d float64
	for k, pk := range pp {
		qk := qq[k]
		if pk > 0 && qk > 0 {
			d += pk * math.Log(pk/qk)
		}
	}
	return d
}

// JS returns the Jensen-Shannon divergence (symmetric, bounded by log 2
// in natural log). We return the *base-2* JS so the value is in [0, 1].
func JS(p, q map[string]float64) float64 {
	pp, qq := smooth(p, q, 1e-9)
	m := map[string]float64{}
	for k, pk := range pp {
		m[k] = 0.5*pk + 0.5*qq[k]
	}
	for k, qk := range qq {
		if _, ok := m[k]; !ok {
			m[k] = 0.5*qk + 0.5*pp[k]
		}
	}
	d := 0.5*KL(pp, m) + 0.5*KL(qq, m)
	return d / math.Log(2)
}

// TVD returns the total variation distance, 0..1.
func TVD(p, q map[string]float64) float64 {
	pp, qq := smooth(p, q, 0)
	keys := map[string]struct{}{}
	for k := range pp {
		keys[k] = struct{}{}
	}
	for k := range qq {
		keys[k] = struct{}{}
	}
	var d float64
	for k := range keys {
		d += math.Abs(pp[k] - qq[k])
	}
	return 0.5 * d
}

// Divergence dispatches on method.
func Divergence(method intelligence.DivergenceMethod, p, q map[string]float64) float64 {
	switch method {
	case intelligence.DivergenceJS:
		return JS(p, q)
	case intelligence.DivergenceTVD:
		return TVD(p, q)
	default:
		return KL(p, q)
	}
}

// BurnRate computes the SLO burn rate per the Google SRE workbook:
//
//	burn = (1 - observed_success_ratio) / (1 - target)
//
// A burn rate of 1.0 means the error budget is being consumed at the
// rate that would exactly exhaust it over the rolling window. >1.0 burns
// faster.
func BurnRate(observed, target float64) float64 {
	budget := 1 - target
	if budget <= 0 {
		return 0
	}
	loss := 1 - observed
	if loss < 0 {
		return 0
	}
	return loss / budget
}

// MultiWindow categorizes the severity of a multi-window burn-rate
// reading per Google SRE workbook table 5-7.
//
//	fast (1h) >= 14 AND slow (6h) >= 6  → critical (page)
//	fast (1h) >= 6  AND slow (6h) >= 3  → warn (ticket)
//	fast (24h) >= 1                     → info (track)
func MultiWindow(burn1h, burn6h, burn24h float64) string {
	if burn1h >= 14 && burn6h >= 6 {
		return "critical"
	}
	if burn1h >= 6 && burn6h >= 3 {
		return "warn"
	}
	if burn24h >= 1 {
		return "info"
	}
	return ""
}
