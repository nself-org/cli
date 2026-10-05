package main

import (
	"io"
	"math"
	"sort"
)

// Package statuses.
const (
	statusPass      = "PASS"
	statusFail      = "FAIL"
	statusUnfloored = "UNFLOORED"
)

// PkgResult is one package of the report. Floor is null when unfloored.
type PkgResult struct {
	Package  string  `json:"package"`
	Coverage float64 `json:"coverage"`
	Floor    *int    `json:"floor"`
	Status   string  `json:"status"`
}

// Report is the verdict over a profile and a floors file. The slices are never
// nil so the JSON always carries `[]`.
type Report struct {
	Packages  []PkgResult `json:"packages"`
	Failed    []string    `json:"failed"`
	Stale     []string    `json:"stale"`
	Unfloored []string    `json:"unfloored"`
}

// Evaluate compares every package of the profile with its floor. A package
// below its floor fails; a floors line whose package has no statements in the
// profile is stale; a profiled package with no line is unfloored.
func Evaluate(cov map[string]*Cover, floors map[string]int) Report {
	r := Report{Packages: []PkgResult{}, Failed: []string{}, Stale: []string{}, Unfloored: []string{}}
	pkgs := make([]string, 0, len(cov))
	for p := range cov {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		pct := cov[p].Percent()
		res := PkgResult{Package: p, Coverage: math.Round(pct*100) / 100}
		if floor, ok := floors[p]; ok {
			f := floor
			res.Floor = &f
			if pct >= float64(floor) {
				res.Status = statusPass
			} else {
				res.Status = statusFail
				r.Failed = append(r.Failed, p)
			}
		} else {
			res.Status = statusUnfloored
			r.Unfloored = append(r.Unfloored, p)
		}
		r.Packages = append(r.Packages, res)
	}
	for p := range floors {
		if _, ok := cov[p]; !ok {
			r.Stale = append(r.Stale, p)
		}
	}
	sort.Strings(r.Stale)
	return r
}

// OK reports whether the gate passes. Unfloored packages fail it only in
// strict mode.
func (r Report) OK(strict bool) bool {
	return len(r.Failed) == 0 && len(r.Stale) == 0 && (!strict || len(r.Unfloored) == 0)
}

// PrintText writes one line per package (`PASS|FAIL|UNFLOORED <pkg> <pct>%
// floor <n>%`), the stale lines and a summary.
func (r Report) PrintText(w io.Writer, floors map[string]int, strict bool) {
	for _, p := range r.Packages {
		if p.Floor == nil {
			say(w, "%s %s %.1f%% floor none\n", p.Status, p.Package, p.Coverage)
			continue
		}
		say(w, "%s %s %.1f%% floor %d%%\n", p.Status, p.Package, p.Coverage, *p.Floor)
	}
	for _, p := range r.Stale {
		say(w, "STALE %s floor %d%% (no statements in the profile; remove the line or fix the path)\n", p, floors[p])
	}
	say(w, "covfloor: %d packages, %d below floor, %d stale, %d unfloored", len(r.Packages), len(r.Failed), len(r.Stale), len(r.Unfloored))
	if len(r.Unfloored) > 0 && strict {
		say(w, "%s", " (strict: an unfloored package fails the gate; run covfloor -write)")
	}
	sayln(w)
}
