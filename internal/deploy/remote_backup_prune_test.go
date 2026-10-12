package deploy

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func tarName(day string) string { return "pre-deploy-202601" + day + "T000000Z-aaaaaa.tar.gz" }

func seedTars(t *testing.T, rp string, days ...string) {
	t.Helper()
	for _, d := range days {
		writeFile(t, filepath.Join(rp, ".nself", "backups", tarName(d)), "tar "+d, 0o600)
	}
}

func listBackups(t *testing.T, rp string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(rp, ".nself", "backups"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestPrunePreDeployTarsKeepsNewestAndProtected(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	seedTars(t, rp, "01", "02", "03", "04", "05", "06", "07", "08")
	b := filepath.Join(rp, ".nself", "backups")
	// Entries that are not pre-deploy tars with a real id are never touched.
	for _, n := range []string{"pre-deploy-bad.tar.gz", tarName("01") + ".bak", "pre-deploy-20260101T000000Z-aaaaaa.tar", "notes.txt", "restore-pre-deploy-20260101T000000Z-aaaaaa/keep", ".list-x", ".empty-x/keep"} {
		writeFile(t, filepath.Join(b, n), "other", 0o600)
	}
	outside := filepath.Join(t.TempDir(), "outside.tar.gz")
	writeFile(t, outside, "must survive", 0o600)
	if err := os.Symlink(outside, filepath.Join(b, "pre-deploy-20250101T000000Z-bbbbbb.tar.gz")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	protected := filepath.Join(b, tarName("01"))
	removed, err := PrunePreDeployTars(context.Background(), f, rp, 5, protected)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(removed)
	if got, want := strings.Join(removed, ","), tarName("02")+","+tarName("03"); got != want {
		t.Errorf("removed %q, want %q (kept: newest 5 = 04..08, plus the protected 01)", got, want)
	}
	for _, d := range []string{"01", "04", "05", "06", "07", "08"} {
		if !exists(filepath.Join(b, tarName(d))) {
			t.Errorf("tar %s was removed", d)
		}
	}
	for _, n := range []string{"pre-deploy-bad.tar.gz", tarName("01") + ".bak", "pre-deploy-20260101T000000Z-aaaaaa.tar", "notes.txt", "restore-pre-deploy-20260101T000000Z-aaaaaa", ".list-x", ".empty-x", "pre-deploy-20250101T000000Z-bbbbbb.tar.gz"} {
		if !exists(filepath.Join(b, n)) {
			t.Errorf("%s was touched", n)
		}
	}
	if readFile(t, outside) != "must survive" {
		t.Error("a symlink target was changed")
	}
	// A second run has nothing left to remove.
	if again, err := PrunePreDeployTars(context.Background(), f, rp, 5, protected); err != nil || len(again) != 0 {
		t.Errorf("second prune = %v, %v", again, err)
	}
	// Fewer tars than keep: nothing happens.
	rp2 := t.TempDir()
	seedTars(t, rp2, "01", "02")
	if r2, err := PrunePreDeployTars(context.Background(), f, rp2, 5); err != nil || len(r2) != 0 || len(listBackups(t, rp2)) != 2 {
		t.Errorf("under the limit: %v %v", r2, err)
	}
}

func TestPrunePreDeployTarsRefusals(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	seedTars(t, rp, "01", "02", "03")
	for _, keep := range []int{0, -1} {
		if _, err := PrunePreDeployTars(context.Background(), f, rp, keep); err == nil {
			t.Errorf("keep=%d was accepted", keep)
		}
	}
	if len(listBackups(t, rp)) != 3 || len(f.commands()) != 0 {
		t.Errorf("a refused call must not reach the host: %v", f.commands())
	}

	// No backups dir at all: nothing to do.
	if got, err := PrunePreDeployTars(context.Background(), f, t.TempDir(), 5); err != nil || len(got) != 0 {
		t.Errorf("missing backups dir = %v, %v", got, err)
	}

	// A backups dir that resolves outside remote_path is refused and nothing is removed.
	rp2 := t.TempDir()
	out := t.TempDir()
	for _, d := range []string{"01", "02", "03", "04", "05", "06", "07"} {
		writeFile(t, filepath.Join(out, tarName(d)), "outside", 0o600)
	}
	if err := os.MkdirAll(filepath.Join(rp2, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, filepath.Join(rp2, ".nself", "backups")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	removed, err := PrunePreDeployTars(context.Background(), f, rp2, 2)
	if err == nil || !strings.Contains(err.Error(), "outside") || len(removed) != 0 {
		t.Fatalf("outside backups dir: removed %v err %v", removed, err)
	}
	if entries, _ := os.ReadDir(out); len(entries) != 7 {
		t.Errorf("tars outside remote_path were removed: %d left", len(entries))
	}

	// A backups dir that is a symlink to a directory inside remote_path is fine.
	rp3 := t.TempDir()
	real := filepath.Join(rp3, "elsewhere")
	for _, d := range []string{"01", "02", "03"} {
		writeFile(t, filepath.Join(real, tarName(d)), "inside", 0o600)
	}
	if err := os.MkdirAll(filepath.Join(rp3, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(rp3, ".nself", "backups")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got, err := PrunePreDeployTars(context.Background(), f, rp3, 2); err != nil || len(got) != 1 || got[0] != tarName("01") {
		t.Errorf("symlinked backups dir inside remote_path: %v %v", got, err)
	}
}

func TestApplyPrunesOldTarsOnSuccessOnly(t *testing.T) {
	seed := func(rp string) {
		seedTars(t, rp, "01", "02", "03", "04", "05", "06", "07")
		writeFile(t, filepath.Join(rp, ".nself", "backups", "restore-keep", "x"), "x", 0o600)
	}
	f, o, rp := applyFixture(t, "")
	seed(rp)
	var out bytes.Buffer
	o.Out = &out
	res, err := ApplyRemote(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if !exists(res.PreDeployTar) {
		t.Fatal("this deploy's own tar was pruned")
	}
	// The new tar and the four newest old ones stay: 5 in all.
	var tars []string
	for _, n := range listBackups(t, rp) {
		if strings.HasPrefix(n, "pre-deploy-") {
			tars = append(tars, n)
		}
	}
	if len(tars) != PreDeployTarKeep || contains(tars, tarName("01")) || !contains(tars, tarName("07")) {
		t.Errorf("tars after a good deploy: %v", tars)
	}
	for _, d := range []string{"01", "02", "03"} {
		if !strings.Contains(out.String(), "  [pruned] "+tarName(d)+" (old pre-deploy tar)") {
			t.Errorf("no [pruned] line for %s:\n%s", d, out.String())
		}
	}
	if !exists(filepath.Join(rp, ".nself", "backups", "restore-keep", "x")) {
		t.Error("a restore-* dir was touched")
	}
	hasura, listed := -1, -1
	for i, c := range f.commands() {
		switch {
		case strings.Contains(c, "hasura metadata apply"):
			hasura = i
		case strings.Contains(c, "pre-deploy-*.tar.gz"):
			listed = i
		}
	}
	if hasura < 0 || listed < hasura {
		t.Errorf("the prune must run after the hasura step (hasura at %d, prune listing at %d)", hasura, listed)
	}

	// A failure anywhere keeps every tar, including after the files were put back.
	for name, sub := range map[string]string{"pull": " pull", "second up": "up -d --no-deps -- 'web'", "hasura": "hasura metadata apply"} {
		t.Run(name, func(t *testing.T) {
			f, o, rp := applyFixture(t, "")
			seed(rp)
			var out bytes.Buffer
			o.Out = &out
			f.fail[sub] = errBoom
			if _, err := ApplyRemote(context.Background(), o); err == nil {
				t.Fatal("the failure was swallowed")
			}
			for _, d := range []string{"01", "02", "03", "04", "05", "06", "07"} {
				if !exists(filepath.Join(rp, ".nself", "backups", tarName(d))) {
					t.Errorf("tar %s removed after a failed deploy", d)
				}
			}
			if strings.Contains(out.String(), "[pruned] pre-deploy-") {
				t.Errorf("pruned after a failure:\n%s", out.String())
			}
		})
	}
}

func TestApplyPruneErrorIsOnlyAWarning(t *testing.T) {
	f, o, rp := applyFixture(t, "")
	seedTars(t, rp, "01", "02", "03", "04", "05", "06", "07")
	var out bytes.Buffer
	o.Out = &out
	f.fail["pre-deploy-*.tar.gz"] = errBoom
	res, err := ApplyRemote(context.Background(), o)
	if err != nil {
		t.Fatalf("a prune failure failed the deploy: %v", err)
	}
	if !res.Synced || !res.Touched {
		t.Errorf("result %+v", res)
	}
	if !strings.Contains(out.String(), "[warn] could not prune old pre-deploy tars on fake-host") {
		t.Errorf("no warning:\n%s", out.String())
	}
	if !exists(filepath.Join(rp, ".nself", "backups", tarName("01"))) {
		t.Error("a tar was removed although the listing failed")
	}
}
