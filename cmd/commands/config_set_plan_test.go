package commands

// config_set_plan_test.go — `nself config set` reconciles (P7-LIVE-05).
//
// Purpose: prove, on the hermetic fixture projects of build_plan_test.go and
// the real binary:
//   - v1.5 dev: one `config set BASE_DOMAIN` regenerates compose and nginx with
//     no separate build, and prints the one-line cascade summary;
//   - --preview writes nothing (tree hash incl. .env) in both modes and prints
//     the plan, or with --json the envelope;
//   - v1.5 prod-class non-interactive without --yes: exit 4, E403, plan on
//     stderr, nothing written; --yes applies;
//   - v1.4: only the env line is written, stdout is origin/main's single
//     line, the summary and the notice go to stderr;
//   - a failing apply after the env write restores the env file byte for
//     byte (a real write failure and an injected one).
// Constraints: no daemon (docker stub), no network; skipped under -short.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

// readFileT returns the file's bytes or fails the test.
func readFileT(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// builtL03 is a fixture project after one successful build in the given mode.
func builtL03(t *testing.T, fixture string, v15 bool, args ...string) *l03Project {
	t.Helper()
	p := newL03Project(t, fixture)
	r := p.run(t, v15, append([]string{"build"}, args...)...)
	if r.code != 0 {
		t.Fatalf("baseline build exited %d\n%s\n%s", r.code, r.stdout, r.stderr)
	}
	return p
}

// TestConfigSetV15DevAppliesCascade: a dev BASE_DOMAIN change regenerates
// compose and nginx in the same command; nothing is left for a build.
func TestConfigSetV15DevAppliesCascade(t *testing.T) {
	p := builtL03(t, "dev-minimal", true)
	compose := filepath.Join(p.project, "docker-compose.yml")
	before := readFileT(t, compose)
	r := p.run(t, true, "config", "set", "BASE_DOMAIN", "other.test")
	if r.code != 0 {
		t.Fatalf("config set exited %d\n%s\n%s", r.code, r.stdout, r.stderr)
	}
	after := readFileT(t, compose)
	if after == before || !strings.Contains(after, "other.test") {
		t.Fatal("docker-compose.yml was not regenerated for the new BASE_DOMAIN")
	}
	confs, _ := filepath.Glob(filepath.Join(p.project, "nginx", "sites", "*.conf"))
	found := false
	for _, c := range confs {
		found = found || strings.Contains(readFileT(t, c), "other.test")
	}
	if !found {
		t.Fatalf("no nginx site conf names the new domain: %v", confs)
	}
	if !strings.Contains(readFileT(t, filepath.Join(p.project, ".env")), "BASE_DOMAIN=other.test\n") {
		t.Fatal("the env line was not written")
	}
	var summary string
	for _, l := range strings.Split(r.stdout, "\n") {
		if strings.HasPrefix(l, "config: BASE_DOMAIN → ") {
			summary = l
		}
	}
	if summary == "" || !strings.Contains(summary, "compose") || !strings.Contains(summary, "nginx") {
		t.Fatalf("summary line missing or does not name compose and nginx:\n%s", r.stdout)
	}
	if data := p.planData(t, true); data["empty"] != true {
		t.Fatalf("a build after config set still has work: %v", data["artifacts"])
	}
}

// TestConfigSetPreviewWritesNothing: --preview leaves the whole root (project
// incl. .env, HOME, plugin dir) unchanged in both modes, on a dev and a prod
// fixture, and prints the plan; --json prints the envelope (applied false).
func TestConfigSetPreviewWritesNothing(t *testing.T) {
	for _, fixture := range []string{"dev-minimal", "prod-plugins"} {
		for _, v15 := range []bool{false, true} {
			p := newL03Project(t, fixture)
			before := p.whole(t)
			r := p.run(t, v15, "config", "set", "BASE_DOMAIN", "other.test", "--preview")
			if r.code != 0 || !strings.Contains(r.stdout, "Preview: BASE_DOMAIN") || !strings.Contains(r.stdout, "Plan ") {
				t.Fatalf("%s v15=%v: preview exited %d\n%s\n%s", fixture, v15, r.code, r.stdout, r.stderr)
			}
			r = p.run(t, v15, "config", "set", "BASE_DOMAIN", "other.test", "--preview", "--json")
			if r.code != 0 {
				t.Fatalf("%s v15=%v: preview --json exited %d\n%s", fixture, v15, r.code, r.stderr)
			}
			data := validateEnvelope(t, r.stdout, "config set")
			plan, _ := data["plan"].(map[string]any)
			trig, _ := plan["trigger"].(map[string]any)
			if data["applied"] != false || data["key"] != "BASE_DOMAIN" || data["file"] != ".env" || trig["subject"] != "BASE_DOMAIN" || plan["command"] != "config-set" {
				t.Fatalf("%s v15=%v: envelope data %v", fixture, v15, data)
			}
			if after := p.whole(t); after != before {
				t.Fatalf("%s v15=%v: --preview changed the root:\n%s", fixture, v15, l03LineDiff(before, after))
			}
		}
	}
}

// TestConfigSetProdRefusal: v1.5 prod-class without --yes and without a
// terminal refuses with E403 (exit 4), prints the plan on stderr and writes
// nothing, not even the env line; --yes applies.
func TestConfigSetProdRefusal(t *testing.T) {
	p := newL03Project(t, "prod-plugins")
	before := p.whole(t)
	r := p.run(t, true, "config", "set", "BASE_DOMAIN", "other.org")
	if r.code != 4 || !strings.Contains(r.stderr, "E403") || !strings.Contains(r.stderr, "Confirmation required: yes") {
		t.Fatalf("exit %d, want 4 with E403 and the plan on stderr\n%s\n%s", r.code, r.stdout, r.stderr)
	}
	r = p.run(t, true, "config", "set", "BASE_DOMAIN", "other.org", "--json")
	if code, exit := errorEnvelope(t, r.stdout); r.code != 4 || code != "E403" || exit != 4 {
		t.Fatalf("--json: exit %d error %s/%v, want 4 E403", r.code, code, exit)
	}
	if after := p.whole(t); after != before {
		t.Fatalf("the refused config set changed the root:\n%s", l03LineDiff(before, after))
	}
	r = p.run(t, true, "config", "set", "BASE_DOMAIN", "other.org", "--yes", "--json")
	if r.code != 0 {
		t.Fatalf("--yes exited %d\n%s\n%s", r.code, r.stdout, r.stderr)
	}
	if data := validateEnvelope(t, r.stdout, "config set"); data["applied"] != true {
		t.Fatalf("applied = %v", data["applied"])
	}
	if !strings.Contains(readFileT(t, filepath.Join(p.project, ".env")), "BASE_DOMAIN=other.org\n") {
		t.Fatal("--yes did not write the line")
	}
	if data := p.planData(t, true); data["empty"] != true {
		t.Fatalf("plan after --yes is not empty: %v", data["artifacts"])
	}
}

// TestConfigSetV14WritesLineOnly: v1.4 writes the env line exactly as before,
// regenerates nothing, prints origin/main's single stdout line and puts the
// plan summary and the notice on stderr.
func TestConfigSetV14WritesLineOnly(t *testing.T) {
	p := builtL03(t, "dev-minimal", false)
	envPath := filepath.Join(p.project, ".env")
	before := p.whole(t)
	r := p.run(t, false, "config", "set", "BASE_DOMAIN", "other.test")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s\n%s", r.code, r.stdout, r.stderr)
	}
	if want := "Updated BASE_DOMAIN in " + envPath; strings.Count(r.stdout, "\n") != 1 || !strings.HasSuffix(strings.TrimRight(r.stdout, "\n"), want) {
		t.Fatalf("stdout must be the single origin/main line ending %q, got:\n%q", want, r.stdout)
	}
	if !strings.Contains(r.stderr, "config: BASE_DOMAIN → ") || !strings.Contains(r.stderr, "not applied in v1.4 mode") {
		t.Fatalf("stderr lacks the summary or the notice:\n%s", r.stderr)
	}
	diff := l03LineDiff(before, p.whole(t))
	for _, l := range strings.Split(diff, "\n") {
		if l != "" && !strings.HasSuffix(strings.Fields(l)[3], "project/.env") {
			t.Fatalf("v1.4 changed more than .env:\n%s", diff)
		}
	}
	if !strings.Contains(readFileT(t, envPath), "BASE_DOMAIN=other.test\n") {
		t.Fatal("the env line was not written")
	}
}

