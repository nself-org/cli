package deploy

// Tests for fix round 1 (F2-F18): each fails on the head before the round.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// F2: on a production-class target the verified backup comes right after the tar,
// before the sync and before any compose command; a failing one leaves neither.
func TestApplyVerifiedBackupBeforeAnyCompose(t *testing.T) {
	f, o, rp := applyFixture(t, "")
	o.Snapshot = func(ctx context.Context, id string) (string, error) {
		return "", errs.New("E206", "backup verify failed")
	}
	before := readFile(t, rp+"/docker-compose.yml")
	res, err := ApplyRemote(context.Background(), o)
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E206" {
		t.Fatalf("err = %v, want E206", err)
	}
	for _, c := range f.commands() {
		if strings.Contains(c, "docker compose") || strings.HasPrefix(c, "rsync ") {
			t.Fatalf("E206 came after %q:\n%s", c, strings.Join(f.commands(), "\n"))
		}
	}
	if res.Synced || readFile(t, rp+"/docker-compose.yml") != before {
		t.Errorf("files were synced before the backup was verified: %+v", res)
	}
	if exists(rp + "/.nself/deploy.lock") {
		t.Error("the deploy lock was left behind")
	}
}

// F4: both compose dry-run line forms parse; a line the preview cannot classify
// refuses a production-class deploy, even with --yes.
func TestPreviewFailsClosed(t *testing.T) {
	rp := "/opt/nself"
	model, _ := parseComposeModel([]byte(modelJSON(rp)))
	v2 := "DRY-RUN MODE -  Container proj-db-1  Recreate\nDRY-RUN MODE -  Container proj-web-1  Recreated\nDRY-RUN MODE -  Network proj_default  Creating\n"
	v5 := " Container proj-db-1 Recreate\n Container proj-web-1 Recreated\n Network proj_default Creating\n"
	for name, out := range map[string]string{"2.x": v2, "5.x": v5} {
		p := parseDryRun(out, model, rp)
		if strings.Join(p.Recreate, ",") != "db" || strings.Join(p.Stateful, ",") != "db" || len(p.Unknown) != 0 {
			t.Errorf("%s form: %+v", name, p)
		}
	}
	for name, out := range map[string]string{
		"unmapped container": " Container ghost-1 Recreate\n",
		"unparsed form":      "Recreating proj-db-1 ... done\n",
	} {
		p := parseDryRun(out, model, rp)
		if len(p.Unknown) != 1 {
			t.Fatalf("%s: %+v", name, p)
		}
		err := ConfirmStateful(p, true, true, nil)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E403" || !strings.Contains(err.Error(), strings.TrimSpace(p.Unknown[0])) {
			t.Errorf("%s: prod-class err = %v, want E403 naming the line", name, err)
		}
		if err := ConfirmStateful(p, false, false, nil); err != nil {
			t.Errorf("%s: a non-prod target only warns: %v", name, err)
		}
	}
}

// F5: a host last deployed by v1.4 (no manifests) gets none left behind after a
// refusal; a rollback removes what the deploy created.
func TestRestoreRemovesWhatTheDeployCreated(t *testing.T) {
	f, o, rp := applyFixture(t, "")
	for _, p := range []string{".nself/compose-files.txt", ".nself/compose-env-files.txt", ".nself/deploy-shipped.txt", ".env.prod", ".nself/compose.env"} {
		_ = os.Remove(filepath.Join(rp, p))
	}
	f.fail["docker run --rm"] = errBoom // nginx -t refuses after the sync
	if _, err := ApplyRemote(context.Background(), o); err == nil {
		t.Fatal("want the nginx -t refusal")
	}
	for _, p := range []string{".nself/compose-files.txt", ".nself/compose-env-files.txt", ".nself/deploy-shipped.txt", ".env.prod", ".nself/compose.env"} {
		if exists(filepath.Join(rp, p)) {
			t.Errorf("%s was left behind by the refused deploy", p)
		}
	}
	// Files the host already had are back to their old content.
	if got := readFile(t, rp+"/nginx/sites/api.conf"); !strings.Contains(got, "# v1") {
		t.Errorf("api.conf not restored: %q", got)
	}
	if !exists(filepath.Join(rp, "nginx/sites/foreign.conf")) || !exists(filepath.Join(rp, "nginx/conf.d/block.conf")) {
		t.Error("a foreign file was removed")
	}
	if got := readFile(t, rp+"/docker-compose.yml"); !strings.Contains(got, "# v1") {
		t.Errorf("compose not restored: %q", got)
	}
	if exists(rp + "/.nself/deploy.lock") {
		t.Error("the lock was left behind")
	}
}

