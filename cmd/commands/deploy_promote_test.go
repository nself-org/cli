package commands

import (
	"bytes"
	"context"
	"fmt"
	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

func promoteFixture(t *testing.T) string {
	t.Helper()
	isolateDeployConfigEnv(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `schema_version: 2
project: fixture
environments:
  local:
    name: local
    kind: local
    tier: local
    servers: []
  ls:
    name: ls
    kind: remote
    tier: local-servers
    servers:
      - {name: ls-app, role: app, host: tester@ls.example.invalid, ssh_key_ref: TEST_KEY, remote_path: /srv/fixture, primary: true, arch: amd64}
  prodsim:
    name: prodsim
    kind: remote
    tier: prod
    servers:
      - {name: prod-app, role: app, host: tester@prod.example.invalid, ssh_key_ref: TEST_KEY, remote_path: /srv/fixture, primary: true, arch: amd64}
`
	if err := os.WriteFile(filepath.Join(dir, ".nself", "control-plane.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

func promoteTestCommand(out *bytes.Buffer, dry, yes, jsonOut bool) *cobra.Command {
	c := &cobra.Command{Use: "promote"}
	c.SetOut(out)
	c.Flags().Bool("dry-run", dry, "")
	c.Flags().Bool("yes", yes, "")
	c.Flags().Bool("json", jsonOut, "")
	return c
}

func TestPromoteDryRun(t *testing.T) {
	compattest.Set(t, true)
	golden, err := os.ReadFile(filepath.Join("testdata", "json", "deploy-promote.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	promoteFixture(t)
	var out bytes.Buffer
	cmd := promoteTestCommand(&out, true, false, false)
	if err := runPromoteEnvironments(cmd, "ls", "prodsim"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := preparePromoteEngine(cmd, mustProjectRoot(t), "local", "ls", true, false); err != nil {
		t.Fatalf("local source dry-run: %v", err)
	}
	for _, name := range []string{"resolve", "confirm", "verify", "preflight", "source-health", "capture", "build", "compare", "snapshot", "save", "apply", "migrate", "health", "record"} {
		if !strings.Contains(out.String(), name) {
			t.Errorf("plan missing %s", name)
		}
	}
	if err := runPromoteEnvironments(promoteTestCommand(&bytes.Buffer{}, false, false, false), "ls", "prodsim"); errs.ExitCodeFor(err) != 4 {
		t.Fatalf("prod without --yes: %v (exit %d), want E403/4", err, errs.ExitCodeFor(err))
	}
	var jsonDoc bytes.Buffer
	oldWriter := pilotWriter
	pilotWriter = func() output.Writer { return output.Writer{Out: &jsonDoc, Err: &bytes.Buffer{}} }
	t.Cleanup(func() { pilotWriter = oldWriter; output.ResetState() })
	if err := runPromoteEnvironments(promoteTestCommand(&bytes.Buffer{}, true, false, true), "ls", "prodsim"); err != nil {
		t.Fatal(err)
	}
	if bytes.Count(jsonDoc.Bytes(), []byte(`"schema_version"`)) != 1 || !bytes.Contains(jsonDoc.Bytes(), []byte(`"dry_run": true`)) {
		t.Fatalf("dry-run JSON is not one envelope: %s", jsonDoc.Bytes())
	}
	if !bytes.Equal(jsonDoc.Bytes(), golden) {
		t.Fatalf("dry-run JSON differs from golden\n--- got ---\n%s\n--- want ---\n%s", jsonDoc.Bytes(), golden)
	}
}

func TestPromoteArity(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		promoteFixture(t)
		cmd := promoteTestCommand(&bytes.Buffer{}, true, false, false)
		if err := runDeployPromote(cmd, nil); err != nil {
			t.Fatalf("zero-argument canary path: %v", err)
		}
		err := runDeployPromote(cmd, []string{"only-one"})
		if compat.V15() {
			if errs.ExitCodeFor(err) != 1 || !strings.Contains(err.Error(), "E401") {
				t.Fatalf("v1.5 one arg = %v, want E401", err)
			}
		} else if err != nil && strings.Contains(err.Error(), "E401") {
			t.Fatalf("v1.4 interpreted args: %v", err)
		} else {
			old, _ := promoteCmd.Flags().GetBool("dry-run")
			_ = promoteCmd.Flags().Set("dry-run", "true")
			t.Cleanup(func() { _ = promoteCmd.Flags().Set("dry-run", fmt.Sprint(old)) })
			if legacyErr := promoteCmd.RunE(promoteCmd, []string{"ls", "prodsim"}); legacyErr != nil {
				t.Fatalf("v1.4 promote did not reach internal/promote dry-run: %v", legacyErr)
			}
		}
	})
}

func TestPromoteEquivalence(t *testing.T) {
	compattest.Set(t, true)
	promoteFixture(t)
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()
	deployDry, _ := deployPromoteCmd.Flags().GetBool("dry-run")
	promoteDry, _ := promoteCmd.Flags().GetBool("dry-run")
	deployOut, promoteOut := deployPromoteCmd.OutOrStdout(), promoteCmd.OutOrStdout()
	t.Cleanup(func() {
		_ = deployPromoteCmd.Flags().Set("dry-run", fmt.Sprint(deployDry))
		_ = promoteCmd.Flags().Set("dry-run", fmt.Sprint(promoteDry))
		deployPromoteCmd.SetOut(deployOut)
		promoteCmd.SetOut(promoteOut)
	})
	oldArgs := os.Args
	os.Args = []string{"nself", "--quiet"}
	t.Cleanup(func() { os.Args = oldArgs })
	resetCanonWarning()
	var warnings bytes.Buffer
	run := func(argv string) string {
		rewritten, notes, err := rewriteCanonArgsWith(&canonTable, RootCmd, strings.Fields(argv), true)
		if err != nil {
			t.Fatal(err)
		}
		emitCanonWarning(&warnings, strings.Fields(argv), notes, "", "deploy promote")
		cmd, args, err := RootCmd.Find(rewritten)
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Flags().Set("dry-run", "true"); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := cmd.RunE(cmd, args); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	canonical := run("deploy promote ls prodsim")
	legacy := run("deploy promote-env ls prodsim")
	top := run("promote ls prodsim")
	if canonical != legacy || canonical != top {
		t.Fatalf("promotion spellings produced different plans")
	}
	if got := strings.Count(warnings.String(), "warning:"); got != 1 {
		t.Fatalf("deprecation warnings = %d, want one: %q", got, warnings.String())
	}
}

func mustProjectRoot(t *testing.T) string {
	t.Helper()
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

type promoteSafetyRemote struct {
	label, repository, tag, imageID, index string
	backupTag                              string
	failVerify                             bool
	log                                    []string
}

func (r *promoteSafetyRemote) Label() string { return r.label }
func (r *promoteSafetyRemote) Rsync(_ context.Context, _ []string, _, _, _ string) error {
	r.log = append(r.log, "rsync")
	return nil
}
func (r *promoteSafetyRemote) Run(_ context.Context, command string) ([]byte, error) {
	r.log = append(r.log, command)
	switch {
	case strings.Contains(command, `printf '%s\n' "$HOME"`):
		return []byte("/home/deploy\n"), nil
	case strings.Contains(command, "compose-files.txt") || strings.Contains(command, "compose-env-files.txt"):
		return []byte("__NSELF_ABSENT__"), nil
	case strings.Contains(command, " ps --all --format json"):
		return []byte(`{"Name":"fixture-api-1","Service":"api","State":"running"}` + "\n"), nil
	case strings.Contains(command, " images --format json"):
		return []byte(fmt.Sprintf(`[{"ContainerName":"fixture-api-1","Repository":%q,"Tag":%q,"ID":%q}]`, r.repository, r.tag, r.imageID)), nil
	case strings.Contains(command, " config --format json"):
		return []byte(fmt.Sprintf(`{"services":{"api":{"image":%q}}}`, r.repository+":"+r.tag)), nil
	case strings.Contains(command, "docker image inspect"):
		return []byte(fmt.Sprintf(`[%q]|linux/amd64`, r.repository+"@"+r.index)), nil
	case strings.Contains(command, "nself backup create"):
		match := tagArg.FindStringSubmatch(command)
		if len(match) == 2 {
			r.backupTag = match[1]
		}
		return nil, nil
	case strings.Contains(command, "nself backup list"):
		return []byte(fmt.Sprintf(`[{"id":"backup_%s","tag":%q,"date":"2026-10-11T00:00:00Z"}]`, r.backupTag, r.backupTag)), nil
	case strings.Contains(command, "nself backup verify") && r.failVerify:
		return nil, fmt.Errorf("injected backup verify failure")
	default:
		return nil, nil
	}
}

func setupPromoteSafety(t *testing.T, sourceRef, targetRef string, failVerify bool) (*promoteSafetyRemote, *promoteSafetyRemote) {
	t.Helper()
	root := promoteFixture(t)
	bin := t.TempDir()
	wf(t, filepath.Join(bin, "docker"), "#!/bin/sh\nprintf '%s' '{\"services\":{\"api\":{\"image\":\""+targetRef+"\"}}}'\n")
	if err := os.Chmod(filepath.Join(bin, "docker"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	wf(t, filepath.Join(root, "docker-compose.yml"), "services:\n  api:\n    image: "+sourceRef+"\n")
	wf(t, filepath.Join(root, ".env"), "PROJECT_NAME=fixture\n")
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	t.Setenv("NSELF_HASURA_METADATA_STRICT", "false")
	sourceRepo, sourceTag := splitImageTag(sourceRef)
	sourceLock := lockedRefByLegacy(t, sourceRef)
	source := &promoteSafetyRemote{label: "ls-app", repository: sourceRepo, tag: sourceTag, imageID: "sha256:running-source", index: sourceLock.IndexDigest}
	target := &promoteSafetyRemote{label: "prod-app", failVerify: failVerify, repository: sourceRepo, tag: sourceTag, imageID: "sha256:running-target", index: sourceLock.IndexDigest}
	oldOpen, oldBuild, oldProber := deployRemoteOpen, deployScratchBuildFn, newDeployProber
	t.Cleanup(func() { deployRemoteOpen, deployScratchBuildFn, newDeployProber = oldOpen, oldBuild, oldProber })
	deployRemoteOpen = func(_ context.Context, srv controlplane.Server, _, _ string) (deploy.Remote, error) {
		if srv.Name == "ls-app" {
			return source, nil
		}
		return target, nil
	}
	deployScratchBuildFn = func(_ context.Context, workdir string, _ bool, steps []deployStep) ([]deployStep, error) {
		wf(t, filepath.Join(workdir, "docker-compose.yml"), "services:\n  api:\n    image: "+targetRef+"\n")
		wf(t, filepath.Join(workdir, ".nself", "compose-files.txt"), filepath.Join(workdir, "docker-compose.yml")+"\n")
		wf(t, filepath.Join(workdir, ".nself", "compose-env-files.txt"), filepath.Join(workdir, ".env")+"\n")
		return steps, nil
	}
	newDeployProber = func(string) controlplane.Prober { return okProber{} }
	return source, target
}

func splitImageTag(ref string) (string, string) {
	i := strings.LastIndex(ref, ":")
	return ref[:i], ref[i+1:]
}

func lockedRefByLegacy(t *testing.T, ref string) compose.Ref {
	t.Helper()
	for _, locked := range compose.LockedImages() {
		if locked.LegacyRef == ref {
			return locked
		}
	}
	t.Fatalf("locked legacy ref %q not found", ref)
	return compose.Ref{}
}

func assertNoPromoteWrites(t *testing.T, commands []string) {
	t.Helper()
	for _, command := range commands {
		if command == "rsync" || strings.Contains(command, "docker compose") || strings.Contains(command, "compose-files.txt") || strings.Contains(command, "compose-env-files.txt") || strings.Contains(command, "tar -czf") || strings.Contains(command, "db migrate") {
			t.Fatalf("promotion wrote before refusal: %s\nlog:\n%s", command, strings.Join(commands, "\n"))
		}
	}
}

func TestPromoteVerifyStopsApply(t *testing.T) {
	skipPromoteShellDoubles(t)
	compattest.Set(t, true)
	locked := compose.LockedImages()[0]
	_, target := setupPromoteSafety(t, locked.LegacyRef, locked.String(), true)
	err := runPromoteEnvironments(promoteTestCommand(&bytes.Buffer{}, false, true, false), "ls", "prodsim")
	if err == nil || !strings.Contains(err.Error(), "E206") {
		t.Fatalf("promotion error = %v, want E206", err)
	}
	if firstIndex(target.log, "nself backup verify") < 0 {
		t.Fatalf("backup verify did not run: %v", target.log)
	}
	assertNoPromoteWrites(t, target.log)
}

func TestPromoteDriftStopsWrite(t *testing.T) {
	skipPromoteShellDoubles(t)
	compattest.Set(t, true)
	locked := compose.LockedImages()
	_, target := setupPromoteSafety(t, locked[0].LegacyRef, locked[1].String(), false)
	err := runPromoteEnvironments(promoteTestCommand(&bytes.Buffer{}, false, true, false), "ls", "prodsim")
	if err == nil || !strings.Contains(err.Error(), "E490") || !strings.Contains(err.Error(), "api") {
		t.Fatalf("promotion error = %v, want E490", err)
	}
	if firstIndex(target.log, "nself backup create") >= 0 {
		t.Fatalf("drift reached backup create: %v", target.log)
	}
	assertNoPromoteWrites(t, target.log)
}

func TestPromoteUnknownNames(t *testing.T) {
	root := promoteFixture(t)
	for _, names := range [][2]string{{"missing-source", "prodsim"}, {"ls", "missing-target"}} {
		_, _, err := preparePromoteEngine(promoteTestCommand(&bytes.Buffer{}, true, false, false), root, names[0], names[1], true, false)
		if err == nil || !strings.Contains(err.Error(), "missing-") {
			t.Fatalf("unknown name lost: %v", err)
		}
	}
}

func TestPromoteRecordRollbackCommands(t *testing.T) {
	for _, shim := range []bool{false, true} {
		t.Run(fmt.Sprint(shim), func(t *testing.T) {
			compattest.Set(t, true)
			root := promoteFixture(t)
			rec := cpromote.Record{ID: "fixture", Source: "ls", Target: "prodsim", Status: "ok", CompletedAt: time.Now(), BackupID: "fixture_backup", BackupHost: "prod-app", SnapshotTag: "pre-promote-fixture", PreDeployTar: map[string]string{"prod-app": "/srv/fixture/.nself/backups/pre-deploy-fixture.tar.gz"}, Changed: map[string][]string{"prod-app": {"docker-compose.yml"}}}
			rec.MigrateStarted = true
			rec.RemotePaths = map[string]string{"prod-app": "/srv/fixture"}
			rec.AppliedManifestHash = map[string]string{"prod-app": cpromote.ManifestHash(nil)}
			if err := cpromote.SaveRecord(root, rec); err != nil {
				t.Fatal(err)
			}
			r := &v15Remote{label: "prod-app"}
			old := deployRemoteOpen
			t.Cleanup(func() { deployRemoteOpen = old })
			deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) { return r, nil }
			tree := &cobra.Command{Use: "nself"}
			mounted := deployCmd
			args := []string{"deploy", "rollback", "prodsim", "--yes"}
			if shim {
				mounted, args = promoteCmd, []string{"promote", "rollback", "--yes"}
			}
			if mounted.GroupID != "" {
				tree.AddGroup(&cobra.Group{ID: mounted.GroupID, Title: "Fixture commands"})
			}
			parent := mounted.Parent()
			if parent != nil {
				parent.RemoveCommand(mounted)
			}
			tree.AddCommand(mounted)
			t.Cleanup(func() {
				tree.RemoveCommand(mounted)
				if parent != nil {
					parent.AddCommand(mounted)
				}
			})
			tree.SetArgs(args)
			err := tree.Execute()
			if err != nil {
				t.Fatal(err)
			}
			log := r.commands()
			prev := -1
			for _, event := range []string{"tar -xzf", "up -d --no-deps -- 'web'", "nself backup restore 'fixture_backup' --yes", "nself doctor"} {
				at := firstIndex(log, event)
				if at <= prev {
					t.Fatalf("event %q at %d after %d: %v", event, at, prev, log)
				}
				prev = at
			}
		})
	}
}

func TestPromoteInapplicableRecordUsesDeployRecord(t *testing.T) {
	for _, superseded := range []bool{false, true} {
		t.Run(fmt.Sprint(superseded), func(t *testing.T) {
			compattest.Set(t, true)
			root := promoteFixture(t)
			when := time.Now()
			rec := cpromote.Record{ID: "foreign", Target: "ls", Status: "ok", CompletedAt: when}
			if superseded {
				rec.Target = "prodsim"
			}
			if err := cpromote.SaveRecord(root, rec); err != nil {
				t.Fatal(err)
			}
			state := deploy.StrategyState{Target: "prodsim", CompletedAt: when.Add(time.Second), BackupHost: "prod-app", PreDeployTars: map[string]string{"prod-app": "/srv/fixture/.nself/backups/pre-deploy-fixture.tar.gz"}, PreDeployChanged: map[string][]string{"prod-app": {"docker-compose.yml"}}}
			if err := deploy.SaveDeployState(root, state); err != nil {
				t.Fatal(err)
			}
			r := &v15Remote{label: "prod-app"}
			old := deployRemoteOpen
			t.Cleanup(func() { deployRemoteOpen = old })
			deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) { return r, nil }
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			if err := runDeployRollback(cmd, []string{"prodsim"}); err != nil {
				t.Fatal(err)
			}
			if firstIndex(r.commands(), "tar -xzf") < 0 {
				t.Fatalf("inapplicable promotion blocked deploy rollback: %v", r.commands())
			}
		})
	}
}

func TestPromoteSameTargetCorruptRecordRefused(t *testing.T) {
	for _, corruption := range []string{"json", "status", "tar", "changed"} {
		t.Run(corruption, func(t *testing.T) {
			compattest.Set(t, true)
			root := promoteFixture(t)
			rec := cpromote.Record{ID: "fixture", Target: "prodsim", Status: "ok", CompletedAt: time.Now(), PreDeployTar: map[string]string{"prod-app": "/srv/fixture/.nself/backups/pre-deploy-fixture.tar.gz"}, Changed: map[string][]string{"prod-app": {"docker-compose.yml"}}}
			switch corruption {
			case "status":
				rec.Status = "unknown"
			case "tar":
				rec.PreDeployTar = nil
			case "changed":
				rec.Changed = nil
			}
			if err := cpromote.SaveRecord(root, rec); err != nil {
				t.Fatal(err)
			}
			if corruption == "json" {
				wf(t, filepath.Join(root, cpromote.RecordFile), `{"target":"prodsim",broken`)
			}
			r := &v15Remote{label: "prod-app"}
			old := deployRemoteOpen
			t.Cleanup(func() { deployRemoteOpen = old })
			opens := 0
			deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) {
				opens++
				return r, nil
			}
			handled, err := rollbackPromoteForTarget(context.Background(), root, "prodsim")
			if !handled || err == nil || !strings.Contains(err.Error(), "E485") || opens != 0 || len(r.commands()) != 0 {
				t.Fatalf("handled=%v err=%v opens=%d remote=%v", handled, err, opens, r.commands())
			}
		})
	}
}

func TestPromoteApplyPreservesRecord(t *testing.T) {
	root, r := v15Project(t)
	key := filepath.Join(t.TempDir(), "key")
	wf(t, key, "fake")
	t.Setenv("TEST_KEY", key)
	inv := prodInventory(controlplane.Server{Name: "prod-app", Role: controlplane.RoleApp, Host: "deploy@app.example.test", SSHKeyRef: "TEST_KEY", RemotePath: "/opt/nself", Primary: true})
	ra, composePath, err := prepareRemoteApply(context.Background(), v15Params{workdir: root, target: "prod", srv: inv.Environments["prod"].Servers[0], yes: true, jsonOut: true})
	if err != nil {
		t.Fatal(err)
	}
	old := newDeployProber
	t.Cleanup(func() { newDeployProber = old })
	newDeployProber = func(string) controlplane.Prober { return okProber{} }
	w := &promoteWiring{workdir: root, target: "prod", inv: inv, apply: ra, compose: composePath, yes: true}
	st := &cpromote.State{Values: map[string]any{}, BackupID: "fixture_backup", BackupHost: r.Label()}
	if err := runPromoteApply(w)(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	state, err := deploy.LoadDeployState(root)
	if err != nil || state.Target != "prod" || len(state.PreDeployTars) == 0 {
		t.Fatalf("deploy state lost: %+v %v", state, err)
	}
}

func TestPromoteStatefulConfirmation(t *testing.T) {
	root, r := v15Project(t)
	r.stateful = true
	key := filepath.Join(t.TempDir(), "key")
	wf(t, key, "fake")
	t.Setenv("TEST_KEY", key)
	inv := prodInventory(controlplane.Server{Name: "prod-app", Role: controlplane.RoleApp, Host: "deploy@app.example.test", SSHKeyRef: "TEST_KEY", RemotePath: "/opt/nself", Primary: true})
	ra, composePath, err := prepareRemoteApply(context.Background(), v15Params{workdir: root, target: "prod", srv: inv.Environments["prod"].Servers[0], yes: true, jsonOut: true})
	if err != nil {
		t.Fatal(err)
	}
	old := newDeployProber
	t.Cleanup(func() { newDeployProber = old })
	newDeployProber = func(string) controlplane.Prober { return okProber{} }
	w := &promoteWiring{workdir: root, target: "prod", inv: inv, apply: ra, compose: composePath}
	err = runPromoteApply(w)(context.Background(), &cpromote.State{Values: map[string]any{}, BackupID: "fixture_backup", BackupHost: r.Label()})
	if err == nil || !strings.Contains(err.Error(), "E403") || errs.ExitCodeFor(err) != 4 {
		t.Fatalf("confirmation bypass: %v", err)
	}
	if firstIndex(r.commands(), "up -d --no-deps") >= 0 {
		t.Fatal("unconfirmed recreate applied")
	}
}

func TestPromoteCompareResolvedManifest(t *testing.T) {
	skipPromoteShellDoubles(t)
	root := promoteFixture(t)
	locked := compose.LockedImages()
	wf(t, filepath.Join(root, "docker-compose.yml"), "services:\n  api:\n    image: "+locked[0].String()+"\n")
	wf(t, filepath.Join(root, "override.yml"), "services:\n  api:\n    image: ${PROMOTE_IMAGE}\n")
	wf(t, filepath.Join(root, ".nself", "compose-files.txt"), "docker-compose.yml\noverride.yml\n")
	wf(t, filepath.Join(root, ".env"), "PROMOTE_IMAGE="+locked[1].String()+"\n")
	wf(t, filepath.Join(root, ".nself", "compose-env-files.txt"), ".env\n")
	bin := t.TempDir()
	script := `#!/bin/sh
case "$*" in
 *"override.yml"*"--env-file"*"config --format json"*) printf '%s' '{"services":{"api":{"image":"` + locked[1].String() + `"}}}' ;;
 *) exit 3 ;;
esac
`
	wf(t, filepath.Join(bin, "docker"), script)
	if err := os.Chmod(filepath.Join(bin, "docker"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	w := &promoteWiring{workdir: root, compose: filepath.Join(root, "docker-compose.yml"), targetServer: controlplane.Server{Arch: "amd64"}}
	st := &cpromote.State{TargetTier: controlplane.TierProd, Values: map[string]any{"source_images": []cpromote.SourceImage{{Service: "api", Digest: locked[0].Platforms["linux/amd64"]}}}}
	err := runPromoteCompare(w)(context.Background(), st)
	if err == nil || !strings.Contains(err.Error(), "E490") {
		t.Fatalf("fragment drift passed: %v", err)
	}
}

func TestPromoteCompareServicePlatformFragment(t *testing.T) {
	skipPromoteShellDoubles(t)
	root := promoteFixture(t)
	locked := compose.LockedImages()[0]
	wf(t, filepath.Join(root, "docker-compose.yml"), "services:\n  api:\n    image: "+locked.String()+"\n")
	wf(t, filepath.Join(root, "override.yml"), "services:\n  api:\n    platform: linux/arm64\n")
	wf(t, filepath.Join(root, ".nself", "compose-files.txt"), "docker-compose.yml\noverride.yml\n")
	bin := t.TempDir()
	script := `#!/bin/sh
case "$*" in
 *"override.yml"*"config --format json"*) printf '%s' '{"services":{"api":{"image":"` + locked.String() + `","platform":"linux/arm64"}}}' ;;
 *) exit 3 ;;
esac
`
	wf(t, filepath.Join(bin, "docker"), script)
	if err := os.Chmod(filepath.Join(bin, "docker"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	r := &promoteSafetyRemote{label: "prod-app"}
	w := &promoteWiring{workdir: root, compose: filepath.Join(root, "docker-compose.yml"), targetServer: controlplane.Server{Arch: "amd64"}, targetRemote: r}
	st := &cpromote.State{TargetTier: controlplane.TierProd, Values: map[string]any{"source_images": []cpromote.SourceImage{{Service: "api", Digest: locked.Platforms["linux/amd64"], Platform: "linux/amd64"}}}}
	err := runPromoteCompare(w)(context.Background(), st)
	if err == nil || !strings.Contains(err.Error(), "E490") || !strings.Contains(err.Error(), "api") {
		t.Fatalf("service platform drift passed: %v", err)
	}
	if len(r.log) != 0 {
		t.Fatalf("platform refusal reached target writes: %v", r.log)
	}
}

func TestPromoteRolledBackRecordNoRemote(t *testing.T) {
	compattest.Set(t, true)
	root := promoteFixture(t)
	if err := cpromote.SaveRecord(root, cpromote.Record{ID: "fixture", Target: "prodsim", Status: "rolled_back", CompletedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	old := deployRemoteOpen
	t.Cleanup(func() { deployRemoteOpen = old })
	opens := 0
	deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) {
		opens++
		return &v15Remote{}, nil
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := deployRollbackCmd.RunE(cmd, []string{"prodsim"})
	if err == nil || !strings.Contains(err.Error(), "the last promote of prodsim was already rolled back; nothing was changed") || opens != 0 {
		t.Fatalf("err=%v opens=%d", err, opens)
	}
}

func TestPromoteRecordMigrationEvidence(t *testing.T) {
	root := promoteFixture(t)
	w := &promoteWiring{workdir: root, started: time.Now()}
	st := &cpromote.State{Source: "ls", Target: "prodsim", MigrateStarted: true, DBRestoredAt: "fixture-restored", Values: map[string]any{}}
	if err := savePromoteRecord(w, st, "rollback_failed"); err != nil {
		t.Fatal(err)
	}
	rec, err := cpromote.LoadAttempt(root)
	if err != nil || rec == nil || !rec.MigrateStarted || rec.DBRestoredAt != st.DBRestoredAt {
		t.Fatalf("record evidence: %+v %v", rec, err)
	}
}

func TestPromoteRollbackAbsentUsesDeployRecord(t *testing.T) {
	compattest.Set(t, true)
	root := promoteFixture(t)
	state := deploy.StrategyState{Target: "prodsim", BackupHost: "prod-app", PreDeployTars: map[string]string{"prod-app": "/srv/fixture/.nself/backups/pre-deploy-fixture.tar.gz"}, PreDeployChanged: map[string][]string{"prod-app": {"docker-compose.yml"}}}
	if err := deploy.SaveDeployState(root, state); err != nil {
		t.Fatal(err)
	}
	r := &v15Remote{label: "prod-app"}
	old := deployRemoteOpen
	t.Cleanup(func() { deployRemoteOpen = old })
	deployRemoteOpen = func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) { return r, nil }
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runPromoteRecordRollback(cmd); err != nil {
		t.Fatal(err)
	}
	if firstIndex(r.commands(), "tar -xzf") < 0 {
		t.Fatal("absent promote did not use deploy record")
	}
}

func TestPromoteRollbackNoLocalSnapshotFallback(t *testing.T) {
	for _, shim := range []bool{false, true} {
		t.Run(fmt.Sprint(shim), func(t *testing.T) {
			compattest.Set(t, true)
			promoteFixture(t)
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			var err error
			if shim {
				err = runPromoteRecordRollback(cmd)
			} else {
				err = runDeployRollback(cmd, []string{"prodsim"})
			}
			if err == nil || !strings.Contains(err.Error(), "E485") || !strings.Contains(err.Error(), "no usable v1.5 rollback record") {
				t.Fatalf("v1.5 rollback reached a local snapshot fallback: %v", err)
			}
		})
	}
}

func TestLocalPromoteOutput(t *testing.T) {
	raw, err := (localPromoteRemote{root: t.TempDir()}).Run(context.Background(), "printf '{\"services\":{}}'; printf 'warning' >&2")
	if err != nil || string(raw) != `{"services":{}}` {
		t.Fatalf("stderr corrupted model: %s %v", raw, err)
	}
}

func TestPromoteModelEnvironment(t *testing.T) {
	t.Setenv("PROMOTE_IMAGE_OPERATOR", "example.invalid/operator:fixture")
	got, err := (localPromoteRemote{root: t.TempDir()}).resolveModel(context.Background(), `[ -n "$HOME" ] && [ -z "$PROMOTE_IMAGE_OPERATOR" ] && printf model`)
	if err != nil || string(got) != "model" {
		t.Fatalf("model runner environment: %q, %v", got, err)
	}
}

// skipPromoteShellDoubles skips tests whose docker double is a POSIX shell script
// put on PATH; Windows neither runs shebang scripts nor splits PATH on ':'.
func skipPromoteShellDoubles(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the docker double is a POSIX shell script")
	}
}
