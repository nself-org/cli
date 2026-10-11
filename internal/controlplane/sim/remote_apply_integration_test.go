//go:build integration

package sim_test

// P7-DEPL-24 simulation: a real sshd host (sdk/go/simharness openssh image) receives
// the v1.5 artifact set through deploy.ApplyRemote. The host has no docker daemon and
// no nself: recording stubs for both are installed on it (lead ruling 5), and the
// shipped tree is mirrored at the same absolute path on the test host, where the real
// `docker compose config` resolves it. The throwaway nginx -t runs for real on the
// test host in TestSimRemoteNginxInvalid. Requires INTEGRATION=1 and Docker.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/cli/sdk/go/v2/simharness"
)

const dockerStub = `#!/bin/sh
echo "$*" >> /tmp/sim-docker.log
case "$*" in
  *"config --format json"*) cat /tmp/sim-model.json ;;
  *"config --services"*) cat /tmp/sim-services.txt ;;
  *"up -d --dry-run"*) cat /tmp/sim-dryrun.txt ;;
  *"ps -a --format json"*)
    for last; do :; done
    if [ -f "/tmp/sim-ps-$last.json" ]; then cat "/tmp/sim-ps-$last.json"; else
      h=''; [ "$last" = db ] && h=healthy
      printf '[{"ID":"id-%s","Name":"proj-%s-1","State":"running","Health":"%s"}]' "$last" "$last" "$h"
    fi ;;
  *"up -d --no-deps"*) for last; do :; done; n=$(cat "/tmp/sim-up-$last" 2>/dev/null || echo 0); echo $((n+1)) > "/tmp/sim-up-$last" ;;
  "inspect -f"*) shift 3; for i in "$@"; do n=$(cat "/tmp/sim-up-${i#id-}" 2>/dev/null || echo 0); printf '2026-10-10T00:00:%02dZ\n' "$n"; done ;;
  "network inspect"*) exit 1 ;;
esac
exit 0
`

const nselfStub = `#!/bin/sh
echo "$*" >> /tmp/sim-nself.log
case "$*" in
  "backup create --tag "*) echo "$4" > /tmp/sim-tag ;;
  "backup list"*) t=$(cat /tmp/sim-tag); printf '[{"id":"sim_full_20261010_120000_%s","tag":"120000_%s","date":"2026-10-10T12:00:00Z"}]' "$t" "$t" ;;
  "backup verify"*) [ -f /tmp/sim-verify-fail ] && { echo "verify failed" >&2; exit 1; } ;;
esac
exit 0
`

var tagRe = regexp.MustCompile(`[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}`)

type simHost struct {
	fleet *simharness.Fleet
	node  *simharness.Node
	home  string // the login user's home (the image sets it, not /home/nself)
	rp    string // remote project dir, an absolute path that also exists on the test host
	host  string
	t     *testing.T
}

