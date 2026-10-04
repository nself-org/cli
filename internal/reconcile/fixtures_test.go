package reconcile

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// update rewrites the golden files: go test ./internal/reconcile -update.
var update = flag.Bool("update", false, "rewrite testdata golden files")

// fullPlan is a finalised plan exercising every contract field. The inputs
// are deliberately unsorted so Finalize's ordering is part of the golden.
func fullPlan(t *testing.T) Plan {
	t.Helper()
	p := Plan{
		Command: CmdBuild,
		Trigger: Trigger{Kind: TriggerConfig, Subject: "BASE_DOMAIN"},
		Env:     "production",
		Artifacts: []Artifact{
			{Kind: KindNginx, Path: "nginx/sites/api.conf", Action: ActionChange, Generated: true, HandEdited: true, DiffLines: 4},
			{Kind: KindCompose, Path: "docker-compose.yml", Action: ActionChange, Generated: true, DiffLines: 2},
			{Kind: KindEnv, Path: ".env.computed", Action: ActionAdd, DiffLines: 9, Redacted: true},
			{Kind: KindLock, Path: "nself.lock.yaml", Action: ActionChange, DiffLines: -1},
		},
		Effects: []Effect{
			{Kind: EffectOrphanRemove, Target: "nginx/sites/old.conf", Detail: "no longer generated"},
			{Kind: EffectHosts, Target: "/etc/hosts", Detail: "add api.example.test"},
		},
		Containers: Containers{Known: true, Items: []ContainerItem{
			{Service: "redis", Action: ContainerRecreate, Stateful: true, AppliedBy: AppliedThisCommand},
			{Service: "nginx", Action: ContainerRecreate, AppliedBy: AppliedNextStart},
			{Service: "hasura", Action: ContainerNone, AppliedBy: AppliedNextStart},
		}},
	}
	if err := p.Finalize(); err != nil {
		t.Fatal(err)
	}
	return p
}

// emptyPlan is a finalised plan with nothing to do.
func emptyPlan(t *testing.T, env string) Plan {
	t.Helper()
	p := Plan{Command: CmdBuild, Trigger: Trigger{Kind: TriggerBuild}, Env: env, Containers: Containers{Known: true}}
	if err := p.Finalize(); err != nil {
		t.Fatal(err)
	}
	return p
}

// golden compares got with testdata/<name>, or rewrites it under -update.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if string(want) != string(got) {
		t.Errorf("%s differs from golden\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}
