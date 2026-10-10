package ledger

// Tests for the bundle ledger (P7-PLUG-18): schema, first key, validation and
// bootstrap. Store behaviour (atomic write, locking) is in store_test.go.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const sum1 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func fixedNow() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }

// fixtureSources: bundle "chat" (bots, livekit), bundle "claw" (ai, cron) which
// overlaps chat on livekit through "shared", and the structural bundle "task".
// Installed: bots, livekit, ai, cron, notes (no bundle), and "mux" which is
// only half of bundle "mux-pack" (mux, missing).
func fixtureSources() Sources {
	return Sources{
		Now: fixedNow,
		Bundles: []BundleDef{
			{Slug: "chat", Installable: true, Plugins: []string{"bots", "livekit", "shared"}},
			{Slug: "claw", Installable: true, Plugins: []string{"ai", "cron", "shared"}},
			{Slug: "mux-pack", Installable: true, Plugins: []string{"mux", "missing"}},
			{Slug: "task", Installable: false, Plugins: []string{"notes", "cron"}},
		},
		Plugins: []InstalledPlugin{
			{Name: "bots", Version: "1.2.0", Tier: "licensed", Checksum: "sha256:" + sum1},
			{Name: "livekit", Version: "1.0.0", Tier: "licensed"},
			{Name: "shared", Version: "0.3.0", Tier: "licensed"},
			{Name: "ai", Version: "2.0.0", Tier: "licensed"},
			{Name: "Cron", Version: "1.1.0", Tier: "free"},
			{Name: "notes", Version: "0.1.0", Tier: "free"},
			{Name: "mux", Version: "0.9.0", Tier: "weird", Checksum: "not-a-checksum"},
		},
	}
}

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "schemas", "bundle-ledger.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("ledger.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func validates(t *testing.T, s *jsonschema.Schema, raw []byte) error {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("test JSON does not parse: %v", err)
	}
	return s.Validate(v)
}

// TestLedgerSchema: marshalled ledgers validate against the committed schema,
// the first key is _generated, and each broken variant is rejected by BOTH the
// schema and ledger.Parse.
func TestLedgerSchema(t *testing.T) {
	s := compileSchema(t)
	good, err := Bootstrap(fixtureSources()).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	empty, err := New().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{"bootstrap": good, "empty": empty} {
		if err := validates(t, s, raw); err != nil {
			t.Errorf("%s ledger fails the schema: %v\n%s", name, err, raw)
		}
		if !bytes.HasPrefix(raw, []byte("{\n  \"_generated\": ")) {
			t.Errorf("%s ledger: first key is not _generated:\n%.80s", name, raw)
		}
		if _, err := Parse(raw); err != nil {
			t.Errorf("%s ledger does not parse back: %v", name, err)
		}
	}

	mutate := func(f func(m map[string]any)) []byte {
		var m map[string]any
		if err := json.Unmarshal(good, &m); err != nil {
			t.Fatal(err)
		}
		f(m)
		b, _ := json.MarshalIndent(m, "", "  ")
		return b
	}
	plug := func(m map[string]any, slug string) map[string]any {
		return m["plugins"].(map[string]any)[slug].(map[string]any)
	}
	bad := map[string][]byte{
		"tier-unknown":       mutate(func(m map[string]any) { plug(m, "bots")["tier"] = "pro" }),
		"checksum-uppercase": mutate(func(m map[string]any) { plug(m, "bots")["checksum"] = strings.ToUpper(sum1) }),
		"checksum-short":     mutate(func(m map[string]any) { plug(m, "bots")["checksum"] = "abc" }),
		"extra-plugin-key":   mutate(func(m map[string]any) { plug(m, "bots")["hand_edit"] = true }),
		"extra-root-key":     mutate(func(m map[string]any) { m["note"] = "x" }),
		"version-2":          mutate(func(m map[string]any) { m["schema_version"] = 2 }),
		"no-generated":       mutate(func(m map[string]any) { delete(m, "_generated") }),
		"wrong-generated":    mutate(func(m map[string]any) { m["_generated"] = "hand written" }),
		"installed-by-dup":   mutate(func(m map[string]any) { plug(m, "bots")["installed_by"] = []any{"chat", "chat"} }),
		"bad-time": mutate(func(m map[string]any) {
			m["bundles"].(map[string]any)["chat"].(map[string]any)["installed_at"] = "yesterday"
		}),
		"missing-explicit":    mutate(func(m map[string]any) { delete(plug(m, "bots"), "explicit") }),
		"null-explicit":       mutate(func(m map[string]any) { plug(m, "bots")["explicit"] = nil }),
		"null-installed-by":   mutate(func(m map[string]any) { plug(m, "bots")["installed_by"] = nil }),
		"null-version":        mutate(func(m map[string]any) { plug(m, "bots")["version"] = nil }),
		"null-checksum":       mutate(func(m map[string]any) { plug(m, "bots")["checksum"] = nil }),
		"null-tier":           mutate(func(m map[string]any) { plug(m, "bots")["tier"] = nil }),
		"null-bundles":        mutate(func(m map[string]any) { m["bundles"] = nil }),
		"null-plugins":        mutate(func(m map[string]any) { m["plugins"] = nil }),
		"null-schema-version": mutate(func(m map[string]any) { m["schema_version"] = nil }),
		"null-installed-at":   mutate(func(m map[string]any) { m["bundles"].(map[string]any)["chat"].(map[string]any)["installed_at"] = nil }),
		"null-plugin-record":  mutate(func(m map[string]any) { m["plugins"].(map[string]any)["bots"] = nil }),
		"null-bundle-record":  mutate(func(m map[string]any) { m["bundles"].(map[string]any)["chat"] = nil }),
	}
	for name, raw := range bad {
		if validates(t, s, raw) == nil {
			t.Errorf("%s: schema accepted a broken ledger", name)
		}
		if _, err := Parse(raw); err == nil {
			t.Errorf("%s: Parse accepted a broken ledger", name)
		}
	}
}