// startHost starts one sshd host with rsync, the docker and nself stubs, and trusts its key.
func startHost(t *testing.T) *simHost {
	t.Helper()
	skipUnlessIntegration(t)
	if out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output(); err == nil {
		t.Setenv("DOCKER_HOST", strings.TrimSpace(string(out)))
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NSELF_V15", "1")
	fleet := simharness.Start(t, simharness.Config{Nodes: []simharness.NodeSpec{{Name: "app"}}})
	t.Cleanup(fleet.Close)
	h := &simHost{fleet: fleet, node: fleet.Node(t, "app"), t: t}
	h.rp = "/tmp/nself-sim-" + strings.ToLower(strings.ReplaceAll(filepath.Base(t.TempDir()), "_", "-")) + "/proj"
	h.host = "nself@" + h.node.Addr()
	h.home = strings.TrimSpace(fleet.Exec(t, "app", "sh", "-c", "getent passwd nself | cut -d: -f6"))
	fleet.Exec(t, "app", "sh", "-c", "command -v rsync >/dev/null || apk add --no-cache rsync >/dev/null")
	for name, body := range map[string]string{"docker": dockerStub, "nself": nselfStub} {
		h.put("/usr/local/bin/"+name, body)
		fleet.Exec(t, "app", "chmod", "755", "/usr/local/bin/"+name)
	}
	fleet.Exec(t, "app", "sh", "-c", "mkdir -p "+h.rp+" && chown -R nself "+filepath.Dir(h.rp)+" && chmod 1777 /tmp")
	spec, _ := remote.ParseHostSpec(h.host)
	scan, err := exec.Command("ssh-keyscan", "-T", "5", "-t", "ed25519", "-p", strconv.Itoa(spec.Port), "--", spec.Host).Output()
	if err != nil {
		t.Fatal(err)
	}
	fp := ""
	for _, line := range strings.Split(string(scan), "\n") {
		if got, err := remote.Fingerprint(line); err == nil {
			fp = got
		}
	}
	if err := controlplane.TrustHostKey(context.Background(), h.host, fp); err != nil {
		t.Fatal(err)
	}
	return h
}

// put writes body to path on the host.
func (h *simHost) put(path, body string) {
	h.t.Helper()
	f := filepath.Join(h.t.TempDir(), "f")
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		h.t.Fatal(err)
	}
	h.fleet.Exec(h.t, "app", "mkdir", "-p", filepath.Dir(path))
	h.fleet.CopyTo(h.t, "app", f, path)
}

// own gives the login user the project tree the test seeded as root.
func (h *simHost) own() { h.sh("chown -R nself " + filepath.Dir(h.rp)) }

// dockerLog is the recorded docker stub log ("" when docker was never called).
func (h *simHost) dockerLog() string {
	return h.sh("if [ -f /tmp/sim-docker.log ]; then cat /tmp/sim-docker.log; fi")
}

func (h *simHost) sh(script string) string {
	h.t.Helper()
	return strings.TrimSpace(h.fleet.Exec(h.t, "app", "sh", "-c", script))
}

func (h *simHost) remote() *recordingRemote {
	r, err := deploy.NewSSHRemote(context.Background(), h.host, h.fleet.KeyPath(), "prod", "sim-app")
	if err != nil {
		h.t.Fatal(err)
	}
	return &recordingRemote{Remote: r}
}

// recordingRemote records every remote command.
type recordingRemote struct {
	deploy.Remote
	log []string
}

