package deploy

// Purpose: D-0419, the shipped-list prune also covers plugin compose fragments
//          under .nself/plugins/<slug>/ that a deploy no longer ships.
// Inputs:  a fake remote over a temp dir, a host shipped list, a local record.
// Outputs: assertions on what was removed and what stayed.
// Constraints: a fragment has no generated marker; only record-and-host-listed
//              paths go, and the symlink guard refuses a batch.

import (
	"context"
	"os"
	"testing"
)

func TestPrunePreviousRemovesDroppedPluginFragment(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	dropped := ".nself/plugins/old/docker-compose.plugin.yml"
	kept := ".nself/plugins/kept/docker-compose.plugin.yml"
	unlisted := ".nself/plugins/hand/docker-compose.plugin.yml"
	notRecorded := ".nself/plugins/other/docker-compose.plugin.yml"
	for _, p := range []string{dropped, kept, unlisted, notRecorded} {
		writeFile(t, rp+"/"+p, "services:\n  x:\n    image: x\n", 0o644) // no generated marker
	}
	writeFile(t, rp+"/"+ShippedFile, dropped+"\n"+kept+"\n"+notRecorded+"\n", 0o644)
	removed, err := PrunePrevious(context.Background(), f, rp, map[string]bool{kept: true}, []string{dropped, kept, unlisted})
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != dropped || exists(rp+"/"+dropped) {
		t.Fatalf("removed %v, want only %s", removed, dropped)
	}
	for _, p := range []string{kept, unlisted, notRecorded} {
		if !exists(rp + "/" + p) {
			t.Errorf("%s was removed", p)
		}
	}
}

func TestPrunePreviousPluginFragmentNeedsPluginPath(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	// Unmarked files outside .nself/plugins/<slug>/ keep needing the marker.
	paths := []string{"hasura/metadata.yaml", ".nself/plugins/loose.yml", ".nself/compose-files.txt"}
	list := ""
	for _, p := range paths {
		writeFile(t, rp+"/"+p, "plain\n", 0o644)
		list += p + "\n"
	}
	writeFile(t, rp+"/"+ShippedFile, list, 0o644)
	removed, err := PrunePrevious(context.Background(), f, rp, map[string]bool{}, paths)
	if err != nil || len(removed) != 0 {
		t.Fatalf("removed %v err %v", removed, err)
	}
	for _, p := range paths {
		if !exists(rp + "/" + p) {
			t.Errorf("unmarked %s was removed", p)
		}
	}
}

func TestPrunePreviousPluginFragmentSymlinkGuard(t *testing.T) {
	needTools(t)
	f := newFakeRemote(t)
	rp := t.TempDir()
	outside := t.TempDir()
	victim := outside + "/docker-compose.plugin.yml"
	writeFile(t, victim, "keep me\n", 0o644)
	if err := os.MkdirAll(rp+"/.nself/plugins", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, rp+"/.nself/plugins/old"); err != nil {
		t.Fatal(err)
	}
	rel := ".nself/plugins/old/docker-compose.plugin.yml"
	writeFile(t, rp+"/"+ShippedFile, rel+"\n", 0o644)
	if _, err := PrunePrevious(context.Background(), f, rp, map[string]bool{}, []string{rel}); err == nil {
		t.Fatal("a plugin dir symlinked outside remote_path was pruned through")
	}
	if !exists(victim) {
		t.Fatal("file outside remote_path was deleted")
	}
	if !isPluginFragment(rel) || isPluginFragment(".nself/plugins/old") || isPluginFragment(".nself/plugins//x") || isPluginFragment("nginx/a.conf") {
		t.Error("isPluginFragment misjudged a path")
	}
}
