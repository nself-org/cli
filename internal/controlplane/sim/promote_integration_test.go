//go:build integration

package sim_test

// P7-DEPL-15 simulation drives the promotion engine through the same remote
// factories used by command wiring. Two sshd hosts stand in for local-servers
// and prod; their nself commands are deterministic recording stubs.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
)

const promoteNselfStub = `#!/bin/sh
echo "$*" >> /tmp/sim-nself.log
case "$*" in
  "doctor") if [ -f /tmp/sim-health-fail ]; then rm /tmp/sim-health-fail; exit 1; fi ;;
  "db migrate up") printf migrated > /tmp/sim-db-fp ;;
  "backup create --tag "*) cp /tmp/sim-db-fp /tmp/sim-db-backup; echo "$4" > /tmp/sim-tag ;;
  "backup list --format json") t=$(cat /tmp/sim-tag); printf '[{"id":"sim_%s","tag":"%s","date":"2026-10-11T12:00:00Z"}]' "$t" "$t" ;;
  "backup verify "*) [ -f /tmp/sim-verify-fail ] && exit 1; exit 0 ;;
  "backup restore "*) [ -f /tmp/sim-restore-fail ] && exit 1; cp /tmp/sim-db-backup /tmp/sim-db-fp ;;
esac
`

// TestPromoteSimRestoreFailureFixture proves the restore-failure fixture exits
// unsuccessfully rather than silently copying the backup and hiding E489.
func TestPromoteSimRestoreFailureFixture(t *testing.T) {
	root := t.TempDir()
	stub := strings.ReplaceAll(promoteNselfStub, "/tmp/sim-", root+"/sim-")
	file := filepath.Join(root, "nself")
	for name, data := range map[string]string{"nself": stub, "sim-db-backup": "original", "sim-db-fp": "migrated", "sim-restore-fail": ""} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := exec.Command(file, "backup", "restore", "sim_backup", "--yes").Run(); err == nil {
		t.Fatal("restore failure fixture succeeded; E489 would never be exercised")
	}
	got, err := os.ReadFile(filepath.Join(root, "sim-db-fp"))
	if err != nil || string(got) != "migrated" {
		t.Fatalf("failed restore changed database: %q, %v", got, err)
	}
}

// classifyPromoteCommands treats only the explicit pre-compare read commands as
// reads. Every other command is a write, including newly introduced commands.
func classifyPromoteCommands(source, target *simHost, commands []string) (reads, writes []string) {
	q := remotesdk.ShellQuote
	cd := "cd " + q(source.rp) + " && "
	allowed := map[string]bool{
		cd + "nself doctor": true,
		cd + "docker compose -f " + q(source.rp+"/docker-compose.yml") + " images --format json":                                                                               true,
		cd + "docker image inspect --format '{{json .RepoDigests}}|{{.Os}}/{{.Architecture}}' " + q("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"): true,
		`printf '%s\n' "$HOME"`: true,
		cd + "docker compose -f " + q(source.rp+"/docker-compose.yml") + " ps --all --format json": true,
	}
	for _, name := range []string{"compose-files.txt", "compose-env-files.txt"} {
		manifest := q(source.rp + "/.nself/" + name)
		allowed[cd+"if [ -f "+manifest+" ]; then cat "+manifest+"; else printf '__NSELF_ABSENT__'; fi"] = true
	}
	pluginDir := q(target.home + "/.nself/plugins")
	for _, script := range []string{
		fmt.Sprintf(`grep -E '^[[:space:]]*(NSELF_PLUGIN_DIR|PLUGIN_DIR)=' %s 2>/dev/null || true`, q(target.rp+"/.env")),
		fmt.Sprintf(`[ -d %s ] || exit 0; find %s -maxdepth 2 -name .disabled -print | sed 's/^/@D /'; find %s -maxdepth 2 -name plugin.json -exec sh -c 'for f; do printf "@F %%s\n" "$f"; cat "$f"; printf "\n"; done' sh {} +`, pluginDir, pluginDir, pluginDir),
		fmt.Sprintf(`find -L %s -maxdepth 2 -name '*.pem' -print 2>/dev/null || true`, q(target.rp+"/ssl/certificates")),
	} {
		allowed["sh -c "+q(script)] = true
	}
	for _, command := range commands {
		if allowed[command] {
			reads = append(reads, command)
		} else {
			writes = append(writes, command)
		}
	}
	return reads, writes
}

