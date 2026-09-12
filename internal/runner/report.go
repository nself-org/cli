package runner

// Purpose: run verify across N hosts and turn the per-host CheckResult sets
//   into a single parity matrix, so "same GitHub Actions runner labels,
//   different tools installed" (the exact 2026-09-11 gh/zip/unzip
//   incident) shows up as one glance at a table instead of N separate
//   verify runs a human has to compare by eye.
// Inputs:  a Manifest and one Executor per host to check.
// Outputs: []HostReport (one per host) plus DetectDrift's []DriftFinding
//   and RenderMatrix's plain-text table.
// Constraints: an unreachable host is reported once (HostReport.Err) and
//   excluded from drift comparison — comparing "unreachable" against every
//   real status would report every check as drifted, burying the actual
//   finding.

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// HostReport is one host's full verify result.
type HostReport struct {
	Host   string        `json:"host"`
	Checks []CheckResult `json:"checks,omitempty"`
	// Err is set when the host itself could not be reached at all (SSH
	// connection failure), as opposed to an individual check failing.
	Err string `json:"error,omitempty"`
}

// VerifyHosts runs a cheap reachability probe against each executor, then
// the full VerifyHost check set against every host that answered. Hosts
// that don't answer get a single HostReport with Err set and no Checks,
// rather than N near-identical "connection refused" failures.
func VerifyHosts(ctx context.Context, executors []Executor, m *Manifest) []HostReport {
	reports := make([]HostReport, len(executors))
	for i, ex := range executors {
		if _, err := ex.Run(ctx, "echo reachable"); err != nil {
			reports[i] = HostReport{Host: ex.Label(), Err: err.Error()}
			continue
		}
		reports[i] = HostReport{Host: ex.Label(), Checks: VerifyHost(ctx, ex, m)}
	}
	return reports
}

// DriftFinding names one check that disagrees across two or more reachable
// hosts — the thing `nself runner verify` exists to surface.
type DriftFinding struct {
	CheckName string                 `json:"check"`
	ByHost    map[string]CheckStatus `json:"by_host"`
}

// DetectDrift compares every check name present on any reachable host and
// reports the ones whose status differs across hosts. Manifest checks are
// identical across hosts by construction, so any disagreement here is real
// drift, not a different question being asked.
func DetectDrift(reports []HostReport) []DriftFinding {
	byCheck := map[string]map[string]CheckStatus{}
	var order []string
	for _, r := range reports {
		if r.Err != "" {
			continue
		}
		for _, c := range r.Checks {
			if _, ok := byCheck[c.Name]; !ok {
				byCheck[c.Name] = map[string]CheckStatus{}
				order = append(order, c.Name)
			}
			byCheck[c.Name][r.Host] = c.Status
		}
	}
	var findings []DriftFinding
	for _, name := range order {
		statuses := byCheck[name]
		if !allEqual(statuses) {
			findings = append(findings, DriftFinding{CheckName: name, ByHost: statuses})
		}
	}
	return findings
}

func allEqual(statuses map[string]CheckStatus) bool {
	first := ""
	for _, st := range statuses {
		if first == "" {
			first = string(st)
			continue
		}
		if string(st) != first {
			return false
		}
	}
	return true
}

// RenderMatrix formats reports as a plain-text parity matrix: one row per
// check, one column per host, plus a trailing drift summary. This is the
// primary human-facing output of `nself runner verify` when checking more
// than one host.
func RenderMatrix(reports []HostReport) string {
	var b strings.Builder
	hosts := make([]string, len(reports))
	for i, r := range reports {
		hosts[i] = r.Host
	}

	checkNames := collectCheckNames(reports)
	statusOf := indexStatuses(reports)

	fmt.Fprintf(&b, "%-40s", "CHECK")
	for _, h := range hosts {
		fmt.Fprintf(&b, "  %-12s", truncate(h, 12))
	}
	b.WriteString("\n")

	for _, name := range checkNames {
		fmt.Fprintf(&b, "%-40s", truncate(name, 40))
		for _, h := range hosts {
			b.WriteString("  ")
			b.WriteString(cellSymbol(statusOf, name, h))
			b.WriteString(strings.Repeat(" ", 10))
		}
		b.WriteString("\n")
	}

	drift := DetectDrift(reports)
	if len(drift) == 0 {
		b.WriteString("\nNo drift detected across reachable hosts.\n")
	} else {
		fmt.Fprintf(&b, "\nDRIFT DETECTED (%d check(s) disagree across hosts):\n", len(drift))
		for _, f := range drift {
			fmt.Fprintf(&b, "  - %s:\n", f.CheckName)
			for _, h := range hosts {
				if st, ok := f.ByHost[h]; ok {
					fmt.Fprintf(&b, "      %-20s %s\n", h, st)
				}
			}
		}
	}
	return b.String()
}

func collectCheckNames(reports []HostReport) []string {
	set := map[string]bool{}
	for _, r := range reports {
		for _, c := range r.Checks {
			set[c.Name] = true
		}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func indexStatuses(reports []HostReport) map[string]map[string]CheckStatus {
	idx := map[string]map[string]CheckStatus{}
	for _, r := range reports {
		for _, c := range r.Checks {
			if _, ok := idx[c.Name]; !ok {
				idx[c.Name] = map[string]CheckStatus{}
			}
			idx[c.Name][r.Host] = c.Status
		}
	}
	return idx
}

func cellSymbol(idx map[string]map[string]CheckStatus, checkName, host string) string {
	st, ok := idx[checkName][host]
	if !ok {
		return "?"
	}
	switch st {
	case StatusPass:
		return "OK"
	case StatusWarn:
		return "warn"
	default:
		return "FAIL"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
