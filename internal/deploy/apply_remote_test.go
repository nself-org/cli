package deploy

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

var psService = regexp.MustCompile(`ps -a --format json -- '([^']+)'`)

// stackHandler answers the docker and nself commands of one fake stack.
// crash names a service whose container exits non-zero after its `up`.
func stackHandler(rp string, crash string) func(string) (string, bool) {
	upDone := map[string]bool{}
	upSvc := regexp.MustCompile(`--no-deps -- '([^']+)'`)
	return func(s string) (string, bool) {
		if m := upSvc.FindStringSubmatch(s); m != nil {
			upDone[m[1]] = true
		}
		switch {
		case strings.Contains(s, `printf 'OK`): // the rollback's check that the recorded images exist
			return "OK " + fakeImageID("a") + "\nOK " + fakeImageID("b") + "\n", true
		case strings.Contains(s, "docker image inspect"):
			return fakeImageID("a") + " postgres:16\n" + fakeImageID("b") + " web:1\n", true
		case strings.Contains(s, "docker tag"):
			return "", true
		case strings.Contains(s, "config --format json"):
			return modelJSON(rp), true
		case strings.Contains(s, "config --services"):
			return "db\nweb\nnginx\n", true
		case strings.Contains(s, "up -d --dry-run"):
			return " Container proj-web-1 Recreate\n Container proj-db-1 Running\n", true
		case strings.Contains(s, "docker run --rm"):
			return "nginx: the configuration file syntax is ok\n", true
		case strings.Contains(s, "up -d --no-deps"), strings.Contains(s, " pull"), strings.Contains(s, "nginx -s reload"), strings.Contains(s, "nself db hasura metadata apply"), strings.Contains(s, "nself backup"):
			return "", true
		case strings.Contains(s, "docker inspect -f"):
			return strings.Repeat("2026-10-10T20:00:00Z\n", strings.Count(s, "'id-")), true
		case psService.MatchString(s):
			svc := psService.FindStringSubmatch(s)[1]
			state, code, health := "running", 0, ""
			if svc == "db" {
				health = "healthy"
			}
			if svc == crash && upDone[svc] {
				state, code = "exited", 1
			}
			return fmt.Sprintf(`[{"ID":"id-%s","Name":"proj-%s-1","State":"%s","ExitCode":%d,"Health":"%s"}]`, svc, svc, state, code, health), true
		}
		return "", false
	}
}

// fakeImageID is a synthetic image id: 64 copies of one hex digit.
func fakeImageID(digit string) string { return "sha256:" + strings.Repeat(digit, 64) }

func classify(s string) string {
	switch {
	case strings.Contains(s, `printf 'OK`):
		return "imgcheck"
	case strings.Contains(s, "docker image inspect"):
		return "imgsave"
	case strings.Contains(s, "docker tag"):
		return "imgrestore"
	case strings.Contains(s, ".images") && strings.Contains(s, `mv -f "$t"`):
		return "imgrecord"
	case strings.HasPrefix(s, "rsync "):
		return "rsync"
	case strings.Contains(s, "deploy.lock"):
		return "lock"
	case strings.Contains(s, "plugin.json") || strings.Contains(s, `"$HOME"`) || strings.Contains(s, "NSELF_PLUGIN_DIR") || strings.Contains(s, "*.pem"):
		return "snapshot"
	case strings.Contains(s, "tar -czf"):
		return "tar"
	case strings.Contains(s, "tar -xzf"):
		return "restore"
	case strings.Contains(s, "tar -tzf") && !strings.Contains(s, "-czf"):
		return "tarlist"
	case strings.Contains(s, "stat -c"):
		return "perms"
	case strings.Contains(s, "nginx -s reload"):
		return "reload"
	case strings.Contains(s, "inside()"):
		return "layout"
	case strings.Contains(s, "UNMARKED"):
		return "foreign"
	case strings.Contains(s, "deploy-shipped.txt"):
		return "shipped"
	case strings.Contains(s, "head -n 5"), strings.Contains(s, "find \"$d\" -type f"):
		return "prune"
	case strings.Contains(s, "config --format json"), strings.Contains(s, "config --services"):
		return "model"
	case strings.Contains(s, "docker run --rm"):
		return "nginx"
	case strings.Contains(s, "--dry-run"):
		return "preview"
	case strings.Contains(s, "nself backup"):
		return "dbbackup"
	case strings.Contains(s, " pull"):
		return "pull"
	case strings.Contains(s, "up -d --no-deps"):
		m := regexp.MustCompile(`--no-deps -- '([^']+)'`).FindStringSubmatch(s)
		return "up:" + m[1]
	case strings.Contains(s, "ps -a"), strings.Contains(s, "docker inspect"):
		return "state"
	case strings.Contains(s, "hasura metadata apply"):
		return "hasura"
	}
	return "other:" + s
}

