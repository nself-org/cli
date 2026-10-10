package commands

// Tests for the update-hint wiring (P7-ADOPT-08): the guards table and the
// invocation decorator path, against an httptest server. No real network.

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/updatecheck"
	"github.com/spf13/cobra"
)

type hintEnv struct {
	root *cobra.Command
	errw *bytes.Buffer
	outw *bytes.Buffer
	env  map[string]string
	tty  bool
	now  time.Time
	path string
	hits int
	body string
	done <-chan struct{}
}

func newHintEnv(t *testing.T) *hintEnv {
	t.Helper()
	output.ResetState()
	t.Cleanup(output.ResetState)
	h := &hintEnv{env: map[string]string{}, tty: true, body: `{"latestCliVersion":"99.0.0"}`,
		now: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC), path: filepath.Join(t.TempDir(), "update-check.json"),
		errw: &bytes.Buffer{}, outw: &bytes.Buffer{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		h.hits++
		_, _ = w.Write([]byte(h.body))
	}))
	t.Cleanup(srv.Close)

	root := &cobra.Command{Use: "nself", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("json", false, "json")
	root.SetOut(h.outw)
	root.SetErr(h.errw)
	ok := func(c *cobra.Command, _ []string) error { c.Println("stdout-body"); return nil }
	bad := func(*cobra.Command, []string) error { return errors.New("boom") }
	root.AddCommand(
		&cobra.Command{Use: "plain", RunE: ok},
		&cobra.Command{Use: "failing", RunE: bad},
		&cobra.Command{Use: "update", RunE: ok},
		&cobra.Command{Use: "envl", RunE: ok},
	)
	entries := map[string]*cmdregistry.Command{
		"nself plain": {JSON: canon.JSONEnvelope}, "nself failing": {JSON: canon.JSONEnvelope},
		"nself update": {JSON: canon.JSONEnvelope}, "nself envl": {JSON: canon.JSONEnvelope},
	}
	oldEntry, oldGuards, oldStarted := jsonEntryFor, updateCheckGuards, updateCheckStarted
	jsonEntryFor = func(c *cobra.Command) (*cmdregistry.Command, error) {
		if e, found := entries[c.CommandPath()]; found {
			return e, nil
		}
		return nil, errors.New("no entry for " + c.CommandPath())
	}
	updateCheckGuards = func(*cobra.Command) updatecheck.Guards {
		_, jsonOn, known := output.Invocation()
		return updatecheck.Guards{
			JSON: jsonOn || !known, TTY: func() bool { return h.tty },
			Env: func(k string) string { return h.env[k] }, Now: func() time.Time { return h.now },
			Path: h.path, Current: "1.4.12", URL: srv.URL, Client: updatecheck.NewClient(),
		}
	}
	updateCheckStarted = func(d <-chan struct{}) { h.done = d }
	t.Cleanup(func() { jsonEntryFor, updateCheckGuards, updateCheckStarted = oldEntry, oldGuards, oldStarted })
	installInvocationDecorator(root)
	h.root = root
	return h
}

// run executes one command and waits for a refresh it started, if any.
func (h *hintEnv) run(args ...string) error {
	h.errw.Reset()
	h.outw.Reset()
	h.done = nil
	h.root.SetArgs(args)
	err := h.root.Execute()
	if h.done != nil {
		<-h.done
	}
	return err
}

func (h *hintEnv) seed(t *testing.T, latest string, checkedAgo, hintedAgo time.Duration) {
	t.Helper()
	c := updatecheck.Cache{CheckedAt: h.now.Add(-checkedAgo), Latest: latest}
	if hintedAgo > 0 {
		c.HintedAt = h.now.Add(-hintedAgo)
	}
	if err := updatecheck.Save(h.path, c); err != nil {
		t.Fatal(err)
	}
}

const hintWord = "nself update"

func TestUpdateCheckHintGuards(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		tty  bool
		args []string
		cmd  string
		want bool
	}{
		{"opted in on a tty", map[string]string{"NSELF_UPDATE_CHECK": "1"}, true, nil, "plain", true},
		{"not opted in", nil, true, nil, "plain", false},
		{"json flag", map[string]string{"NSELF_UPDATE_CHECK": "1"}, true, []string{"--json"}, "plain", false},
		{"non-tty stderr", map[string]string{"NSELF_UPDATE_CHECK": "1"}, false, nil, "plain", false},
		{"CI", map[string]string{"NSELF_UPDATE_CHECK": "1", "CI": "true"}, true, nil, "plain", false},
		{"GITHUB_ACTIONS", map[string]string{"NSELF_UPDATE_CHECK": "1", "GITHUB_ACTIONS": "true"}, true, nil, "plain", false},
		{"GITLAB_CI", map[string]string{"NSELF_UPDATE_CHECK": "1", "GITLAB_CI": "true"}, true, nil, "plain", false},
		{"BUILDKITE", map[string]string{"NSELF_UPDATE_CHECK": "1", "BUILDKITE": "true"}, true, nil, "plain", false},
		{"JENKINS_URL", map[string]string{"NSELF_UPDATE_CHECK": "1", "JENKINS_URL": "http://j"}, true, nil, "plain", false},
		{"failed command", map[string]string{"NSELF_UPDATE_CHECK": "1"}, true, nil, "failing", false},
		{"update command itself", map[string]string{"NSELF_UPDATE_CHECK": "1"}, true, nil, "update", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHintEnv(t)
			h.env, h.tty = c.env, c.tty
			h.seed(t, "99.0.0", time.Hour, 0)
			_ = h.run(append([]string{c.cmd}, c.args...)...)
			got := strings.Contains(h.errw.String(), hintWord)
			if got != c.want {
				t.Fatalf("hint printed = %v, want %v; stderr %q", got, c.want, h.errw.String())
			}
			if strings.Contains(h.outw.String(), hintWord) {
				t.Fatalf("hint reached stdout: %q", h.outw.String())
			}
			if h.hits != 0 {
				t.Fatalf("fresh cache must not trigger a request, hits=%d", h.hits)
			}
		})
	}
	t.Run("cached latest empty invalid or not newer", func(t *testing.T) {
		for _, latest := range []string{"", "nonsense", "1.4.12", "1.0.0"} {
			h := newHintEnv(t)
			h.env = map[string]string{"NSELF_UPDATE_CHECK": "1"}
			h.seed(t, latest, time.Hour, 0)
			_ = h.run("plain")
			if strings.Contains(h.errw.String(), hintWord) {
				t.Fatalf("latest %q printed a hint", latest)
			}
		}
	})
}

