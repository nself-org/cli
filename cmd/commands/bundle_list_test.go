package commands

// Tests for the additive `ledger` field of `nself bundle list --json`
// (P7-PLUG-18).

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle/ledger"
)

type ledgerRow struct {
	Slug   string `json:"slug"`
	Ledger struct {
		Installed   bool   `json:"installed"`
		InstalledAt string `json:"installed_at"`
		Plugins     []struct {
			Slug        string   `json:"slug"`
			InstalledBy []string `json:"installed_by"`
			Explicit    bool     `json:"explicit"`
			Tier        string   `json:"tier"`
			Version     string   `json:"version"`
		} `json:"plugins"`
	} `json:"ledger"`
}

func ledgerProject(t *testing.T, plugins ...string) string {
	t.Helper()
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pdir := t.TempDir()
	for _, name := range plugins {
		d := filepath.Join(pdir, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		m := `{"name":"` + name + `","version":"1.2.3","description":"d","category":"c","license":"MIT"}`
		if err := os.WriteFile(filepath.Join(d, "plugin.json"), []byte(m), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NSELF_PLUGIN_DIR", pdir)
	t.Chdir(proj)
	return proj
}

func listLedgerRows(t *testing.T) (map[string]ledgerRow, error) {
	t.Helper()
	root := newBundleTestCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"bundle", "list", "--json", "--installed=false"})
	if err := root.Execute(); err != nil {
		return nil, err
	}
	var rows []ledgerRow
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("output is not a JSON array of rows: %v\n%s", err, buf.String())
	}
	m := map[string]ledgerRow{}
	for _, r := range rows {
		m[r.Slug] = r
	}
	return m, nil
}

// TestBundleListLedgerView: every row carries a ledger view; with no ledger
// file the view is bootstrapped from the plugin dir and nothing is written;
// with a ledger file the view reads the file.
func TestBundleListLedgerView(t *testing.T) {
	proj := ledgerProject(t, "bots", "livekit", "ai", "notifications")

	rows, err := listLedgerRows(t)
	if err != nil {
		t.Fatal(err)
	}
	for slug, r := range rows {
		if r.Ledger.Plugins == nil {
			t.Errorf("row %s: ledger.plugins is null, want a list", slug)
		}
	}
	chat := rows["chat"]
	if !chat.Ledger.Installed || chat.Ledger.InstalledAt == "" || len(chat.Ledger.Plugins) != 2 {
		t.Fatalf("chat (bots+livekit installed) ledger = %+v", chat.Ledger)
	}
	for _, p := range chat.Ledger.Plugins {
		if p.Explicit || len(p.InstalledBy) != 1 || p.InstalledBy[0] != "chat" || p.Version != "1.2.3" {
			t.Errorf("chat plugin %+v: want installed_by [chat], explicit false", p)
		}
	}
	// claw is only partly installed (ai): not an installed bundle, ai is explicit.
	claw := rows["claw"]
	if claw.Ledger.Installed || len(claw.Ledger.Plugins) != 1 || !claw.Ledger.Plugins[0].Explicit {
		t.Fatalf("claw (partial) ledger = %+v", claw.Ledger)
	}
	// task is a structural bundle: never installed as a unit, notifications explicit.
	task := rows["task"]
	if task.Ledger.Installed || len(task.Ledger.Plugins) != 1 || !task.Ledger.Plugins[0].Explicit {
		t.Fatalf("task ledger = %+v", task.Ledger)
	}
	if _, err := os.Stat(filepath.Join(proj, ".nself")); !os.IsNotExist(err) {
		t.Fatalf("bundle list wrote %s/.nself (it must be read only): %v", proj, err)
	}

	// A ledger file wins over the on-disk inference.
	st := ledger.NewStore(proj)
	if err := st.Update(func(l *ledger.Ledger) error {
		l.Bundles["family"] = ledger.BundleRecord{InstalledAt: "2026-01-02T03:04:05Z"}
		l.Plugins["social"] = ledger.PluginRecord{InstalledBy: []string{"family"}, Tier: ledger.TierLicensed, Version: "9.9.9"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rows, err = listLedgerRows(t)
	if err != nil {
		t.Fatal(err)
	}
	fam := rows["family"]
	if !fam.Ledger.Installed || fam.Ledger.InstalledAt != "2026-01-02T03:04:05Z" || len(fam.Ledger.Plugins) != 1 || fam.Ledger.Plugins[0].Version != "9.9.9" {
		t.Fatalf("family should come from the ledger file: %+v", fam.Ledger)
	}
}

// TestBundleListLedgerDamaged: a damaged ledger fails the command with the file
// path; it is not replaced by a bootstrap guess.
func TestBundleListLedgerDamaged(t *testing.T) {
	proj := ledgerProject(t, "bots")
	dir := filepath.Join(proj, ".nself", "state")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bundles.json"), []byte(`{"hand":"edited"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := listLedgerRows(t)
	if err == nil || !strings.Contains(err.Error(), "bundles.json") {
		t.Fatalf("damaged ledger: err = %v, want an error naming the file", err)
	}
}