// TestLedgerFirstKeyEnforced: key order is not expressible in the schema, so
// Parse must refuse a file whose first key is not _generated.
func TestLedgerFirstKeyEnforced(t *testing.T) {
	good, _ := New().Marshal()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(good, &m); err != nil {
		t.Fatal(err)
	}
	reordered := []byte(`{"schema_version":1,"_generated":` + string(m["_generated"]) + `,"bundles":{},"plugins":{}}`)
	if _, err := Parse(reordered); err == nil || !strings.Contains(err.Error(), "first key") {
		t.Fatalf("Parse accepted a ledger whose first key is not _generated: %v", err)
	}
	if _, err := Parse(append(append([]byte{}, good...), []byte(`{}`)...)); err == nil {
		t.Fatal("Parse accepted trailing data")
	}
}

// TestLedgerValidateRules: the references the schema cannot check.
func TestLedgerValidateRules(t *testing.T) {
	mk := func(f func(l *Ledger)) error {
		l := Bootstrap(fixtureSources())
		f(&l)
		_, err := l.Marshal()
		return err
	}
	cases := map[string]func(l *Ledger){
		"installed-by-unknown-bundle": func(l *Ledger) {
			p := l.Plugins["bots"]
			p.InstalledBy = []string{"ghost"}
			l.Plugins["bots"] = p
		},
		"orphan-record": func(l *Ledger) {
			l.Plugins["stray"] = PluginRecord{InstalledBy: []string{}, Explicit: false, Tier: TierFree}
		},
		"bad-slug": func(l *Ledger) { l.Plugins["Bad Slug"] = PluginRecord{Explicit: true, Tier: TierFree} },
	}
	for name, f := range cases {
		if mk(f) == nil {
			t.Errorf("%s: Marshal accepted an invalid ledger", name)
		}
	}
	if err := mk(func(*Ledger) {}); err != nil {
		t.Fatalf("control: unmodified bootstrap ledger rejected: %v", err)
	}
}

// TestLedgerBootstrap: installed_by comes from bundle membership, explicit is
// true exactly for plugins in no installed bundle.
func TestLedgerBootstrap(t *testing.T) {
	l := Bootstrap(fixtureSources())

	wantBundles := []string{"chat", "claw"}
	var gotBundles []string
	for b, r := range l.Bundles {
		gotBundles = append(gotBundles, b)
		if r.InstalledAt != "2026-10-10T12:00:00Z" {
			t.Errorf("bundle %s installed_at = %q", b, r.InstalledAt)
		}
	}
	sort.Strings(gotBundles)
	if !reflect.DeepEqual(gotBundles, wantBundles) {
		t.Fatalf("installed bundles = %v, want %v (mux-pack is partial, task is structural)", gotBundles, wantBundles)
	}

	type want struct {
		by       []string
		explicit bool
		tier     string
		checksum string
	}
	cases := map[string]want{
		"bots":    {[]string{"chat"}, false, TierLicensed, sum1},
		"livekit": {[]string{"chat"}, false, TierLicensed, ""},
		"shared":  {[]string{"chat", "claw"}, false, TierLicensed, ""},
		"ai":      {[]string{"claw"}, false, TierLicensed, ""},
		"cron":    {[]string{"claw"}, false, TierFree, ""},
		"notes":   {[]string{}, true, TierFree, ""},
		"mux":     {[]string{}, true, TierFree, ""},
	}
	if len(l.Plugins) != len(cases) {
		t.Errorf("recorded %d plugins, want %d: %v", len(l.Plugins), len(cases), l.Plugins)
	}
	for slug, w := range cases {
		p, ok := l.Plugins[slug]
		if !ok {
			t.Errorf("plugin %s not recorded", slug)
			continue
		}
		if !reflect.DeepEqual(p.InstalledBy, w.by) || p.Explicit != w.explicit || p.Tier != w.tier || p.Checksum != w.checksum {
			t.Errorf("plugin %s = %+v, want %+v", slug, p, w)
		}
	}
	if _, err := l.Marshal(); err != nil {
		t.Errorf("bootstrap result is not a valid ledger: %v", err)
	}
	// Nothing installed: an empty, valid ledger.
	if e := Bootstrap(Sources{Now: fixedNow}); len(e.Bundles)+len(e.Plugins) != 0 {
		t.Errorf("empty sources produced %+v", e)
	}
}

