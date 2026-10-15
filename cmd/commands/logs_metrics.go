package commands

// Purpose: Per-service log analysis split out of logs.go (CLI-R12 Batch B
// mechanical file-size split). Backs `nself logs --summary` and `--top` by
// fetching/sampling docker compose logs and computing error/warning counts
// and volume rates.
// Inputs: a context, project workdir, service name, and either a max-lines
// tail count (CollectLogSummary) or a sample duration (SampleLogVolume).
// Outputs: LogSummary (totals, top error messages, last line) or LogVolume
// (lines/bytes per minute, error rate).
// Constraints: pure move, no behavior change. Consumed by
// runLogsSummary/runLogsTop in logs_reports.go.

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/docker"
)

// LogSummary holds per-service log analysis results.
type LogSummary struct {
	Service    string
	TotalLines int
	ErrorCount int
	WarnCount  int
	TopErrors  []string // up to 5 most frequent error messages (deduped)
	LastLine   string
}

// CollectLogSummary fetches and analyses recent logs for a single service.
func CollectLogSummary(ctx context.Context, workdir string, service string, maxLines int) (*LogSummary, error) {
	summary := &LogSummary{Service: service}

	out, err := docker.NewCompose().LogsOutput(ctx, workdir,
		"--tail", strconv.Itoa(maxLines),
		"--no-log-prefix",
		service,
	)
	if err != nil {
		// Service may not be running — return empty summary, no error.
		return summary, nil
	}

	errorFreq := make(map[string]int)
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		summary.TotalLines++
		summary.LastLine = line

		level := detectLineLevel(line)
		switch level {
		case "error":
			summary.ErrorCount++
			// Strip timestamp for dedup key
			key := timestampPrefixRE.ReplaceAllString(line, "")
			errorFreq[key]++
		case "warn":
			summary.WarnCount++
		}
	}

	// Build top 5 errors sorted by frequency descending.
	type errEntry struct {
		msg   string
		count int
	}
	entries := make([]errEntry, 0, len(errorFreq))
	for msg, cnt := range errorFreq {
		entries = append(entries, errEntry{msg, cnt})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count > entries[j].count
	})
	max := 5
	if len(entries) < max {
		max = len(entries)
	}
	for i := 0; i < max; i++ {
		summary.TopErrors = append(summary.TopErrors, entries[i].msg)
	}

	return summary, nil
}

// LogVolume holds per-service log volume sample data.
type LogVolume struct {
	Service     string
	LinesPerMin float64
	BytesPerMin float64
	ErrorRate   float64 // errors / total lines in sample window
}

// SampleLogVolume samples docker logs for a service over sampleDuration and computes volume metrics.
func SampleLogVolume(ctx context.Context, workdir string, service string, sampleDuration time.Duration) (*LogVolume, error) {
	vol := &LogVolume{Service: service}

	sinceSeconds := int(sampleDuration.Seconds())
	out, err := docker.NewCompose().LogsOutput(ctx, workdir,
		"--since", strconv.Itoa(sinceSeconds)+"s",
		"--no-log-prefix",
		service,
	)
	if err != nil {
		// Service may not be running — return empty volume, no error.
		return vol, nil
	}

	totalLines := 0
	totalBytes := len(out)
	errorLines := 0
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		totalLines++
		if detectLineLevel(line) == "error" {
			errorLines++
		}
	}

	minutes := sampleDuration.Minutes()
	if minutes > 0 && totalLines > 0 {
		vol.LinesPerMin = float64(totalLines) / minutes
		vol.BytesPerMin = float64(totalBytes) / minutes
	}
	if totalLines > 0 {
		vol.ErrorRate = float64(errorLines) / float64(totalLines)
	}

	return vol, nil
}