// steps is the log as step names, consecutive repeats collapsed.
func steps(f *fakeRemote) []string {
	var out []string
	for _, c := range f.commands() {
		if s := classify(c); len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}

func indexOf(ss []string, s string) int {
	for i, x := range ss {
		if x == s {
			return i
		}
	}
	return -1
}

// applyFixture stages a set against a fake host that already runs a deploy.
func applyFixture(t *testing.T, crash string) (*fakeRemote, ApplyOptions, string) {
	t.Helper()
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	seedDeployedHost(t, rp)
	m, compose, envs, snap := buildScratchFor(t, rp)
	set, err := Stage(stageIn(m, compose, envs, snap, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	f.handler = stackHandler(rp, crash)
	o := ApplyOptions{
		Remote: f, RemotePath: rp, Set: set, Previous: []string{"nginx/sites/old.conf"}, ProdClass: true, Yes: true, StrictHasura: true,
		Clock: &fakeClock{now: time.Unix(0, 0)},
		Now:   func() time.Time { return time.Date(2026, 10, 10, 20, 0, 0, 0, time.UTC) },
		Snapshot: func(ctx context.Context, id string) (string, error) {
			_, err := f.Run(ctx, "cd "+rp+" && nself backup create --tag "+id)
			return "backup-1", err
		},
	}
	return f, o, rp
}

var orderedMilestones = []string{"snapshot", "lock", "tar", "dbbackup", "shipped", "rsync", "perms", "model", "nginx", "imgsave", "imgrecord", "pull", "preview", "up:db", "up:web", "up:nginx", "reload", "hasura"}

func TestRemoteApplyOrder(t *testing.T) {
	f, o, rp := applyFixture(t, "")
	res, err := ApplyRemote(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	got := steps(f)
	prev := -1
	for _, m := range orderedMilestones {
		i := indexOf(got, m)
		if i < 0 || i <= prev {
			t.Fatalf("step %q at %d (previous at %d) in %v", m, i, prev, got)
		}
		prev = i
	}
	if res.BackupID != "backup-1" || res.BackupHost != "fake-host" || !strings.HasPrefix(res.PreDeployTar, rp+"/.nself/backups/pre-deploy-20261010T200000Z-") {
		t.Errorf("result %+v", res)
	}
	if strings.Join(res.Recreated, ",") != "web" {
		t.Errorf("recreated %v", res.Recreated)
	}
	// Services come up in `config --services` order, and each is gated after its up.
	for _, svc := range []string{"db", "web", "nginx"} {
		up := indexOf(got, "up:"+svc)
		if up < 0 || got[up+1] != "state" {
			t.Errorf("service %s was not gated right after its up: %v", svc, got)
		}
	}
	if indexOf(got, "restore") >= 0 || indexOf(got, "imgrestore") >= 0 {
		t.Errorf("a clean apply restored files or image tags: %v", got)
	}
	for _, c := range f.commands() {
		if strings.Contains(c, "--delete") {
			t.Errorf("recorded %q", c)
		}
	}

	// A failure at each step leaves every later step unrun.
	type failure struct {
		name, sub string
		errCheck  func(error) bool
		last      string // the failing milestone: nothing after it may appear
		restored  bool   // files put back (nothing touched yet)
	}
	code := func(c string) func(error) bool {
		return func(err error) bool { var ce *errs.CLIError; return errors.As(err, &ce) && ce.Code == c }
	}
	any := func(error) bool { return true }
	cases := []failure{
		{"snapshot", "plugin.json", any, "snapshot", false},
		{"lock", "if mkdir", any, "lock", false},
		{"tar", "tar -czf", any, "tar", false},
		{"db backup", "nself backup create", code("E206"), "dbbackup", false},
		{"rsync", "<rsync>", any, "rsync", true},
		{"perms", "stat -c", any, "perms", true},
		{"model", "config --format json", any, "model", true},
		{"nginx -t", "docker run --rm", any, "nginx", true},
		{"image ids", "docker image inspect", any, "imgsave", true},
		{"image record", `mv -f "$t"`, any, "imgrecord", true},
		{"pull", " pull", any, "pull", true},
		{"preview", "up -d --dry-run", any, "preview", true},
		{"first up", "up -d --no-deps -- 'db'", any, "up:db", false},
		{"second up", "up -d --no-deps -- 'web'", any, "up:web", false},
		{"reload", "nginx -s reload", any, "reload", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, o, rp := applyFixture(t, "")
			before := readFile(t, rp+"/docker-compose.yml")
			f.fail[c.sub] = errBoom
			if c.name == "db backup" {
				f.fail[c.sub] = errs.New("E206", "backup verify failed")
			}
			_, err := ApplyRemote(context.Background(), o)
			if err == nil || !c.errCheck(err) {
				t.Fatalf("err = %v", err)
			}
			got := steps(f)
			if i := indexOf(got, "tarlist"); i >= 0 { // the restore re-runs a prune: judge only the forward steps
				got = got[:i]
			}
			after := false
			for _, m := range orderedMilestones {
				if m == c.last {
					after = true
					continue
				}
				if after && indexOf(got, m) >= 0 {
					t.Errorf("step %q ran after the failure at %s: %v", m, c.name, got)
				}
			}
			if restored := indexOf(steps(f), "restore") >= 0; restored != c.restored {
				t.Errorf("restore ran = %v, want %v: %v", restored, c.restored, steps(f))
			}
			// Image tags go back only when the pull may have moved them and no container was touched.
			wantTags := c.restored && indexOf(orderedMilestones, c.last) > indexOf(orderedMilestones, "imgsave")
			if tags := indexOf(steps(f), "imgrestore") >= 0; tags != wantTags {
				t.Errorf("image tag restore ran = %v, want %v: %v", tags, wantTags, steps(f))
			}
			if c.restored && c.name != "rsync" {
				if after := readFile(t, rp+"/docker-compose.yml"); after != before {
					t.Errorf("the old compose was not put back: %q", after)
				}
			}
		})
	}
}

func TestRemoteApplyRefusals(t *testing.T) {
	t.Run("drift before anything is written", func(t *testing.T) {
		f, o, _ := applyFixture(t, "")
		o.LocalPlugins = []PluginRef{{Name: "ai", Version: "1.0.0"}}
		_, err := ApplyRemote(context.Background(), o)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E485" || !strings.Contains(err.Error(), "target drift") || !strings.Contains(err.Error(), `"ai" "1.0.0"`) {
			t.Fatalf("err = %v", err)
		}
		for _, s := range steps(f) {
			if s != "snapshot" {
				t.Fatalf("a step other than the snapshot ran before the drift refusal: %v", steps(f))
			}
		}
	})
	t.Run("a second server that differs from the build snapshot", func(t *testing.T) {
		_, o, _ := applyFixture(t, "")
		o.Expected = &TargetSnapshot{Plugins: []PluginRef{{Name: "ai", Version: "1.0.0"}}}
		if _, err := ApplyRemote(context.Background(), o); err == nil || !strings.Contains(err.Error(), "target drift") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("stateful recreate unconfirmed on prod", func(t *testing.T) {
		f, o, _ := applyFixture(t, "")
		o.Yes = false
		snapshots := 0
		o.Snapshot = func(context.Context, string) (string, error) { snapshots++; return "backup-1", nil }
		f.handler = func(s string) (string, bool) {
			if strings.Contains(s, "up -d --dry-run") {
				return " Container proj-db-1 Recreate\n", true
			}
			return stackHandler(o.RemotePath, "")(s)
		}
		res, err := ApplyRemote(context.Background(), o)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E403" || !strings.Contains(err.Error(), "db") {
			t.Fatalf("err = %v", err)
		}
		if res.Synced || !strings.Contains(readFile(t, o.RemotePath+"/docker-compose.yml"), "# v1") {
			t.Errorf("an E403 refusal must be restored and report Synced=false: %+v", res)
		}
		if snapshots != 1 {
			t.Errorf("the verified backup runs before the preview: snapshots = %d", snapshots)
		}
		got := steps(f)
		for _, s := range []string{"up:db", "reload", "hasura"} {
			if indexOf(got, s) >= 0 {
				t.Errorf("%s ran after E403: %v", s, got)
			}
		}
		// D-0426: the tags the pull moved point back at the images they had, so a
		// manual compose up cannot run the refused images. Absent and digest-pinned refs are left alone.
		var tagCmd string
		for _, c := range f.commands() {
			if strings.Contains(c, "docker tag") {
				tagCmd = c
			}
		}
		for _, want := range []string{"docker tag '" + fakeImageID("a") + "' 'postgres:16' || rc=1", "docker tag '" + fakeImageID("b") + "' 'web:1' || rc=1", "exit $rc"} {
			if !strings.Contains(tagCmd, want) {
				t.Errorf("tag restore %q lacks %q", tagCmd, want)
			}
		}
		if strings.Contains(tagCmd, "job:1") || strings.Contains(tagCmd, "nginx:") {
			t.Errorf("a ref the host lacked, or a digest pin, was retagged: %q", tagCmd)
		}
		if indexOf(got, "imgrestore") < indexOf(got, "preview") {
			t.Errorf("tags restored before the refusal: %v", got)
		}
	})
	t.Run("E403 says so when the image tags cannot be put back", func(t *testing.T) {
		f, o, _ := applyFixture(t, "")
		o.Yes = false
		f.fail["docker tag"] = errBoom
		var out strings.Builder
		o.Out = &out
		f.handler = func(s string) (string, bool) {
			if strings.Contains(s, "up -d --dry-run") {
				return " Container proj-db-1 Recreate\n", true
			}
			return stackHandler(o.RemotePath, "")(s)
		}
		_, err := ApplyRemote(context.Background(), o)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E403" || !strings.Contains(err.Error(), "image tags the pull moved could not be put back") {
			t.Fatalf("err = %v", err)
		}
		// The rendered E403 shows only the CLIError message, so the warning must reach the output too.
		if !strings.Contains(out.String(), "[warn] the image tags the pull moved could not be put back") {
			t.Errorf("no warning line in the output: %q", out.String())
		}
	})
	t.Run("non-prod takes no database backup and needs no confirm", func(t *testing.T) {
		f, o, _ := applyFixture(t, "")
		o.ProdClass, o.Yes, o.Snapshot = false, false, nil
		res, err := ApplyRemote(context.Background(), o)
		if err != nil || res.BackupID != "" || indexOf(steps(f), "dbbackup") >= 0 {
			t.Fatalf("err %v res %+v steps %v", err, res, steps(f))
		}
		if res.PreDeployTar == "" {
			t.Error("the pre-deploy tar is taken on every tier")
		}
	})
	t.Run("a crashed service stops the apply with E250", func(t *testing.T) {
		f, o, _ := applyFixture(t, "web")
		_, err := ApplyRemote(context.Background(), o)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E250" || !strings.Contains(err.Error(), "web") {
			t.Fatalf("err = %v", err)
		}
		got := steps(f)
		if indexOf(got, "up:nginx") >= 0 || indexOf(got, "hasura") >= 0 || indexOf(got, "restore") >= 0 {
			t.Errorf("later steps ran, or files were restored mid-apply: %v", got)
		}
	})
	t.Run("hasura metadata strict and warn", func(t *testing.T) {
		for _, strict := range []bool{true, false} {
			f, o, _ := applyFixture(t, "")
			o.StrictHasura = strict
			f.fail["hasura metadata apply"] = errBoom
			_, err := ApplyRemote(context.Background(), o)
			if (err != nil) != strict {
				t.Errorf("strict=%v: err = %v", strict, err)
			}
		}
	})
}