// TestLedgerView: per-bundle view used by `bundle list --json`.
func TestLedgerView(t *testing.T) {
	l := Bootstrap(fixtureSources())
	v := l.View("chat", []string{"bots", "livekit", "shared", "absent"})
	if !v.Installed || v.InstalledAt == "" || len(v.Plugins) != 3 || v.Plugins[0].Slug != "bots" {
		t.Errorf("chat view = %+v", v)
	}
	n := l.View("mux-pack", []string{"mux", "missing"})
	if n.Installed || n.InstalledAt != "" || len(n.Plugins) != 1 || !n.Plugins[0].Explicit {
		t.Errorf("mux-pack view = %+v", n)
	}
	raw, _ := json.Marshal(l.View("nothing", nil))
	if !strings.Contains(string(raw), `"plugins":[]`) {
		t.Errorf("empty view must print plugins as [], got %s", raw)
	}
}

// TestLedgerDuplicateKeys: encoding/json keeps the last of two equal keys; the
// reader must refuse them at every depth.
func TestLedgerDuplicateKeys(t *testing.T) {
	good, _ := Bootstrap(fixtureSources()).Marshal()
	if _, err := Parse(good); err != nil {
		t.Fatalf("control: %v", err)
	}
	bots := `"bots": {"installed_by": ["chat"], "explicit": false, "tier": "licensed", "version": "1", "checksum": ""}`
	head := `{"_generated": ` + string(mustJSON(t, Generated)) + `, "schema_version": 1, `
	chat := `"chat": {"installed_at": "2026-10-10T12:00:00Z"}`
	cases := map[string]string{
		"dup-plugin":    head + `"bundles": {` + chat + `}, "plugins": {` + bots + `, ` + bots + `}}`,
		"dup-bundle":    head + `"bundles": {` + chat + `, ` + chat + `}, "plugins": {}}`,
		"dup-field":     head + `"bundles": {` + chat + `}, "plugins": {"bots": {"installed_by": ["chat"], "explicit": false, "explicit": true, "tier": "free", "version": "", "checksum": ""}}}`,
		"dup-root":      head + `"bundles": {}, "plugins": {}, "bundles": {}}`,
		"dup-generated": `{"_generated": ` + string(mustJSON(t, Generated)) + `, "_generated": ` + string(mustJSON(t, Generated)) + `, "schema_version": 1, "bundles": {}, "plugins": {}}`,
	}
	for name, doc := range cases {
		if _, err := Parse([]byte(doc)); err == nil || !strings.Contains(err.Error(), "duplicate key") {
			t.Errorf("%s: err = %v, want a duplicate key error", name, err)
		}
	}
	ok := head + `"bundles": {` + chat + `}, "plugins": {` + bots + `}}`
	if _, err := Parse([]byte(ok)); err != nil {
		t.Errorf("control without duplicates rejected: %v", err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestLedgerBootstrapSkipsInvalidSlugs: a name that is not a slug is reported
// and left out, so the ledger stays writable.
func TestLedgerBootstrapSkipsInvalidSlugs(t *testing.T) {
	src := fixtureSources()
	src.Plugins = append(src.Plugins, InstalledPlugin{Name: "my plugin", Version: "1"}, InstalledPlugin{Name: "UPPER_ok", Version: "1"})
	src.Bundles = append(src.Bundles, BundleDef{Slug: "Bad Bundle", Installable: true, Plugins: []string{"notes"}})
	l, skipped := BootstrapReport(src)
	if len(skipped) != 2 {
		t.Fatalf("skipped = %v, want the plugin %q and the bundle", skipped, "my plugin")
	}
	if _, ok := l.Plugins["my plugin"]; ok {
		t.Error("invalid plugin slug was recorded")
	}
	if _, ok := l.Plugins["upper_ok"]; !ok {
		t.Error("a valid name after lower-casing was dropped")
	}
	if _, err := l.Marshal(); err != nil {
		t.Fatalf("bootstrap result with a bad name is not writable: %v", err)
	}
	// And through the store: the first write works and warns.
	s := newTestStore(t)
	s.Sources = func() (Sources, error) { return src, nil }
	var warned []string
	s.Warn = func(m string) { warned = append(warned, m) }
	if created, err := s.EnsureBootstrapped(); err != nil || !created {
		t.Fatalf("EnsureBootstrapped with a bad plugin name: created=%v err=%v", created, err)
	}
	if len(warned) != 2 {
		t.Errorf("warnings = %v", warned)
	}
}