// F8: a service whose build context the deploy cannot provide is refused before
// the first remote write; a context that exists on the host passes.
func TestBuildContextRefused(t *testing.T) {
	f, o, rp := applyFixture(t, "")
	compose := o.Set.Compose[0]
	rel := strings.TrimPrefix(compose, rp+"/")
	writeFile(t, filepath.Join(o.Set.StageDir, rel), "services:\n  api:\n    build: ./api\n  ai:\n    build:\n      context: ${NSELF_PLUGIN_DIR}/ghost\n  ok:\n    build: ./present\n", 0o644)
	writeFile(t, filepath.Join(rp, "present", "Dockerfile"), "FROM scratch\n", 0o644)
	_, err := ApplyRemote(context.Background(), o)
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E485" || !strings.Contains(err.Error(), "build context not shippable") || !strings.Contains(err.Error(), "ai, api") || strings.Contains(err.Error(), "ok") {
		t.Fatalf("err = %v", err)
	}
	for _, c := range f.commands() {
		if strings.Contains(c, "tar -czf") || strings.HasPrefix(c, "rsync ") || strings.Contains(c, "deploy.lock") {
			t.Fatalf("a write happened before the refusal: %q", c)
		}
	}
}

// F9: the throwaway nginx gets a host entry for every service name, container
// name and network alias, so proxy_pass to a service name passes nginx -t.
func TestNginxCheckAddsStackHosts(t *testing.T) {
	f := newFakeRemote(t)
	var run string
	f.handler = func(s string) (string, bool) {
		if strings.Contains(s, "docker run --rm") {
			run = s
			return "ok", true
		}
		return "", false
	}
	model, _ := parseComposeModel([]byte(`{"name":"p","services":{
	  "nginx":{"image":"nginx:1","volumes":[{"type":"bind","source":"/opt/n/nginx","target":"/etc/nginx/sites","read_only":true}]},
	  "hasura":{"image":"h:1","container_name":"p-hasura","networks":{"default":{"aliases":["graphql","bad host;rm"]}}}}}`))
	if err := CheckNginx(context.Background(), f, "/opt/n", model, os.Stderr); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"hasura", "nginx", "p-hasura", "graphql"} {
		if !strings.Contains(run, "--add-host '"+h+":127.0.0.1'") {
			t.Errorf("no host entry for %s in %q", h, run)
		}
	}
	if strings.Contains(run, "bad host") {
		t.Errorf("an invalid alias reached the command: %q", run)
	}
}