// TestConfigSetSecretsFileApplies: rotating a live secret is
// `config set <KEY> <v> --env secrets`: .env.secrets gets the line (0600) and
// the regeneration is applied (a plan afterwards is empty).
func TestConfigSetSecretsFileApplies(t *testing.T) {
	p := builtL03(t, "dev-minimal", true)
	r := p.run(t, true, "config", "set", "HASURA_GRAPHQL_ADMIN_SECRET", "Zq8vX3kLm9Pw2Rt7Yb4Nc6Hd1Jf5Gs0Qe", "--env", "secrets")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s\n%s", r.code, r.stdout, r.stderr)
	}
	path := filepath.Join(p.project, ".env.secrets")
	if !strings.Contains(readFileT(t, path), "HASURA_GRAPHQL_ADMIN_SECRET=Zq8vX3kLm9Pw2Rt7Yb4Nc6Hd1Jf5Gs0Qe\n") {
		t.Fatal(".env.secrets lacks the rotated value")
	}
	if info, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf(".env.secrets mode: %v %v", info, err)
	}
	if data := p.planData(t, true); data["empty"] != true {
		t.Fatalf("plan after the rotation is not empty: %v", data["artifacts"])
	}
}

// TestConfigSetRestoresEnvWhenWriteFails: the build cannot write a generated
// file (a read-only docker-compose.yml) after the env line was written; the env file is restored byte for byte
// and the command fails.
func TestConfigSetRestoresEnvWhenWriteFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	p := builtL03(t, "dev-minimal", true)
	envPath := filepath.Join(p.project, ".env")
	l03Write(t, envPath, readFileT(t, envPath)+"# trailing comment\r\nKEEP=1", 0o600)
	was := readFileT(t, envPath)
	compose := filepath.Join(p.project, "docker-compose.yml")
	if err := os.Chmod(compose, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(compose, 0o600) })
	r := p.run(t, true, "config", "set", "BASE_DOMAIN", "other.test")
	if r.code == 0 {
		t.Fatalf("config set succeeded with a read-only docker-compose.yml\n%s", r.stdout)
	}
	if got := readFileT(t, envPath); got != was {
		t.Fatalf("env file not restored:\n--- want\n%q\n--- got\n%q", was, got)
	}
	if !strings.Contains(r.stderr, "restored") {
		t.Fatalf("stderr does not say the env file was restored:\n%s", r.stderr)
	}
}

