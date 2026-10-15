package security

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/docker"
)

// WAFMode represents the Coraza WAF enforcement mode.
type WAFMode string

const (
	WAFModeOff       WAFMode = "off"
	WAFModeDetection WAFMode = "detection"
	WAFModeBlocking  WAFMode = "blocking"
)

// WAFEvent holds a single parsed WAF audit log entry.
type WAFEvent struct {
	Timestamp time.Time
	ClientIP  string
	Path      string
	Rule      string
	Severity  string
	Action    string
}

// ParseWAFEvent parses a single line from the Coraza WAF audit log. Lines that
// do not match the expected format are returned with ok=false.
//
// Expected format (space-separated, subset of Coraza Serial audit log):
//
//	[timestamp] client=<ip> path=<path> rule=<id> severity=<sev> action=<act>
func ParseWAFEvent(line string) (WAFEvent, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return WAFEvent{}, false
	}

	ev := WAFEvent{}

	// Extract bracketed timestamp.
	if open := strings.Index(line, "["); open != -1 {
		if close := strings.Index(line, "]"); close > open {
			ts, err := time.Parse("02/Jan/2006:15:04:05 -0700", line[open+1:close])
			if err == nil {
				ev.Timestamp = ts
				line = strings.TrimSpace(line[close+1:])
			}
		}
	}

	// Parse key=value fields.
	for _, field := range strings.Fields(line) {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch k {
		case "client":
			ev.ClientIP = v
		case "path":
			ev.Path = v
		case "rule":
			ev.Rule = v
		case "severity":
			ev.Severity = v
		case "action":
			ev.Action = v
		}
	}

	// Require at least a client IP or path to be considered a real event.
	if ev.ClientIP == "" && ev.Path == "" {
		return WAFEvent{}, false
	}

	return ev, true
}

// LogWAFBlock emits a structured slog event for a WAF block decision.
// This is the canonical log site for WAF block actions; all callers that need
// to record a blocked request should route through this function.
func LogWAFBlock(ctx context.Context, clientIP, path, rule string) {
	slog.WarnContext(ctx, "waf_blocked",
		"client_ip", clientIP,
		"path", path,
		"rule", rule,
	)
}

// LogWAFDetect emits a structured slog event for a WAF detection (log-only) hit.
func LogWAFDetect(ctx context.Context, clientIP, path, rule string) {
	slog.InfoContext(ctx, "waf_detected",
		"client_ip", clientIP,
		"path", path,
		"rule", rule,
	)
}

// LogWAFModeChange emits a structured slog event when the WAF mode is changed.
func LogWAFModeChange(ctx context.Context, oldMode, newMode WAFMode) {
	slog.InfoContext(ctx, "waf_mode_changed",
		"old_mode", string(oldMode),
		"new_mode", string(newMode),
	)
}

// LogWAFEnable emits a structured slog event when the WAF is first enabled.
func LogWAFEnable(ctx context.Context, projectDir string) {
	slog.InfoContext(ctx, "waf_enabled",
		"project_dir", projectDir,
		"initial_mode", string(WAFModeDetection),
	)
}

// ScanWAFAuditLog reads WAF audit log lines from r and returns parsed events.
// Lines that cannot be parsed are counted and logged at debug level.
// Returns all successfully parsed events and the count of unparseable lines.
func ScanWAFAuditLog(ctx context.Context, r io.Reader) ([]WAFEvent, int, error) {
	var events []WAFEvent
	unparseable := 0

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		ev, ok := ParseWAFEvent(line)
		if !ok {
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "#") {
				unparseable++
				slog.DebugContext(ctx, "waf_audit_line_unparseable", "line", line)
			}
			continue
		}

		// Emit a structured log for each event found during scan.
		if ev.Action == "blocked" || ev.Severity == "CRITICAL" || ev.Severity == "ERROR" {
			slog.WarnContext(ctx, "waf_event",
				"client_ip", ev.ClientIP,
				"path", ev.Path,
				"rule", ev.Rule,
				"severity", ev.Severity,
				"action", ev.Action,
				"timestamp", ev.Timestamp,
			)
		} else {
			slog.InfoContext(ctx, "waf_event",
				"client_ip", ev.ClientIP,
				"path", ev.Path,
				"rule", ev.Rule,
				"severity", ev.Severity,
				"action", ev.Action,
				"timestamp", ev.Timestamp,
			)
		}

		events = append(events, ev)
	}

	if err := scanner.Err(); err != nil {
		return events, unparseable, fmt.Errorf("scan waf audit log: %w", err)
	}

	slog.InfoContext(ctx, "waf_audit_scan_complete",
		"events_found", len(events),
		"unparseable_lines", unparseable,
	)

	return events, unparseable, nil
}

// ReadWAFAuditLogFromContainer reads the WAF audit log from the nginx container
// and returns parsed events. workdir is the nself project directory.
func ReadWAFAuditLogFromContainer(ctx context.Context, workdir string) ([]WAFEvent, int, error) {
	cmd := docker.NewProjectCompose(workdir).CommandContext(ctx, workdir, "exec", "nginx",
		"sh", "-c", "cat /var/log/nginx/waf_audit.log 2>/dev/null || true",
	)
	cmd.Dir = workdir

	out, err := cmd.Output()
	if err != nil {
		return nil, 0, fmt.Errorf("read waf audit log from container: %w", err)
	}

	events, unparseable, scanErr := ScanWAFAuditLog(ctx, strings.NewReader(string(out)))
	if scanErr != nil {
		slog.ErrorContext(ctx, "waf_audit_scan_error",
			"error", scanErr,
			"workdir", workdir,
		)
		return events, unparseable, scanErr
	}

	return events, unparseable, nil
}