func TestSimPromote(t *testing.T) {
	source, target := startHost(t), startHost(t)
	// startHost resets HOME for each fleet. Trust both keys again in the final
	// HOME so strict promotion remotes can open either host.
	for _, h := range []*simHost{source, target} {
		spec, err := remotesdk.ParseHostSpec(h.host)
		if err != nil {
			t.Fatal(err)
		}
		scan, err := exec.Command("ssh-keyscan", "-T", "5", "-t", "ed25519", "-p", strconv.Itoa(spec.Port), "--", spec.Host).Output()
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(scan), "\n") {
			if fingerprint, fpErr := remotesdk.Fingerprint(line); fpErr == nil {
				if err := controlplane.TrustHostKey(context.Background(), h.host, fingerprint); err != nil {
					t.Fatal(err)
				}
				break
			}
		}
	}
	for _, h := range []*simHost{source, target} {
		h.put("/usr/local/bin/nself", promoteNselfStub)
		docker := strings.Replace(dockerStub, `case "$*" in`, `case "$*" in
  *" images --format json"*)
    if [ -f /tmp/sim-running-web ]; then
      image=$(cat /tmp/sim-running-web); id=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
      [ "$image" = web:2 ] && id=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
      printf '[{"ContainerName":"proj-web-1","Repository":"%s","Tag":"%s","ID":"sha256:%s"}]' "${image%:*}" "${image##*:}" "$id"
    else printf '[]'; fi ;;
  *" ps --all --format json"*)
    printf '{"Name":"proj-web-1","Service":"web","State":"running"}\n' ;;
  *"image inspect --format"*)
    for last; do :; done
    digest=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    [ "$last" = sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc ] && digest=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    printf '["web@sha256:%s"]|linux/amd64' "$digest" ;;`, 1)
		docker = strings.Replace(docker, `*"up -d --no-deps"*)`, `*"up -d --no-deps"*)
    for last; do :; done
    if [ "$last" = web ]; then
      awk '/^  web:/{web=1; next} web && /^    image:/{print $2; exit}' docker-compose.yml > /tmp/sim-running-web
    fi;`, 1)
		h.put("/usr/local/bin/docker", docker)
		h.sh("chmod 755 /usr/local/bin/nself /usr/local/bin/docker; printf original > /tmp/sim-db-fp; chown nself /tmp/sim-db-fp")
		h.put(h.rp+"/docker-compose.yml", "old\n")
		h.own()
		h.scenario(" Container proj-web-1 Recreate\n")
	}
	source.sh("printf web:2 > /tmp/sim-running-web; chown nself /tmp/sim-running-web")
	sourceRemote, targetRemote := source.remote(), target.remote()
	tag := "pre-promote-1791720000-deadbeef"
	recordRoot := t.TempDir()

	const equalDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	run := func(targetDigest string) error {
		state := &promote.State{Source: "ls", Target: "prodsim", TargetTier: controlplane.TierProd, Values: map[string]any{}}
		state.RecoveryCheckpoint = func(st *promote.State) error {
			return promote.SaveRecord(recordRoot, promote.Record{Source: st.Source, Target: st.Target,
				Status: "in_progress", BackupID: st.BackupID, BackupHost: st.BackupHost,
				RecoveryPhase: st.RecoveryPhase, DBRestoredAt: st.DBRestoredAt, FilesRestored: st.FilesRestored})
		}
		steps := make([]promote.Step, 0, len(promote.StepNames))
		for _, name := range promote.StepNames {
			steps = append(steps, promote.Step{Name: name})
		}
		steps[4] = promote.SourceHealthStep(sourceRemote, source.rp)
		steps[5].Run = func(ctx context.Context, st *promote.State) error {
			runRemote := func(c context.Context, command string) ([]byte, error) {
				return sourceRemote.Run(c, "cd "+remotesdk.ShellQuote(source.rp)+" && "+command)
			}
			capturedRun, _, err := promote.CaptureSourceManifest(ctx, runRemote, source.rp)
			if err != nil {
				return err
			}
			sourceLookup := func(context.Context, string) (map[string]string, error) {
				return map[string]string{"linux/amd64": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, nil
			}
			images, err := promote.CaptureSource(ctx, capturedRun, []promote.Image{{Service: "web", Ref: "web:2"}}, sourceLookup)
			st.Values["source_images"] = images
			return err
		}
		steps[6].Run = func(ctx context.Context, st *promote.State) error {
			set, _ := stageSet(t, target.rp, "2")
			st.Values["staged_set"] = set
			expected, err := deploy.TakeTargetSnapshot(ctx, targetRemote, target.rp)
			st.Values["expected"] = expected
			return err
		}
		steps[7].Run = func(ctx context.Context, st *promote.State) error {
			// Drift is a registry fixture difference consumed by real comparison.
			lookup := func(context.Context, string) (map[string]string, error) {
				return map[string]string{"linux/amd64": targetDigest}, nil
			}
			rows, err := promote.CompareDigests(ctx, st.Values["source_images"].([]promote.SourceImage),
				[]promote.Image{{Service: "web", Ref: "web:2"}}, "amd64", st.TargetTier, lookup, nil)
			st.Digests = rows
			return err
		}
		steps[8] = promote.SnapshotStep(targetRemote, target.rp, tag)
		steps[9] = promote.SaveStep(targetRemote, target.rp)
		steps[10].Run = func(ctx context.Context, st *promote.State) error {
			set := st.Values["staged_set"].(deploy.ArtifactSet)
			ra := &controlplane.RemoteApply{Set: set, Expected: st.Values["expected"].(deploy.TargetSnapshot), ProdClass: true, Yes: true, BackupID: st.BackupID,
				Open: func(context.Context, controlplane.Server, string, string) (deploy.Remote, error) {
					return targetRemote, nil
				},
				Record: func(_ controlplane.Server, res deploy.ApplyResult, _ error) { promote.RecordApplied(st, res) }}
			return controlplane.ApplyServer(ctx, "prodsim", controlplane.Server{Name: "app", RemotePath: target.rp}, "", ra)
		}
		steps[10].Undo = func(ctx context.Context, st *promote.State) error {
			return promote.UndoApplied(ctx, st, map[string]deploy.Remote{targetRemote.Label(): targetRemote}, map[string]string{targetRemote.Label(): target.rp})
		}
		steps[11] = promote.MigrateStep(targetRemote, target.rp)
		steps[12] = promote.HealthStep(targetRemote, target.rp)
		steps[13].Run = func(_ context.Context, st *promote.State) error {
			saved, _ := st.Values["saved_manifest"].(promote.SavedManifest)
			return promote.SaveRecord(recordRoot, promote.Record{Source: st.Source, Target: st.Target,
				Status: "ok", BackupID: st.BackupID, BackupHost: st.BackupHost, SavedManifest: saved, Digests: st.Digests})
		}
		_, err := (promote.Engine{Steps: steps}).Run(context.Background(), state)
		return err
	}

	t.Run("full promote", func(t *testing.T) {
		if err := run(equalDigest); err != nil {
			t.Fatal(err)
		}
		if got := target.sh("cat /tmp/sim-db-fp"); got != "migrated" {
			t.Fatalf("database fingerprint = %q", got)
		}
		rec, err := promote.LoadRecord(recordRoot)
		if err != nil || rec == nil || rec.SavedManifest.Host != targetRemote.Label() {
			t.Fatalf("saved target manifest = %#v, %v", rec, err)
		}
		if len(rec.Digests) != 1 || rec.Digests[0].Source != equalDigest || rec.Digests[0].Target != equalDigest || rec.Digests[0].Status != "equal" {
			t.Fatalf("source and target digest identities differ: %#v", rec.Digests)
		}
		capture := func(host *simHost, remote *recordingRemote) []promote.SourceImage {
			runner := func(ctx context.Context, command string) ([]byte, error) {
				return remote.Run(ctx, "cd "+remotesdk.ShellQuote(host.rp)+" && "+command)
			}
			manifest, _, err := promote.CaptureSourceManifest(context.Background(), runner, host.rp)
			if err != nil {
				t.Fatal(err)
			}
			images, err := promote.CaptureSource(context.Background(), manifest, []promote.Image{{Service: "web", Ref: "web:2"}},
				func(_ context.Context, ref string) (map[string]string, error) {
					return map[string]string{"linux/amd64": strings.Split(ref, "@")[1]}, nil
				})
			if err != nil {
				t.Fatal(err)
			}
			return images
		}
		sourceImages, targetImages := capture(source, sourceRemote), capture(target, targetRemote)
		if len(sourceImages) == 0 || !reflect.DeepEqual(sourceImages, targetImages) {
			t.Fatalf("running target identities = %#v, source = %#v", targetImages, sourceImages)
		}
	})
	t.Run("health rollback", func(t *testing.T) {
		_, err := targetRemote.Run(context.Background(), "printf original > /tmp/sim-db-fp && printf old > "+remotesdk.ShellQuote(target.rp+"/docker-compose.yml")+" && touch /tmp/sim-health-fail")
		if err != nil {
			t.Fatal(err)
		}
		if got := target.sh("cat /tmp/sim-db-fp"); got != "original" {
			t.Fatalf("pre-promote database fingerprint = %q, want original", got)
		}
		beforeCompose, err := targetRemote.Run(context.Background(), "cat "+remotesdk.ShellQuote(target.rp+"/docker-compose.yml"))
		if err != nil {
			t.Fatal(err)
		}
		beforeFP, err := targetRemote.Run(context.Background(), "cat /tmp/sim-db-fp")
		if err != nil {
			t.Fatal(err)
		}
		err = run(equalDigest)
		var cliErr *errs.CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != "E488" {
			t.Fatalf("error = %v, want E488", err)
		}
		afterCompose, err := targetRemote.Run(context.Background(), "cat "+remotesdk.ShellQuote(target.rp+"/docker-compose.yml"))
		if err != nil || string(afterCompose) != string(beforeCompose) {
			t.Fatalf("compose changed: before %q, after %q, %v", beforeCompose, afterCompose, err)
		}
		afterFP, err := targetRemote.Run(context.Background(), "cat /tmp/sim-db-fp")
		if err != nil || string(afterFP) != string(beforeFP) {
			t.Fatalf("database changed: before %q, after %q, %v", beforeFP, afterFP, err)
		}
	})
	t.Run("verify and drift stop before apply", func(t *testing.T) {
		targetRemote.log = nil
		target.sh("touch /tmp/sim-verify-fail")
		err := run(equalDigest)
		var verifyErr *errs.CLIError
		verifyLog := strings.Join(targetRemote.log, "\n")
		if !errors.As(err, &verifyErr) || verifyErr.Code != "E206" {
			t.Fatalf("verify failure = %v, want E206\n%s", err, verifyLog)
		}
		for _, forbidden := range []string{"compose-files.txt", "compose-env-files.txt", "docker compose", "rsync", "tar -czf", "db migrate"} {
			if strings.Contains(verifyLog, forbidden) {
				t.Fatalf("verify failure reached %q:\n%s", forbidden, verifyLog)
			}
		}
		target.sh("rm -f /tmp/sim-verify-fail")
		targetRemote.log = nil
		sourceRemote.log = nil
		err = run("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
		var driftErr *errs.CLIError
		driftLog := strings.Join(targetRemote.log, "\n")
		if !errors.As(err, &driftErr) || driftErr.Code != "E490" {
			t.Fatalf("drift failure = %v, want E490\n%s", err, driftLog)
		}
		reads, writes := classifyPromoteCommands(source, target, append(sourceRemote.log, targetRemote.log...))
		if len(reads) == 0 || len(writes) != 0 {
			t.Fatalf("drift commands: reads = %#v; writes = %#v", reads, writes)
		}
	})
	t.Run("restore failure stops rollback", func(t *testing.T) {
		target.sh("touch /tmp/sim-health-fail /tmp/sim-restore-fail")
		targetRemote.log = nil
		err := run(equalDigest)
		var cliErr *errs.CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != "E489" || !strings.Contains(err.Error(), tag) {
			t.Fatalf("restore failure = %v, want E489 naming %s", err, tag)
		}
		log := targetRemote.log
		if len(log) == 0 || !strings.Contains(log[len(log)-1], "nself backup restore") {
			t.Fatalf("commands continued after failed restore: %#v", log)
		}
	})
}