// configSetTestCmd is a command carrying config set's flags, for in-process runs.
func configSetTestCmd(args ...string) *cobra.Command {
	c := &cobra.Command{Use: "set"}
	c.Flags().Bool("preview", false, "")
	c.Flags().Bool("yes", false, "")
	c.Flags().Bool("force", false, "")
	c.Flags().Bool("json", false, "")
	_ = c.Flags().Parse(args)
	c.SetOut(&strings.Builder{})
	c.SetErr(&strings.Builder{})
	return c
}

// TestConfigSetRestoresEnvOnInjectedFailure: whatever fails after the env
// write (here: an apply that writes, then errors), the previous bytes come
// back exactly, a file the write created is removed, and an apply that fails
// before the write leaves the file alone.
func TestConfigSetRestoresEnvOnInjectedFailure(t *testing.T) {
	compattest.Set(t, true)
	old := configSetApply
	t.Cleanup(func() { configSetApply = old })
	injected := errors.New("injected apply failure")
	writeThenFail := func(_ context.Context, _ reconcile.Request, opt reconcile.ApplyOptions) (*reconcile.Plan, error) {
		if err := opt.BeforeWrite(); err != nil {
			return nil, err
		}
		return nil, injected
	}
	failBefore := func(context.Context, reconcile.Request, reconcile.ApplyOptions) (*reconcile.Plan, error) {
		return nil, injected
	}
	for _, tc := range []struct {
		name   string
		seed   *string
		apply  func(context.Context, reconcile.Request, reconcile.ApplyOptions) (*reconcile.Plan, error)
		output string
	}{
		{"existing file", ptr("# head\r\nA=1\nKEY=old\n\n#tail"), writeThenFail, "restored"},
		{"new file", nil, writeThenFail, "restored"},
		{"refused before the write", ptr("KEY=old\n"), failBefore, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".env.secrets")
			if tc.seed != nil {
				if err := os.WriteFile(path, []byte(*tc.seed), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			configSetApply = func(ctx context.Context, req reconcile.Request, opt reconcile.ApplyOptions) (*reconcile.Plan, error) {
				if got := req.EnvOverlay[".env.secrets"]["KEY"]; got != "new" {
					t.Fatalf("overlay = %v", req.EnvOverlay)
				}
				return tc.apply(ctx, req, opt)
			}
			cmd := configSetTestCmd()
			r := configSetRun{cmd: cmd, projectDir: dir, envFile: path, key: "KEY", value: "new"}
			if err := r.apply(); !errors.Is(err, injected) {
				t.Fatalf("apply error = %v, want the injected one", err)
			}
			got, err := os.ReadFile(path)
			switch {
			case tc.seed == nil && !os.IsNotExist(err):
				t.Fatalf("a created env file must be removed: %q %v", got, err)
			case tc.seed != nil && string(got) != *tc.seed:
				t.Fatalf("env file not restored byte for byte:\n%q\n%q", *tc.seed, got)
			}
			if stderr := cmd.ErrOrStderr().(*strings.Builder).String(); !strings.Contains(stderr, tc.output) || (tc.output == "" && stderr != "") {
				t.Fatalf("stderr %q, want %q", stderr, tc.output)
			}
		})
	}
}

