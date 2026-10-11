//go:build integration

package sim_test

// P7-DEPL-15 simulation drives the promotion engine through the same remote
// factories used by command wiring. Two sshd hosts stand in for local-servers
// and prod; their nself commands are deterministic recording stubs.

import (
	"context"
	"errors"
	"os/exec"
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
  "backup restore "*) cp /tmp/sim-db-backup /tmp/sim-db-fp ;;
esac
`

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
  *"compose images --format json"*) printf '[{"Service":"web","Repository":"example.invalid/web","Tag":"2","ID":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}]' ;;
  *"image inspect --format"*) printf '["example.invalid/web@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]|linux/amd64' ;;`, 1)
		h.put("/usr/local/bin/docker", docker)
		h.sh("chmod 755 /usr/local/bin/nself; printf original > /tmp/sim-db-fp")
		h.put(h.rp+"/docker-compose.yml", "old\n")
		h.own()
		h.scenario(" Container proj-web-1 Recreate\n")
	}
	sourceRemote, targetRemote := source.remote(), target.remote()
	tag := "pre-promote-1791720000-deadbeef"
	recordRoot := t.TempDir()

	run := func(compareErr error) error {
		state := &promote.State{Source: "ls", Target: "prodsim", TargetTier: controlplane.TierProd, Values: map[string]any{}}
		steps := make([]promote.Step, 0, len(promote.StepNames))
		for _, name := range promote.StepNames {
			steps = append(steps, promote.Step{Name: name})
		}
		steps[4] = promote.SourceHealthStep(sourceRemote, source.rp)
		steps[5].Run = func(ctx context.Context, st *promote.State) error {
			runRemote := func(c context.Context, command string) ([]byte, error) {
				return sourceRemote.Run(c, "cd "+source.rp+" && "+command)
			}
			sourceLookup := func(context.Context, string) (map[string]string, error) {
				return map[string]string{"linux/amd64": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, nil
			}
			images, err := promote.CaptureSource(ctx, runRemote, []promote.Image{{Service: "web", Ref: "example.invalid/web:2"}}, sourceLookup)
			st.Values["source_images"] = images
			return err
		}
		steps[6].Run = func(_ context.Context, st *promote.State) error {
			set, _ := stageSet(t, target.rp, "2")
			st.Values["staged_set"] = set
			return nil
		}
		steps[7].Run = func(ctx context.Context, st *promote.State) error {
			digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			if compareErr != nil {
				digest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			}
			lookup := func(context.Context, string) (map[string]string, error) {
				return map[string]string{"linux/amd64": digest}, nil
			}
			rows, err := promote.CompareDigests(ctx, st.Values["source_images"].([]promote.SourceImage),
				[]promote.Image{{Service: "web", Ref: "example.invalid/web:2"}}, "amd64", st.TargetTier, lookup, nil)
			st.Digests = rows
			return err
		}
		steps[8] = promote.SnapshotStep(targetRemote, target.rp, tag)
		steps[9] = promote.SaveStep(targetRemote, target.rp)
		steps[10].Run = func(ctx context.Context, st *promote.State) error {
			set := st.Values["staged_set"].(deploy.ArtifactSet)
			res, err := deploy.ApplyRemote(ctx, deploy.ApplyOptions{Remote: targetRemote, RemotePath: target.rp, Set: set,
				ProdClass: true, Yes: true, BackupID: st.BackupID, Env: "prodsim"})
			if res.Synced {
				st.PreDeployTar = map[string]string{"prodsim": res.PreDeployTar}
				st.PreDeployChanged = map[string][]string{"prodsim": res.Changed}
				st.PreDeployAbsent = map[string][]string{"prodsim": res.Absent}
			}
			return err
		}
		steps[10].Undo = func(ctx context.Context, st *promote.State) error {
			return deploy.RollbackRemote(ctx, deploy.RollbackOptions{Remote: targetRemote, RemotePath: target.rp,
				Tar: st.PreDeployTar["prodsim"], Changed: st.PreDeployChanged["prodsim"], Absent: st.PreDeployAbsent["prodsim"], StrictHasura: true})
		}
		steps[11] = promote.MigrateStep(targetRemote, target.rp)
		steps[12] = promote.HealthStep(targetRemote, target.rp)
		steps[13].Run = func(_ context.Context, st *promote.State) error {
			saved, _ := st.Values["saved_manifest"].(promote.SavedManifest)
			return promote.SaveRecord(recordRoot, promote.Record{Source: st.Source, Target: st.Target,
				Status: "ok", BackupID: st.BackupID, BackupHost: st.BackupHost, SavedManifest: saved})
		}
		_, err := (promote.Engine{Steps: steps}).Run(context.Background(), state)
		return err
	}

	t.Run("full promote", func(t *testing.T) {
		if err := run(nil); err != nil {
			t.Fatal(err)
		}
		if got := target.sh("cat /tmp/sim-db-fp"); got != "migrated" {
			t.Fatalf("database fingerprint = %q", got)
		}
		rec, err := promote.LoadRecord(recordRoot)
		if err != nil || rec == nil || rec.SavedManifest.Host != targetRemote.Label() {
			t.Fatalf("saved target manifest = %#v, %v", rec, err)
		}
	})
	t.Run("health rollback", func(t *testing.T) {
		target.sh("printf original > /tmp/sim-db-fp; printf old > " + target.rp + "/docker-compose.yml; touch /tmp/sim-health-fail")
		err := run(nil)
		var cliErr *errs.CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != "E488" {
			t.Fatalf("error = %v, want E488", err)
		}
		if got := target.sh("cat " + target.rp + "/docker-compose.yml; cat /tmp/sim-db-fp"); !strings.Contains(got, "old") || !strings.Contains(got, "original") {
			t.Fatalf("rollback state = %q", got)
		}
	})
	t.Run("verify and drift stop before apply", func(t *testing.T) {
		targetRemote.log = nil
		target.sh("touch /tmp/sim-verify-fail")
		err := run(nil)
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
		err = run(errs.New("E490", "release drift: web"))
		var driftErr *errs.CLIError
		driftLog := strings.Join(targetRemote.log, "\n")
		if !errors.As(err, &driftErr) || driftErr.Code != "E490" {
			t.Fatalf("drift failure = %v, want E490\n%s", err, driftLog)
		}
		for _, forbidden := range []string{"backup create", "db migrate", "docker compose", "rsync", "tar -czf"} {
			if strings.Contains(driftLog, forbidden) {
				t.Fatalf("drift reached remote write %q:\n%s", forbidden, driftLog)
			}
		}
	})
}