func (r *recordingRemote) Run(ctx context.Context, c string) ([]byte, error) {
	r.log = append(r.log, c)
	return r.Remote.Run(ctx, c)
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// stageSet builds an artifact set for host path rp from a scratch project that holds
// local paths everywhere, the way a build leaves them. It also returns those local
// roots (scratch, operator, plugin dir, home): the only paths that must not survive.
// The remote path itself may sit under os.TempDir (it does on Linux runners).
func stageSet(t *testing.T, rp, version string) (deploy.ArtifactSet, []string) {
	t.Helper()
	base := t.TempDir()
	operator, scratch := filepath.Join(base, "operator"), filepath.Join(base, "operator", ".nself", "deploy-build", "prod")
	plugins := filepath.Join(base, "home", ".nself", "plugins")
	w := func(p, body string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w(filepath.Join(scratch, "docker-compose.yml"), "name: proj\nservices:\n  web:\n    image: web:"+version+"\n    env_file: "+scratch+"/.env\n    volumes:\n      - "+scratch+"/nginx:/etc/nginx/sites:ro\n  db:\n    image: postgres:16\n    volumes:\n      - pgdata:/var/lib/postgresql/data\nvolumes:\n  pgdata: {}\n")
	w(filepath.Join(plugins, "ai", "docker-compose.plugin.yml"), "services:\n  ai:\n    image: ai:"+version+"\n    environment:\n      DIR: "+plugins+"/ai\n")
	w(filepath.Join(scratch, ".env"), "PROJECT_NAME=proj\nNSELF_PLUGIN_DIR="+plugins+"\n")
	w(filepath.Join(scratch, ".nself", "compose.env"), "NSELF_PLUGIN_DIR="+plugins+"\n")
	w(filepath.Join(scratch, "nginx", "sites", "api.conf"), "# Generated by nself build - DO NOT EDIT MANUALLY\nssl_certificate /etc/nginx/ssl/certificates/api-example-com/fullchain.pem;\nssl_trusted_certificate /etc/nginx/ssl/certificates/api-example-com/chain.pem;\n# "+version+"\n")
	w(filepath.Join(scratch, "nginx", "conf.d", "default.conf"), "# Generated by nself build - the shipped set must not carry conf.d\n")
	w(filepath.Join(scratch, "ssl", "certificates", "api-example-com", "fullchain.pem"), "STUB")
	w(filepath.Join(scratch, "hasura", "metadata", "version.yaml"), "version: 3\n")
	w(filepath.Join(scratch, ".env.prod"), "ENV=prod\n")
	set, err := deploy.Stage(deploy.StageInput{
		Map:      deploy.PathMap{ScratchRoot: scratch, OperatorRoot: operator, PluginDir: plugins, Home: filepath.Join(base, "home"), RemotePath: rp},
		StageDir: filepath.Join(base, "stage"), Compose: []string{filepath.Join(scratch, "docker-compose.yml"), filepath.Join(plugins, "ai", "docker-compose.plugin.yml")},
		EnvFiles: []string{filepath.Join(scratch, ".env"), filepath.Join(scratch, ".nself", "compose.env")}, EnvSnapshot: filepath.Join(scratch, ".env.prod"), EnvName: "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	return set, []string{scratch, operator, plugins, filepath.Join(base, "home")}
}

func (h *simHost) scenario(dryRun string) {
	// init is a one-shot (restart no), web, ai and nginx have no healthcheck, db has one.
	h.put("/tmp/sim-ps-init.json", `[{"ID":"id-init","Name":"proj-init-1","State":"exited","ExitCode":0}]`)
	h.put("/tmp/sim-model.json", `{"name":"proj","services":{"init":{"image":"i:1","restart":"no"},"nginx":{"image":"nginx:1"},"web":{"image":"web:1","restart":"always"},"ai":{"image":"ai:1","restart":"always"},"db":{"image":"postgres:16","restart":"unless-stopped","healthcheck":{"test":["CMD","pg_isready"]},"volumes":[{"type":"volume","source":"pgdata","target":"/var/lib/postgresql/data"}]}}}`)
	h.put("/tmp/sim-services.txt", "init\ndb\nweb\nai\nnginx\n")
	h.put("/tmp/sim-dryrun.txt", dryRun)
}

func (h *simHost) options(set deploy.ArtifactSet, r deploy.Remote, yes bool) deploy.ApplyOptions {
	return deploy.ApplyOptions{
		Remote: r, RemotePath: h.rp, Set: set, ProdClass: true, Yes: yes, StrictHasura: true,
		Snapshot: func(ctx context.Context, id string) (string, error) {
			return controlplane.VerifiedSnapshot(ctx, r, h.rp, id)
		},
	}
}

func TestSimRemoteApplyAndRollback(t *testing.T) {
	h := startHost(t)
	h.scenario(" Container proj-web-1 Recreate\n Container proj-db-1 Running\n")
	// What the host already runs: a foreign site conf, a conf.d block, a generated conf
	// the new set drops, the target's own TLS dir with a chain, and a deployed v1 tree.
	h.sh("mkdir -p " + h.rp + "/nginx/sites " + h.rp + "/nginx/conf.d " + h.rp + "/ssl/certificates/api-example-com " + h.rp + "/ssl/certificates/target-only-example-org")
	h.put(h.rp+"/nginx/sites/foreign.conf", "# hand written, fronted\nserver { listen 8081; }\n")
	h.put(h.rp+"/nginx/conf.d/block.conf", "# Generated by nself build - but under conf.d, so never touched\n")
	h.put(h.rp+"/nginx/sites/dropped.conf", "# Generated by nself build - DO NOT EDIT MANUALLY\n")
	// Another nself project's generated conf in the same dir: marker-carrying, never shipped here.
	h.put(h.rp+"/nginx/sites/other-project.conf", "# Generated by nself build - DO NOT EDIT MANUALLY\n# another project\n")
	h.put(h.rp+"/.nself/deploy-shipped.txt", "nginx/sites/dropped.conf\n")
	// A same-size rewrite with the same whole-second mtime: only a checksum restore sees it.
	h.put(h.rp+"/nginx/sites/same.conf", "# AAAA\n")
	h.sh("touch -t 202001010000.00 " + h.rp + "/nginx/sites/same.conf")
	h.put(h.rp+"/docker-compose.yml", "# v0\n")
	h.put(h.rp+"/.env", "HOST_ONLY=1\nNSELF_PLUGIN_DIR=/srv/host-plugins\n")
	for _, n := range []string{"fullchain.pem", "privkey.pem", "chain.pem"} {
		h.put(h.rp+"/ssl/certificates/api-example-com/"+n, "PEM "+n+"\n")
	}
	h.put(h.rp+"/ssl/certificates/target-only-example-org/fullchain.pem", "PEM\n")
	h.own()
	foreign := h.sh("sha256sum " + h.rp + "/nginx/sites/foreign.conf " + h.rp + "/nginx/conf.d/block.conf")
	inode := h.sh("stat -c %i " + h.rp + "/nginx")

	ctx := context.Background()
	// 1. Unconfirmed stateful recreate on a prod-class target: E403, nothing applied.
	h.scenario(" Container proj-db-1 Recreate\n")
	set, local := stageSet(t, h.rp, "1")
	r := h.remote()
	_, err := deploy.ApplyRemote(ctx, h.options(set, r, false))
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E403" || !strings.Contains(err.Error(), "db") {
		t.Fatalf("unconfirmed stateful recreate: %v, want E403 naming db", err)
	}
	if log := h.dockerLog(); strings.Contains(log, "--no-deps") {
		t.Fatalf("E403 left an apply behind:\n%s", log)
	}
	if got := h.sh("cat " + h.rp + "/docker-compose.yml"); got != "# v0" {
		t.Fatalf("the refused deploy left new files on the host: %q", got)
	}
	// F5: what the refused deploy created is gone again (the host had no manifests).
	for _, created := range []string{"/.nself/compose-files.txt", "/.nself/compose.env", "/.env.prod", "/.nself/plugins/ai/docker-compose.plugin.yml", "/nginx/sites/api.conf"} {
		if h.sh("test -e "+h.rp+created+" && echo left || echo gone") != "gone" {
			t.Errorf("%s was left behind by the refused deploy", created)
		}
	}
	if h.sh("test -e "+h.rp+"/.nself/deploy.lock && echo held || echo free") != "free" {
		t.Error("the deploy lock was left behind")
	}

	// 2. A failing backup verify on tier prod: E206 before any apply.
	h.scenario(" Container proj-web-1 Recreate\n")
	h.put("/tmp/sim-verify-fail", "1")
	h.sh("rm -f /tmp/sim-docker.log")
	if _, err := deploy.ApplyRemote(ctx, h.options(set, h.remote(), false)); !errors.As(err, &ce) || ce.Code != "E206" {
		t.Fatalf("failing verify: %v, want E206", err)
	}
	// F2: nothing but nself ran before E206: no compose command, and no file was synced.
	if log := h.dockerLog(); log != "" {
		t.Fatalf("E206 came after docker commands:\n%s", log)
	}
	if got := h.sh("cat " + h.rp + "/docker-compose.yml"); got != "# v0" {
		t.Fatalf("files were synced before the backup was verified: %q", got)
	}
	h.sh("rm -f /tmp/sim-verify-fail /tmp/sim-docker.log /tmp/sim-nself.log /tmp/sim-up-*")

	// 3. The deploy: ordered, gated, hasura applied, backup recorded.
	res, err := deploy.ApplyRemote(ctx, h.options(set, h.remote(), false))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.BackupID, "sim_full_20261010_120000_pre-deploy-") || !tagRe.MatchString(res.PreDeployTar) {
		t.Fatalf("result %+v", res)
	}
	if h.sh("test -s "+res.PreDeployTar+" && echo yes") != "yes" {
		t.Fatal("the pre-deploy tar is missing")
	}
	log := h.dockerLog()
	prev := -1
	for _, want := range []string{"config --format json", " pull", "up -d --dry-run", "up -d --no-deps -- init", "up -d --no-deps -- db", "up -d --no-deps -- web", "up -d --no-deps -- ai", "up -d --no-deps -- nginx", "exec -T -- nginx nginx -s reload"} {
		i := strings.Index(log, want)
		if i <= prev {
			t.Fatalf("%q out of order in:\n%s", want, log)
		}
		prev = i
	}
	if nl := h.sh("cat /tmp/sim-nself.log"); !strings.Contains(nl, "backup verify") || !strings.Contains(nl, "db hasura metadata apply") {
		t.Fatalf("nself calls:\n%s", nl)
	}
	for _, rel := range append(append([]string{}, set.Files...), set.EnvFiles...) {
		want, _ := os.ReadFile(filepath.Join(set.StageDir, rel))
		if got := h.sh("sha256sum " + h.rp + "/" + rel + " | cut -d' ' -f1"); got != sha(want) {
			t.Errorf("%s checksum differs on the host", rel)
		}
	}
	for _, rel := range set.EnvFiles {
		if mode := h.sh("stat -c %a " + h.rp + "/" + rel); mode != "600" {
			t.Errorf("env file %s mode %s, want 600", rel, mode)
		}
	}
	if got := h.sh("cat " + h.rp + "/.env"); got != "HOST_ONLY=1\nNSELF_PLUGIN_DIR=/srv/host-plugins" {
		t.Errorf("the host's own .env was replaced by the operator's: %q", got)
	}
	if h.sh("test -e "+h.rp+"/nginx/sites/dropped.conf && echo kept || echo gone") != "gone" {
		t.Error("the dropped generated conf was not removed")
	}
	if h.sh("test -e "+h.rp+"/nginx/sites/other-project.conf && echo kept || echo gone") != "kept" {
		t.Error("a marker-carrying conf this project never shipped was pruned")
	}
	h.put(h.rp+"/nginx/sites/same.conf", "# BBBB\n") // same size as before the deploy
	h.sh("touch -t 202001010000.00 " + h.rp + "/nginx/sites/same.conf")
	if got := h.sh("sha256sum " + h.rp + "/nginx/sites/foreign.conf " + h.rp + "/nginx/conf.d/block.conf"); got != foreign {
		t.Errorf("foreign conf or conf.d block changed:\n%s\nwas:\n%s", got, foreign)
	}
	if h.sh("ls "+h.rp+"/ssl/certificates") != "api-example-com\ntarget-only-example-org" {
		t.Error("the deploy touched the target's TLS dirs")
	}
	if shipped := h.sh("cat " + h.rp + "/nginx/sites/api.conf"); !strings.Contains(shipped, "certificates/api-example-com/chain.pem") {
		t.Errorf("shipped conf does not reference the target's TLS dir and chain:\n%s", shipped)
	}
	for _, f := range []string{"/docker-compose.yml", "/.nself/compose-files.txt", "/.nself/compose.env", "/.nself/plugins/ai/docker-compose.plugin.yml"} {
		body := h.sh("cat " + h.rp + f)
		for _, root := range local {
			if strings.Contains(body, root) {
				t.Errorf("%s names the operator-local path %s:\n%s", f, root, body)
			}
		}
	}
	mirrorResolves(t, h, local)

	// 4. Rollback restores in place: files, foreign conf, conf.d, the bind-mounted dir's inode.
	rb := h.remote()
	if err := deploy.RollbackRemote(ctx, deploy.RollbackOptions{Remote: rb, RemotePath: h.rp, Tar: res.PreDeployTar, BackupID: res.BackupID}); err != nil {
		t.Fatal(err)
	}
	if got := h.sh("cat " + h.rp + "/docker-compose.yml"); got != "# v0" {
		t.Errorf("compose not restored: %q", got)
	}
	if got := h.sh("sha256sum " + h.rp + "/nginx/sites/foreign.conf " + h.rp + "/nginx/conf.d/block.conf"); got != foreign {
		t.Errorf("rollback changed a foreign conf or conf.d block")
	}
	if got := h.sh("cat " + h.rp + "/nginx/sites/same.conf"); got != "# AAAA" {
		t.Errorf("a same-size, same-mtime rewrite was not restored: %q", got)
	}
	if h.sh("test -e "+h.rp+"/.nself/compose-files.txt && echo left || echo gone") != "gone" {
		t.Error("the manifest the deploy created survived the rollback")
	}
	if !strings.Contains(h.dockerLog()[strings.LastIndex(h.dockerLog(), "up -d --no-deps -- init"):], "nginx -s reload") {
		t.Error("nginx was not reloaded after the rollback re-apply")
	}
	if h.sh("stat -c %i "+h.rp+"/nginx") != inode {
		t.Error("the nginx/ directory was replaced: its inode changed")
	}
	if !strings.Contains(h.sh("cat /tmp/sim-nself.log"), "backup restore "+res.BackupID+" --yes") {
		t.Error("the verified database backup was not restored")
	}
}

// mirrorResolves copies the shipped tree to the same absolute path on the test host and
// resolves it with the real docker compose; no operator-local path may appear.
func mirrorResolves(t *testing.T, h *simHost, local []string) {
	t.Helper()
	root := filepath.Dir(h.rp)
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if out, err := exec.Command("docker", "cp", h.node.Container+":"+root, filepath.Dir(root)).CombinedOutput(); err != nil {
		t.Fatalf("docker cp: %v\n%s", err, out)
	}
	compose := []string{"docker", "compose"}
	if exec.Command("docker", "compose", "version").Run() != nil {
		compose = []string{"docker-compose"}
	}
	list := func(f string) []string {
		b, err := os.ReadFile(filepath.Join(h.rp, ".nself", f))
		if err != nil {
			t.Fatal(err)
		}
		return strings.Fields(string(b))
	}
	args := append([]string{}, compose[1:]...)
	for _, f := range list("compose-files.txt") {
		args = append(args, "-f", f)
	}
	for _, e := range list("compose-env-files.txt") {
		args = append(args, "--env-file", e)
	}
	cmd := exec.Command(compose[0], append(args, "config", "--format", "json")...)
	cmd.Dir = h.rp
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config on the mirrored tree: %v\n%s", err, out)
	}
	for _, root := range local {
		if strings.Contains(string(out), root) {
			t.Errorf("resolved model names the operator-local path %s:\n%s", root, out)
		}
	}
	if !strings.Contains(string(out), h.rp+"/nginx") {
		t.Errorf("resolved model does not use the remote layout:\n%s", out)
	}
}

func TestSimRemoteApplyDriftAndSnapshot(t *testing.T) {
	h := startHost(t)
	h.scenario("")
	h.put(h.home+"/.nself/plugins/ai/plugin.json", `{"name":"ai","version":"1.0.0"}`)
	h.put(h.home+"/.nself/cache/registry.json", `{"cached":true}`)
	h.put(h.rp+"/docker-compose.yml", "# v0\n")
	h.sh("chown -R nself " + h.home + "/.nself")
	treeSum := func() string {
		return h.sh("find " + h.home + "/.nself " + h.rp + " -type f -exec sha256sum {} + | sort")
	}
	before := treeSum()
	r := h.remote()
	snap, err := deploy.TakeTargetSnapshot(context.Background(), r, h.rp)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Plugins) != 1 || snap.Plugins[0].Name != "ai" || snap.Plugins[0].Version != "1.0.0" {
		t.Fatalf("snapshot %+v", snap)
	}
	if after := treeSum(); after != before {
		t.Fatalf("the snapshot changed files on the host:\n%s\nvs\n%s", before, after)
	}
	for _, c := range r.log {
		if regexp.MustCompile(`(^|[;&|(\s])nself(\s|$)`).MatchString(c) {
			t.Fatalf("the snapshot ran nself: %q", c)
		}
	}
	if h.sh("test -e /tmp/sim-nself.log && echo ran || echo clean") != "clean" {
		t.Error("the nself stub was called during the snapshot")
	}

	// A local plugin set that differs refuses before any sync, naming the difference.
	set, _ := stageSet(t, h.rp, "1")
	o := h.options(set, h.remote(), true)
	o.LocalPlugins = []deploy.PluginRef{{Name: "ai", Version: "2.0.0"}}
	_, err = deploy.ApplyRemote(context.Background(), o)
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E485" || !strings.Contains(err.Error(), "target drift") || !strings.Contains(err.Error(), "ai is 2.0.0 locally but 1.0.0") {
		t.Fatalf("drift: %v", err)
	}
	if got := h.sh("cat " + h.rp + "/docker-compose.yml"); got != "# v0" {
		t.Errorf("drift refusal came after a sync: %q", got)
	}
	if h.sh("ls "+h.rp+"/.nself 2>/dev/null | wc -l") != "0" {
		t.Error("a pre-deploy tar or other file was written before the drift refusal")
	}
}

