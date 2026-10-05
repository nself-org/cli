package main

import (
	"math"
	"sort"
)

// percentile returns the nearest-rank p-th percentile (0 < p <= 100) of
// samples. The slice is not modified. An empty slice yields 0.
func percentile(samples []float64, p float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	s := append([]float64(nil), samples...)
	sort.Float64s(s)
	rank := int(math.Ceil(p / 100 * float64(len(s))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(s) {
		rank = len(s)
	}
	return s[rank-1]
}

// round1 rounds to one decimal place.
func round1(v float64) float64 { return math.Round(v*10) / 10 }

// summarize turns raw samples of one metric into the JSON metric: nearest-rank
// p50, p95 and max, each rounded to 0.1.
func summarize(name, unit string, samples []float64) Metric {
	max := 0.0
	for i, v := range samples {
		if i == 0 || v > max {
			max = v
		}
	}
	return Metric{
		Name: name,
		Unit: unit,
		P50:  round1(percentile(samples, 50)),
		P95:  round1(percentile(samples, 95)),
		Max:  round1(max),
		N:    len(samples),
	}
}
