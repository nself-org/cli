package entitlement

// Table tests for plugin.entitlement-token v1.1 publication (D-0851).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
)

func TestPublishCandidatesMultiKey(t *testing.T) {
	sign := policySigner(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "key-a")
	t.Setenv("NSELF_LICENSE_KEY_1", "key-b")
	t.Setenv("NSELF_LICENSE_KEY_2", "key-c")
	now := time.Now()
	hour, later := now.Add(time.Hour).Unix(), now.Add(2*time.Hour).Unix()
	past := now.Add(-license.PostExpiryGraceWindow - time.Hour).Unix()
	chatA := sign("key-a", []string{"chat"}, hour)
	clawB := sign("key-b", []string{"claw"}, later)
	bothA := sign("key-a", []string{"chat", "claw"}, hour)
	newerA := sign("key-a", []string{"chat"}, later+60)
	chatB := sign("key-b", []string{"chat"}, later)
	mailC := sign("key-c", []string{"mail"}, hour)
	expiredA := sign("key-a", []string{"claw"}, past)
	parts := strings.Split(chatA, ".")
	forgedB := strings.Join(append(strings.Split(clawB, ".")[:2], parts[2]), ".") // key-b claims, key-a signature
	for _, tc := range []struct {
		name       string
		retained   string // existing file content (lines)
		candidates []string
		names      []string
		rejected   map[string]bool
		want       []string
		uncovered  string // substring E125 must name; empty = success
	}{
		{"one-key-as-today", "", []string{chatA}, []string{"chat"}, nil, []string{chatA}, ""},
		{"two-keys-two-bundles", "", []string{chatA, clawB}, []string{"chat", "claw"}, nil, []string{clawB, chatA}, ""},
		{"retained-other-key-line", clawB, []string{chatA}, []string{"chat", "claw"}, nil, []string{clawB, chatA}, ""},
		{"bad-signature-line-1-valid-line-2", forgedB + "\n" + clawB, []string{chatA}, []string{"chat", "claw"}, nil, []string{clawB, chatA}, ""},
		{"expired-line-plus-valid", expiredA + "\n" + clawB, nil, []string{"claw"}, nil, []string{clawB}, ""},
		{"expired-line-grants-nothing", chatB, []string{expiredA}, []string{"chat", "claw"}, nil, []string{chatB}, "claw"},
		{"slug-no-line-covers", "", []string{chatA, clawB}, []string{"chat", "claw", "paid", "mail"}, nil, []string{clawB, chatA}, "paid, mail"},
		{"line-1-covers-all-is-v1-file", "", []string{chatB, bothA}, []string{"chat", "claw"}, nil, []string{bothA}, ""},
		{"most-covering-first", "", []string{mailC, bothA}, []string{"chat", "claw", "mail"}, nil, []string{bothA, mailC}, ""},
		{"one-line-per-key-newest", "", []string{chatA, newerA, clawB}, []string{"chat", "claw"}, nil, []string{newerA, clawB}, ""},
		{"rejected-key-grants-nothing", "", []string{chatA, clawB}, []string{"chat", "claw"}, map[string]bool{license.HashKey("key-b"): true}, []string{chatA}, "claw"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.retained != "" {
				if err := os.MkdirAll(filepath.Join(dir, ".nself", "state"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(Path(dir), []byte(tc.retained), 0600); err != nil {
					t.Fatal(err)
				}
			}
			candidates := append(append([]string{}, tc.candidates...), retainedTokens(Path(dir))...)
			err := publishCandidates(dir, tc.names, tc.rejected, candidates)
			data, readErr := os.ReadFile(Path(dir))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if got := string(data); got != strings.Join(tc.want, "\n") {
				t.Fatalf("file lines %d, want %d (or order differs)", len(strings.Split(got, "\n")), len(tc.want))
			}
			if first, _, _ := strings.Cut(string(data), "\n"); first != tc.want[0] {
				t.Fatal("line 1 is not the best single token")
			}
			if tc.uncovered == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var coded *errs.CLIError
			if !errors.As(err, &coded) || coded.Code != "E125" || !strings.Contains(err.Error(), "installed plugins: "+tc.uncovered) {
				t.Fatalf("want E125 naming %q, got %v", tc.uncovered, err)
			}
		})
	}
}

func TestRetainedTokensPerLine(t *testing.T) {
	sign := policySigner(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "key-a")
	t.Setenv("NSELF_LICENSE_KEY_1", "key-b")
	hour := time.Now().Add(time.Hour).Unix()
	a := sign("key-a", []string{"chat"}, hour)
	b := sign("key-b", []string{"claw"}, hour)
	stranger := sign("key-z", []string{"*"}, hour)
	path := filepath.Join(t.TempDir(), "entitlement")
	for _, tc := range []struct {
		name, content string
		want          []string
	}{
		{"v1-single-line", a, []string{a}},
		{"two-lines", a + "\n" + b, []string{a, b}},
		{"garbage-line-1", "not.a.jwt\n" + b, []string{b}},
		{"unconfigured-subject", stranger + "\n" + a, []string{a}},
		{"trailing-newline", a + "\n", []string{a}},
		{"empty", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			if got := retainedTokens(path); strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("retained %d lines, want %d", len(got), len(tc.want))
			}
		})
	}
}

func TestWithRetainedNeverFallsBackForRenewedKey(t *testing.T) {
	sign := policySigner(t)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "key-a")
	t.Setenv("NSELF_LICENSE_KEY_1", "key-b")
	hour := time.Now().Add(time.Hour).Unix()
	oldA := sign("key-a", []string{"chat", "claw"}, hour)
	renewedA := sign("key-a", []string{"chat"}, hour+60)
	b := sign("key-b", []string{"mail"}, hour)
	path := filepath.Join(t.TempDir(), "entitlement")
	if err := os.WriteFile(path, []byte(oldA+"\n"+b), 0600); err != nil {
		t.Fatal(err)
	}
	got := withRetained([]string{renewedA}, path)
	if strings.Join(got, "\n") != renewedA+"\n"+b {
		t.Fatalf("got %d candidates; the renewed key's older line must be dropped", len(got))
	}
	lines, err := selectTokens(got, []string{"chat", "claw", "mail"}, nil)
	if err == nil || !strings.Contains(err.Error(), "installed plugins: claw") || len(lines) != 2 {
		t.Fatalf("a narrowed bundle was widened by its older line: %v", err)
	}
}