// localRemote runs the scripts on the test host, so the real docker runs nginx -t.
type localRemote struct{}

func (localRemote) Label() string { return "test host" }
func (localRemote) Rsync(context.Context, []string, string, string, string) error {
	return errors.New("no rsync")
}
func (localRemote) Run(ctx context.Context, c string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", c)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, out)
	}
	return out, nil
}

func TestSimRemoteNginxInvalid(t *testing.T) {
	skipUnlessIntegration(t)
	rp := t.TempDir()
	conf := func(body string) {
		if err := os.MkdirAll(filepath.Join(rp, "nginx", "sites"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rp, "nginx", "nginx.conf"), []byte("events {}\nhttp {\n  include /etc/nginx/sites/*.conf;\n}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rp, "nginx", "sites", "a.conf"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	model := deploy.ComposeModel{Services: map[string]deploy.ModelService{"hasura": {Image: "hasura:1"}, "nginx": {Image: "nginx:1.27-alpine", Mounts: []deploy.Mount{
		{Type: "bind", Source: rp + "/nginx/nginx.conf", Target: "/etc/nginx/nginx.conf", ReadOnly: true},
		{Type: "bind", Source: rp + "/nginx/sites", Target: "/etc/nginx/sites", ReadOnly: true},
	}}}}
	var out strings.Builder
	conf("upstream gql { server hasura:8080; }\nserver { listen 8080; location / { proxy_pass http://hasura:8080; } location /g { proxy_pass http://gql; } }\n")
	if err := deploy.CheckNginx(context.Background(), localRemote{}, rp, model, &out); err != nil {
		t.Fatalf("a valid shipped config failed nginx -t: %v", err)
	}
	conf("server { listen 8080; no_such_directive on; }\n")
	err := deploy.CheckNginx(context.Background(), localRemote{}, rp, model, &out)
	if err == nil || !strings.Contains(err.Error(), "nothing was reloaded or started") || !strings.Contains(err.Error(), "no_such_directive") {
		t.Fatalf("an invalid shipped config must fail nginx -t with nginx's words, got %v", err)
	}
}
