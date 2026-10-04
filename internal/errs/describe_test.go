package errs

import (
	"encoding/json"
	"fmt"
	"testing"
)

// goldenJSON marshals a Detail the way the envelope will.
func goldenJSON(t *testing.T, err error) string {
	t.Helper()
	b, e := json.Marshal(Describe(err))
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}

// TestDescribe_Golden pins the JSON shape and field order for a CLIError, a
// wrapped sentinel, a plain error and ExitWith(2, plain).
func TestDescribe_Golden(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{
			"CLIError",
			New("E403", "refusing to reset prod").WithWhy("prod needs --force").WithFix("add --force"),
			`{"code":"E403","message":"refusing to reset prod","cause":"prod needs --force","remediation":"add --force","docs_url":"https://nself.org/docs/reference/error-codes#e403","exit_code":4,"class":"destructive_blocked"}`,
		},
		{
			"wrapped sentinel",
			fmt.Errorf("start: %w", ErrDockerNotRunning),
			`{"code":"E002","message":"start: docker daemon is not running","cause":"The Docker daemon is not responding to commands.","remediation":"Start Docker Desktop or run: sudo systemctl start docker","docs_url":"https://nself.org/docs/reference/error-codes#e002","exit_code":2,"class":"infra"}`,
		},
		{
			"plain error",
			fmt.Errorf("boom"),
			`{"code":"E400","message":"boom","docs_url":"https://nself.org/docs/reference/error-codes#e400","exit_code":1,"class":"user"}`,
		},
		{
			"ExitWith plain",
			ExitWith(2, fmt.Errorf("boom")),
			`{"code":"E400","message":"boom","docs_url":"https://nself.org/docs/reference/error-codes#e400","exit_code":2,"class":"infra"}`,
		},
	} {
		if got := goldenJSON(t, tc.err); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.name, got, tc.want)
		}
	}
}

// TestDescribe_SentinelUsesRegistryDefaults checks the acceptance example for
// E002 field by field against the registry entry.
func TestDescribe_SentinelUsesRegistryDefaults(t *testing.T) {
	d := Describe(fmt.Errorf("start: %w", ErrDockerNotRunning))
	e := Registry["E002"]
	if d.Code != "E002" || d.Class != "infra" || d.ExitCode != 2 {
		t.Errorf("got %+v", d)
	}
	if d.Cause != e.DefaultWhy || d.Remediation != e.DefaultFix {
		t.Errorf("cause/remediation are not the E002 defaults: %+v", d)
	}
	if d.DocsURL != "https://nself.org/docs/reference/error-codes#e002" {
		t.Errorf("docs_url = %q", d.DocsURL)
	}
}

// TestDescribe_EveryMappedSentinel runs Describe over the whole table: the
// code, exit and class must agree with the registry.
func TestDescribe_EveryMappedSentinel(t *testing.T) {
	for _, s := range sentinelTable {
		d := Describe(fmt.Errorf("ctx: %w", s.Err))
		e := Registry[s.Code]
		if d.Code != s.Code || d.ExitCode != e.Exit || d.Class != ClassFor(e.Exit) || d.Cause != e.DefaultWhy || d.Remediation != e.DefaultFix {
			t.Errorf("%v: Describe = %+v, registry %+v", s.Err, d, e)
		}
	}
}

// TestDescribe_CLIErrorFieldsWin proves a CLIError's own fields beat defaults,
// and that a CLIError's code beats a wrapped sentinel's.
func TestDescribe_CLIErrorFieldsWin(t *testing.T) {
	d := Describe(Wrap("E401", "bad flag --x", ErrDockerNotRunning).WithFix("use --y"))
	if d.Code != "E401" || d.Message != "bad flag --x" || d.Remediation != "use --y" || d.ExitCode != 1 {
		t.Errorf("got %+v", d)
	}
}

// TestDescribe_Nil returns nil for a nil error.
func TestDescribe_Nil(t *testing.T) {
	if Describe(nil) != nil {
		t.Error("Describe(nil) must be nil")
	}
}

// TestClassFor covers the class names.
func TestClassFor(t *testing.T) {
	for code, want := range map[int]string{0: "other", 1: "user", 2: "infra", 3: "auth", 4: "destructive_blocked", 10: "other", 127: "other"} {
		if got := ClassFor(code); got != want {
			t.Errorf("ClassFor(%d) = %q, want %q", code, got, want)
		}
	}
}
