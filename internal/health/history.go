package health

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// historyFile is the filename used inside the history directory.
const historyFile = "history.jsonl"

// historyServiceEntry is the JSON shape for one service in a JSONL line.
type historyServiceEntry struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
}

// historySummary is the JSON shape for the summary block in a JSONL line.
type historySummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
}

// historyLine is the JSON shape for one complete JSONL line.  It maps
// to/from HealthReport for persistence.
type historyLine struct {
	Timestamp string                `json:"timestamp"`
	Services  []historyServiceEntry `json:"services"`
	Summary   historySummary        `json:"summary"`
}

// toLine converts a HealthReport to the JSONL representation.
func toLine(r *HealthReport) historyLine {
	services := make([]historyServiceEntry, len(r.Results))
	for i, res := range r.Results {
		services[i] = historyServiceEntry{
			Name:       res.Service,
			Status:     res.Status,
			DurationMs: res.Duration.Milliseconds(),
		}
	}
	return historyLine{
		Timestamp: r.Timestamp.UTC().Format(time.RFC3339),
		Services:  services,
		Summary: historySummary{
			Total:     r.Total,
			Healthy:   r.Healthy,
			Unhealthy: r.Unhealthy,
		},
	}
}

// fromLine converts a JSONL representation back to a HealthReport.
func fromLine(l historyLine) HealthReport {
	results := make([]HealthResult, len(l.Services))
	for i, s := range l.Services {
		results[i] = HealthResult{
			Service:  s.Name,
			Status:   s.Status,
			Duration: time.Duration(s.DurationMs) * time.Millisecond,
		}
	}
	var report HealthReport
	report.Results = results
	report.Total = l.Summary.Total
	report.Healthy = l.Summary.Healthy
	report.Unhealthy = l.Summary.Unhealthy
	// Best-effort timestamp parse; zero value on failure.
	if t, err := time.Parse(time.RFC3339, l.Timestamp); err == nil {
		report.Timestamp = t
	}
	return report
}

// SaveHistory appends a single HealthReport as one JSON line to
// {historyDir}/history.jsonl.  The directory is created if it does not
// exist.
func SaveHistory(report *HealthReport, historyDir string) error {
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(toLine(report))
	if err != nil {
		return err
	}

	f, err := os.OpenFile(
		filepath.Join(historyDir, historyFile),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	// Write JSON line followed by newline.
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

// GetHistory reads the last limit HealthReport entries from
// {historyDir}/history.jsonl.  If the file does not exist an empty
// slice is returned.  When limit <= 0 all entries are returned.
func GetHistory(historyDir string, limit int) ([]HealthReport, error) {
	p := filepath.Join(historyDir, historyFile)

	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. No history file means no health reports recorded yet —
			// true on every project before the first check, and identical to
			// an empty file. ENOENT only; other open failures propagate.
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	// Read all lines first, then take the tail.  Health history files
	// are expected to stay small (hundreds of entries at most).
	var all []HealthReport
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var hl historyLine
		if err := json.Unmarshal(line, &hl); err != nil {
			// Skip malformed lines rather than failing the entire read.
			continue
		}
		all = append(all, fromLine(hl))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}

	return all, nil
}
