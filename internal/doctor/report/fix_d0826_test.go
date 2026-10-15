package report

// fix_d0826_test.go: regression tests for debt D-0826 (curl -u passwords with a
// space or comma, a log-name collision loop, a dropped container-state error).
// No docker is started: the state query runs against a stub script.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestScrubCurlUserQuotedPasswords(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		absent   []string
		present  []string
	}{
		{"double quoted with spaces", `curl -u "admin:my pass word" http://h/x`, []string{"pass", "word"}, []string{"http://h/x"}},
		{"single quoted with spaces", `curl --user 'admin:my pass word' http://h/x`, []string{"pass", "word"}, []string{"http://h/x"}},
		{"equals form quoted", `curl --user="admin:my pass word" http://h/x`, []string{"pass", "word"}, []string{"http://h/x"}},
		{"quoted with comma", `curl -u "admin:abc,defTAIL" http://h/x`, []string{"defTAIL", "abc"}, []string{"http://h/x"}},
		{"quoted with leading space", `curl -u "admin: lead TAIL" http://h/x`, []string{"lead", "TAIL"}, []string{"http://h/x"}},
		{"json list form with space", `["curl","-u","admin:my pass word","http://h"]`, []string{"pass", "word"}, []string{`"http://h"`}},
		{"unquoted still works", `curl -sS -u admin:plainpw1 http://h/x`, []string{"plainpw1"}, []string{"curl -sS -u admin:", "http://h/x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewScrubber(nil)
			out := s.Scrub(tc.in)
			for _, a := range tc.absent {
				if strings.Contains(out, a) {
					t.Errorf("secret part %q survived:\n%s", a, out)
				}
			}
			for _, p := range tc.present {
				if !strings.Contains(out, p) {
					t.Errorf("neighbour %q was over-redacted:\n%s", p, out)
				}
			}
		})
	}
}

// Two service names that both begin with a licence key keep the same scrubbed
// stem; the counter must stay visible so the names differ (no endless loop).
func TestCollectLogsCollidingScrubbedNames(t *testing.T) {
	logs := map[string]string{
		"nself_pro_abcdef123456-a": "one\n",
		"nself_pro_abcdef123456-b": "two\n",
		"nself_pro_abcdef123456-c": "three\n",
	}
	src := Sources{
		Logs: func(context.Context, int) (map[string]string, error) { return logs, nil },
		LogNotes: func() []string {
			return []string{"container state for service nself_pro_abcdef123456-a could not be read"}
		},
	}
	var notes []string
	got := map[string]string{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		notes = collectLogs(context.Background(), Options{}, src, func(n, text string) { got[n] = text }, NewScrubber(nil))
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("collectLogs did not return: unique-name loop is stuck")
	}
	if len(got) != 3 {
		t.Errorf("want 3 distinct log entries, got %d: %v", len(got), got)
	}
	if len(notes) != 1 || strings.Contains(notes[0], "abcdef123456") {
		t.Errorf("manifest notes leak the key: %v", notes)
	}
	for n := range got {
		if strings.Contains(n, "abcdef123456") {
			t.Errorf("entry name %q leaks the key", n)
		}
	}
}

// A failed state query for a service is a manifest note naming the service,
// never the error text (LogNotes are appended unscrubbed).
func TestDockerLogsNotesUnreadableState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$*\" in\n*\"config --services\"*) echo web;;\n*) echo 'boom nself_pro_abcdef123456' >&2; exit 1;;\nesac\n"
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	var notes []string
	out, err := dockerLogs(context.Background(), DockerSetup{ProjectDir: dir, DockerPath: bin}, 10, func(s string) { notes = append(notes, s) })
	if err != nil || len(out) != 0 {
		t.Fatalf("out=%v err=%v", out, err)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "web") {
		t.Fatalf("notes = %v, want one note naming the service web", notes)
	}
	if strings.Contains(notes[0], "boom") || strings.Contains(notes[0], "abcdef123456") {
		t.Errorf("note carries error text: %q", notes[0])
	}
}