func TestUpdateCheckHintInvocation(t *testing.T) {
	h := newHintEnv(t)
	h.env = map[string]string{"NSELF_UPDATE_CHECK": "1"}

	// Behind and no cache: the first command refreshes in the background.
	if err := h.run("plain"); err != nil {
		t.Fatal(err)
	}
	if h.hits != 1 {
		t.Fatalf("hits = %d, want 1", h.hits)
	}
	if strings.Contains(h.errw.String(), hintWord) {
		t.Fatalf("first command read an empty cache yet printed a hint: %q", h.errw.String())
	}
	// The next command prints one hint naming current and latest.
	h.now = h.now.Add(time.Minute)
	if err := h.run("plain"); err != nil {
		t.Fatal(err)
	}
	out := h.errw.String()
	if strings.Count(out, "\n") != 1 || !strings.Contains(out, "1.4.12") || !strings.Contains(out, "99.0.0") || !strings.Contains(out, hintWord) {
		t.Fatalf("hint line = %q", out)
	}
	// A second command within 24 h prints nothing and sends nothing.
	h.now = h.now.Add(time.Hour)
	_ = h.run("plain")
	if h.errw.Len() != 0 || h.hits != 1 {
		t.Fatalf("within 24h: stderr %q hits %d", h.errw.String(), h.hits)
	}
	// After 24 h it prints again, and the stale cache refreshes.
	h.now = h.now.Add(25 * time.Hour)
	_ = h.run("plain")
	if !strings.Contains(h.errw.String(), hintWord) || h.hits != 2 {
		t.Fatalf("after 24h: stderr %q hits %d", h.errw.String(), h.hits)
	}
	if strings.Contains(h.outw.String(), hintWord) {
		t.Fatal("hint on stdout")
	}
}

func TestUpdateCheckHintNoRequestWithoutOptIn(t *testing.T) {
	h := newHintEnv(t)
	_ = h.run("plain")
	_ = h.run("plain", "--json")
	if h.hits != 0 || h.errw.Len() != 0 {
		t.Fatalf("hits %d stderr %q", h.hits, h.errw.String())
	}
	if _, err := os.Stat(h.path); err == nil {
		t.Fatal("cache file written without opt-in")
	}
}

func TestUpdateCheckHintHangingServerAddsNoDelayOrOutput(t *testing.T) {
	h := newHintEnv(t)
	h.env = map[string]string{"NSELF_UPDATE_CHECK": "1"}
	release := make(chan struct{})
	hang := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); hang.Close() })
	prev := updateCheckGuards
	updateCheckGuards = func(c *cobra.Command) updatecheck.Guards {
		g := prev(c)
		g.URL = hang.URL
		return g
	}
	h.root.SetArgs([]string{"plain"})
	start := time.Now()
	err := h.root.Execute()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if h.done == nil {
		t.Fatal("refresh was not started")
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("command took %v with a hanging server, want < 300ms", elapsed)
	}
	if strings.Contains(h.errw.String(), hintWord) || strings.Contains(h.errw.String(), "Error") {
		t.Fatalf("stderr = %q", h.errw.String())
	}
	<-h.done // let the abandoned goroutine finish before the temp dir goes
	if c := updatecheck.Load(h.path); c.LastError == "" {
		t.Fatalf("hang not recorded: %+v", c)
	}
}

func TestUpdateCheckHintDefaultGuardsTreatUnrecordedAsJSON(t *testing.T) {
	output.ResetState()
	t.Cleanup(output.ResetState)
	if g := defaultUpdateCheckGuards(nil); !g.JSON {
		t.Fatal("an invocation that was never recorded must count as JSON mode (silent)")
	}
	output.SetInvocation("plain", false)
	g := defaultUpdateCheckGuards(nil)
	if g.JSON || g.Current == "" || g.Path == "" || g.TTY == nil || g.Env == nil || g.Now == nil {
		t.Fatalf("default guards incomplete: %+v", g)
	}
	output.SetInvocation("plain", true)
	if g := defaultUpdateCheckGuards(nil); !g.JSON {
		t.Fatal("recorded JSON mode not honoured")
	}
}
