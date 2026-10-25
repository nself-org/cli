package bundle

// remover_deps_unreadable_test.go — D-0662: an outside plugin whose plugin.json
// cannot be read refuses the plan; an unreadable planned member does not; and
// dependents are matched by directory name, not manifest name.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func breakManifest(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name, "plugin.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveRefusesWhenAnOutsiderManifestIsUnreadable(t *testing.T) {
	dir := chatDir(t)
	breakManifest(t, dir, "outsider")
	before := ledgerBytes(t, dir)
	_, _, rec, err := removeChat(t, dir, nil)
	if err == nil || !strings.Contains(err.Error(), "outsider") || !strings.Contains(err.Error(), "nothing was removed") {
		t.Fatalf("err = %v, want a refusal naming outsider", err)
	}
	if len(rec.calls) != 0 || !isAlreadyInstalled(dir, "px") || string(before) != string(ledgerBytes(t, dir)) {
		t.Errorf("a refused plan changed state; removed %v", rec.names())
	}
}

func TestRemoveIgnoresAnUnreadableManifestOfAPlannedMember(t *testing.T) {
	dir := chatDir(t)
	breakManifest(t, dir, "px")
	_, out, rec, err := removeChat(t, dir, nil)
	if err != nil {
		t.Fatalf("remove: %v\n%s", err, out)
	}
	if !sameStrings(rec.names(), []string{"py", "px"}) {
		t.Errorf("removed %v, want [py px]", rec.names())
	}
}

// A dependent's manifest name equals a planned member, but it lives in another
// directory: it is outside the plan, so the member it needs is kept.
func TestRemoveMatchesDependentsByDirectoryNotManifestName(t *testing.T) {
	dir := chatDir(t)
	setDeps(t, dir, "outsider", "px")
	path := filepath.Join(dir, "outsider", "plugin.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["name"], m["schema"] = "py", "np_py"
	out, _ := json.Marshal(m)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
	_, outp, rec, err := removeChat(t, dir, nil)
	if err != nil {
		t.Fatalf("remove: %v\n%s", err, outp)
	}
	if !sameStrings(rec.names(), []string{"py"}) || !strings.Contains(outp, "kept: px (needed by outsider; recorded explicit)") {
		t.Errorf("removed %v, want only py\n%s", rec.names(), outp)
	}
}