// F10: nginx is reloaded after the apply only when it is running.
func TestNginxReloadOnlyWhenRunning(t *testing.T) {
	f, o, _ := applyFixture(t, "")
	if _, err := ApplyRemote(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if indexOf(steps(f), "reload") < 0 {
		t.Fatalf("no reload: %v", steps(f))
	}
	// A stack without an nginx service is never reloaded.
	f2, o2, rp2 := applyFixture(t, "")
	inner := stackHandler(rp2, "")
	f2.handler = func(s string) (string, bool) {
		switch {
		case strings.Contains(s, "config --format json"):
			return `{"name":"proj","services":{"db":{"image":"p:1","healthcheck":{"test":["CMD","x"]}},"web":{"image":"w:1","restart":"always"}}}`, true
		case strings.Contains(s, "config --services"):
			return "db\nweb\n", true
		}
		return inner(s)
	}
	if _, err := ApplyRemote(context.Background(), o2); err != nil {
		t.Fatal(err)
	}
	if indexOf(steps(f2), "reload") >= 0 {
		t.Errorf("a stack without nginx was reloaded: %v", steps(f2))
	}
}

// F12: the restore completes although the caller's context is cancelled.
func TestRestoreSurvivesCancellation(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	seedDeployedHost(t, rp)
	bk, err := CreatePreDeployTar(context.Background(), f, rp, NewBackupID(time.Now()), []string{"docker-compose.yml"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, rp+"/docker-compose.yml", "# broken\n", 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RestorePreDeployTar(ctx, f, rp, bk.Tar); err != nil {
		t.Fatalf("restore with a cancelled context: %v", err)
	}
	if got := readFile(t, rp+"/docker-compose.yml"); !strings.Contains(got, "# v1") {
		t.Errorf("not restored: %q", got)
	}
}

// F13: a second deploy or rollback refuses while the lock is held, naming the holder.
func TestDeployLock(t *testing.T) {
	needTools(t)
	f, o, rp := applyFixture(t, "")
	release, err := AcquireLock(context.Background(), f, rp, "20261010T000000Z-aaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	before := readFile(t, rp+"/docker-compose.yml")
	_, err = ApplyRemote(context.Background(), o)
	if err == nil || !strings.Contains(err.Error(), "holds the lock") || !strings.Contains(err.Error(), "id=20261010T000000Z-aaaaaa") || !strings.Contains(err.Error(), "rmdir") {
		t.Fatalf("err = %v", err)
	}
	if readFile(t, rp+"/docker-compose.yml") != before {
		t.Error("a deploy ran under a held lock")
	}
	if err := RollbackRemote(context.Background(), RollbackOptions{Remote: f, RemotePath: rp, Tar: rp + "/.nself/backups/pre-deploy-20261010T000000Z-bbbbbb.tar.gz"}); err == nil || !strings.Contains(err.Error(), "holds the lock") {
		t.Fatalf("rollback under a held lock: %v", err)
	}
	release()
	if exists(rp + "/.nself/deploy.lock") {
		t.Fatal("release left the lock")
	}
	if _, err := ApplyRemote(context.Background(), o); err != nil {
		t.Fatalf("after release: %v", err)
	}
}

// F15 + F16: service names follow `--`; a failure after the first up names the
// tar and the rollback command.
func TestApplyArgvAndRollbackHint(t *testing.T) {
	f, o, _ := applyFixture(t, "web")
	o.Env = "prod"
	res, err := ApplyRemote(context.Background(), o)
	if err == nil || !strings.Contains(err.Error(), res.PreDeployTar) || !strings.Contains(err.Error(), "nself deploy rollback prod") {
		t.Fatalf("err = %v", err)
	}
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E250" {
		t.Errorf("the gate code was lost: %v", err)
	}
	for _, c := range f.commands() {
		if (strings.Contains(c, "--no-deps") && !strings.Contains(c, "--no-deps -- '")) || (strings.Contains(c, "ps -a --format json") && !strings.Contains(c, "json -- '")) {
			t.Errorf("service name not after --: %q", c)
		}
	}
}

// F18 (N3): a plugin fragment with a space in its path is saved by the tar.
func TestTarKeepsOddFragmentNames(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	writeFile(t, rp+"/.nself/plugins/my plugin/docker-compose.plugin.yml", "services: {}\n", 0o644)
	bk, err := CreatePreDeployTar(context.Background(), f, rp, NewBackupID(time.Now()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tarList(t, bk.Tar), "my plugin/docker-compose.plugin.yml") {
		t.Errorf("tar lacks the fragment:\n%s", tarList(t, bk.Tar))
	}
}

// F18 (CR S2): the copied parsers agree with internal/docker's on the same
// documents, through a stub docker binary.
func TestCopiedParsersMatchDocker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	dir := t.TempDir()
	cfg := `{"name":"p","services":{
	  "a":{"restart":"no"},
	  "b":{"restart":"always","healthcheck":{"test":["CMD","true"]}},
	  "c":{"restart":"no","healthcheck":{"test":["NONE"]}},
	  "d":{"restart":"no","healthcheck":{"disable":true,"test":["CMD","x"]}},
	  "e":{"healthcheck":{"test":"curl -f localhost"}}}}`
	psArray := `[{"ID":"1","Name":"p-a-1","State":"running","Health":"healthy","Labels":{"com.docker.compose.oneoff":"False"}},
	  {"ID":"2","Name":"p-a-run-x","State":"exited","ExitCode":3,"Labels":"com.docker.compose.oneoff=True,x=y"},
	  {"ID":"3","Name":"p-a-2","State":"exited","ExitCode":1}]`
	psLines := strings.ReplaceAll(strings.ReplaceAll(psArray[1:len(psArray)-1], "},\n", "}\n"), "\t", "")
	writeFile(t, filepath.Join(dir, "cfg.json"), cfg, 0o644)
	stub := filepath.Join(dir, "docker")
	script := "#!/bin/sh\ncase \"$*\" in\n*\"config --format json\"*) cat " + dir + "/cfg.json;;\n*\"ps -a --format json\"*) cat \"" + dir + "/ps-$PSFORM.json\";;\nesac\n"
	writeFile(t, stub, script, 0o755)
	writeFile(t, filepath.Join(dir, "ps-array.json"), psArray, 0o644)
	writeFile(t, filepath.Join(dir, "ps-lines.json"), strings.ReplaceAll(psLines, ",\n  {", "\n{"), 0o644)
	c := docker.NewCompose("f.yml")
	c.DockerPath = stub
	specs, err := docker.ComposeServiceSpecs(context.Background(), c, dir)
	if err != nil {
		t.Fatal(err)
	}
	mine, err := parseComposeModel([]byte(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != len(mine.Services) {
		t.Fatalf("service counts differ: %d vs %d", len(specs), len(mine.Services))
	}
	for _, sp := range specs {
		if got := mine.Services[sp.Name].Spec; got != sp {
			t.Errorf("service %s: docker %+v, copy %+v", sp.Name, sp, got)
		}
	}
	for _, form := range []string{"array", "lines"} {
		t.Setenv("PSFORM", form)
		want, err := c.ComposeServiceStates(context.Background(), dir, "a")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(filepath.Join(dir, "ps-"+form+".json"))
		all, err := decodeContainerStates(raw)
		if err != nil {
			t.Fatal(err)
		}
		var got []docker.ContainerState
		for _, s := range all {
			if !s.OneOff() {
				got = append(got, s)
			}
		}
		if len(got) != len(want) {
			t.Fatalf("%s: %d rows vs docker's %d", form, len(got), len(want))
		}
		for i := range got {
			if got[i].ID != want[i].ID || got[i].State != want[i].State || got[i].ExitCode != want[i].ExitCode || got[i].Health != want[i].Health {
				t.Errorf("%s row %d: copy %+v, docker %+v", form, i, got[i], want[i])
			}
		}
	}
}