func ptr(s string) *string { return &s }

// TestConfigSetRelFile: the overlay names the file the write targets; a
// name with a path in it is passed through for the planner to refuse.
func TestConfigSetRelFile(t *testing.T) {
	dir := t.TempDir()
	for env, want := range map[string]string{"": ".env", "secrets": ".env.secrets", "x/.env.prod": ".env.x/.env.prod"} {
		r := configSetRun{projectDir: dir, envFile: envFileName(dir, env)}
		if got := r.relFile(); got != want {
			t.Errorf("--env %q: relFile %q, want %q", env, got, want)
		}
	}
}

// TestConfigSetApplyErrorKeepsCodes: a refusal is re-issued as E403 naming
// the key; any other error keeps its code and gains the key.
func TestConfigSetApplyErrorKeepsCodes(t *testing.T) {
	err := configSetApplyError("K", reconcile.Confirm(reconcile.Plan{RequiresConfirmation: true}, reconcile.ApplyOptions{}, true, nil))
	if !strings.Contains(err.Error(), "config set K") {
		t.Fatalf("refusal: %v", err)
	}
	inner := errors.New("boom")
	if err := configSetApplyError("K", inner); !errors.Is(err, inner) || !strings.Contains(err.Error(), "config set K") {
		t.Fatalf("other: %v", err)
	}
}

func TestConfigSetSummary(t *testing.T) {
	p := reconcile.Plan{Artifacts: []reconcile.Artifact{
		{Kind: reconcile.KindNginx, Path: "nginx/sites/a.conf"},
		{Kind: reconcile.KindCompose, Path: "docker-compose.yml"},
		{Kind: reconcile.KindNginx, Path: "nginx/sites/b.conf"},
	}}
	if got, want := configSetSummary("K", p), "config: K → 3 artifacts changed (compose, nginx)"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, want := configSetSummary("K", reconcile.Plan{}), "config: K → 0 artifacts changed"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestConfigSetWriteLineUnchangedBytes: setEnvFileLine writes what it always
// wrote (the shared rewrite), creates missing files 0600 and enforces 0600.
func TestConfigSetWriteLineUnchangedBytes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env")
		if updated, err := setEnvFileLine(path, "A", "x y"); err != nil || updated {
			t.Fatalf("create: %v %v", updated, err)
		}
		if got := readFileT(t, path); got != "A=\"x y\"\n" {
			t.Fatalf("created %q", got)
		}
		if err := os.WriteFile(path, []byte("# c\r\nA=1\nB=2"), 0o644); err != nil {
			t.Fatal(err)
		}
		if updated, err := setEnvFileLine(path, "A", "2"); err != nil || !updated {
			t.Fatalf("update: %v %v", updated, err)
		}
		if got := readFileT(t, path); got != "# c\nA=2\nB=2\n" {
			t.Fatalf("updated %q", got)
		}
		if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", info.Mode().Perm())
		}
		if err := os.WriteFile(path, []byte("B="+strings.Repeat("x", 70*1024)), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := setEnvFileLine(path, "A", "1"); err == nil || !strings.Contains(err.Error(), "scanning") {
			t.Fatalf("an overlong line must fail as before: %v", err)
		}
	})
}
