package canon

// Tests for the fragment loader and its validation rules (P7-CANON-02,
// contract:cli.canon-fragments v1): the root-token table, every rule with a
// positive and a negative fixture, and the missing-fragment check.

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

const movedDir = "testdata/fragments/moved"

// movedFS returns the moved fixture as an in-memory FS the test can mutate.
func movedFS(t *testing.T, mutate func(m fstest.MapFS)) fs.FS {
	t.Helper()
	m := fstest.MapFS{}
	root := os.DirFS(movedDir)
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(root, p)
		m[p] = &fstest.MapFile{Data: b}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(m)
	}
	return m
}

func setFile(path, data string) func(fstest.MapFS) {
	return func(m fstest.MapFS) { m[path] = &fstest.MapFile{Data: []byte(data)} }
}

// fragmentTable is the EPIC P7-CANON contract table: root token -> fragment.
var fragmentTable = map[string]string{
	"init": "lifecycle", "start": "lifecycle", "stop": "lifecycle", "restart": "lifecycle", "dev": "lifecycle",
	"build": "build", "reset": "build", "clean": "build", "uninstall": "build",
	"exec": "io", "logs": "io",
	"status": "observe", "doctor": "observe", "urls": "observe", "health": "observe", "self-heal": "observe",
	"help": "observe", "help-topics": "observe", "completion": "observe", "man": "observe", "version": "observe",
	"config": "config", "env": "config", "secrets": "config", "oauth": "config", "telemetry": "config", "trust": "config",
	"license": "account", "account": "account", "login": "account", "logout": "account",
	"db": "data", "backup": "data", "migrate": "data", "generate": "data", "template": "data",
	"deploy": "deploy", "update": "deploy", "promote": "deploy", "ops": "deploy", "access": "deploy",
	"security": "deploy", "verify-sbom": "deploy",
	"service": "services",
	"install": "extend", "remove": "extend", "plugin": "extend", "bundle": "extend",
	"mcp": "mcp",
	"ci":  "breakouts", "runner": "breakouts", "functions": "breakouts", "server": "breakouts",
	"admin": "admin",
}

// authorTools are the `plugin <x>` subtrees the table assigns to breakouts.
var authorTools = map[string]bool{"init": true, "new": true, "dev": true, "debug": true, "link": true, "unlink": true, "test": true, "marketplace": true, "submit": true}

func fragmentFor(key string) string {
	w := strings.Fields(key)
	if w[0] == "plugin" && len(w) > 1 && authorTools[w[1]] {
		return "breakouts"
	}
	return fragmentTable[w[0]]
}

// TestFragmentAssignment: every entry of the pre-split canon lives in the
// fragment the EPIC table chooses for its root token, and nothing was lost.
func TestFragmentAssignment(t *testing.T) {
	pre := readPresplit(t)
	raw, err := LoadRaw()
	if err != nil {
		t.Fatal(err)
	}
	where := map[string]string{}
	count := map[string]int{}
	for k := range raw.Commands {
		frag := strings.TrimSuffix(strings.TrimPrefix(raw.origin["commands:"+k], "domains/"), ".yaml")
		where[k] = frag
		count[frag]++
	}
	// A moved, shimmed, broken-out or removed command satisfies the pre-split presence check
	// for its old path in the fragment that claims it.
	for _, m := range raw.Moves {
		frag := strings.TrimSuffix(strings.TrimPrefix(raw.origin["from:"+m.From], "domains/"), ".yaml")
		where[m.From] = frag
		prefix := m.To + " "
		oldPrefix := m.From + " "
		for k := range raw.Commands {
			if strings.HasPrefix(k, prefix) {
				oldKey := oldPrefix + strings.TrimPrefix(k, prefix)
				where[oldKey] = where[k]
				delete(where, k)
			}
		}
		delete(where, m.To)
	}
	for _, m := range raw.Shims {
		where[m.From] = strings.TrimSuffix(strings.TrimPrefix(raw.origin["from:"+m.From], "domains/"), ".yaml")
	}
	for _, m := range raw.RetiredHubs {
		where[m.From] = strings.TrimSuffix(strings.TrimPrefix(raw.origin["from:"+m.From], "domains/"), ".yaml")
	}
	for _, m := range raw.Breakouts {
		where[m.From] = strings.TrimSuffix(strings.TrimPrefix(raw.origin["from:"+m.From], "domains/"), ".yaml")
	}
	for _, m := range raw.Removed {
		where[m.From] = strings.TrimSuffix(strings.TrimPrefix(raw.origin["from:"+m.From], "domains/"), ".yaml")
	}
	for _, name := range RequiredFragments {
		if count[name] == 0 {
			t.Errorf("fragment %s has no entries", name)
		}
	}
	for key := range pre.Commands {
		want := fragmentFor(key)
		if want == "" {
			t.Fatalf("%q: root token is not in the fragment table", key)
		}
		got, ok := where[key]
		if !ok {
			t.Errorf("%q is missing from every fragment", key)
			continue
		}
		if got != want {
			t.Errorf("%q is in fragment %s, the table says %s", key, got, want)
		}
	}
	delete(where, "deploy targets") // Added after the pre-split snapshot.
	if len(where) != len(pre.Commands) {
		var extra []string
		for k := range where {
			if _, ok := pre.Commands[k]; !ok {
				extra = append(extra, k)
			}
		}
		t.Errorf("fragments hold %d keys, pre-split canon had %d. Extra keys: %v", len(where), len(pre.Commands), extra)
	}
}

func readPresplit(t *testing.T) *File {
	t.Helper()
	b, err := os.ReadFile("testdata/presplit.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFragmentValidation(t *testing.T) {
	if _, err := LoadFS(movedFS(t, nil)); err != nil {
		t.Fatalf("good fixture: %v", err)
	}
	row := func(list, body string) func(fstest.MapFS) {
		return setFile("domains/extra.yaml", "schema_version: 1\ncommands: {}\n"+list+":\n  - {"+body+"}\n")
	}
	ok := "since: v1.5.0, removal_at: v1.6.0"
	cases := []struct {
		name   string
		mutate func(fstest.MapFS)
		want   []string
	}{
		{"key in two fragments", setFile("domains/extra.yaml", "schema_version: 1\ncommands:\n  config: {canon: core, side_effect: read}\n"),
			[]string{`commands["config"] is defined in both domains/config.yaml and domains/extra.yaml`}},
		{"from repeated across fragments", row("moves", "from: env, to: config env use, "+ok),
			[]string{`from "env" is declared in both domains/config.yaml and domains/extra.yaml`}},
		{"from repeated across lists", row("removed", "from: install, since: v1.5.0, message: gone"),
			[]string{`from "install" is declared in both domains/extend.yaml and domains/extra.yaml`}},
		{"hub path twice", row("hubs", "path: db migrate, summary: again"),
			[]string{`hub "db migrate" is declared in both domains/data.yaml and domains/extra.yaml`}},
		{"builtin twice", setFile("domains/extra.yaml", "schema_version: 1\ncommands: {}\nbuiltins: [completion]\n"),
			[]string{`builtin "completion" is declared in both domains/extra.yaml and domains/observe.yaml`}},
		{"removal_at not after since", row("moves", "from: a, to: b, since: v1.5.0, removal_at: v1.5.0"),
			[]string{`domains/extra.yaml: moves[0] ("a"): removal_at v1.5.0 must be after since v1.5.0`}},
		{"removal_at before since", row("moves", "from: a, to: b, since: v1.6.0, removal_at: v1.5.9"), []string{"must be after since"}},
		{"bad version", row("moves", "from: a, to: b, since: 1.5, removal_at: v1.6.0"), []string{`since "1.5" must look like vX.Y.Z`}},
		{"shim to is a shim", func(m fstest.MapFS) {
			setFile("domains/extra.yaml", "schema_version: 1\ncommands:\n  b: {canon: pending, side_effect: read, mode: v1.4}\n  d: {canon: pending, side_effect: read, mode: v1.4}\nshims:\n  - {from: b, to: service stop, "+ok+", equivalence: TestX}\n  - {from: d, to: b, "+ok+", equivalence: TestY}\n")(m)
		}, []string{`to "b" is itself a shim`}},
		{"shim to missing", row("shims", "from: gone, to: nowhere, "+ok+", equivalence: TestX"), []string{`to "nowhere" does not resolve`}},
		{"shim without equivalence", row("shims", "from: a, to: stop, "+ok), []string{"equivalence"}},
		{"mode not v1.4", setFile("domains/extra.yaml", "schema_version: 1\ncommands:\n  zed: {canon: pending, side_effect: read, mode: v1.5}\n"), []string{`mode "v1.5" is not v1.4`}},
		{"generated shim collides with a command", row("shims", "from: config env, to: stop, "+ok+", equivalence: TestX"), []string{`from "config env" is also a command at its canonical path`}},
		{"hub collides with a command", row("hubs", "path: config env, summary: x"), []string{`hub "config env" is also a command`}},
		{"move root missing key", row("moves", "from: x, to: y, "+ok), []string{`to "y" does not resolve`}},
		{"two entries map to one v1.4 path", func(m fstest.MapFS) {
			setFile("domains/extra.yaml", "schema_version: 1\ncommands:\n  env: {canon: pending, side_effect: read}\n")(m)
		}, []string{`maps onto v1.4 path "env"`}},
		{"bad fragment name", setFile("domains/Bad-Name.yaml", "schema_version: 1\ncommands: {}\n"), []string{"fragment name"}},
		{"fragment schema_version", setFile("domains/extra.yaml", "schema_version: 2\ncommands: {}\n"), []string{"domains/extra.yaml: schema_version: 2"}},
		{"unknown key in fragment", setFile("domains/extra.yaml", "schema_version: 1\ncommands: {}\nwat: 1\n"), []string{"domains/extra.yaml: decode"}},
		{"malformed fragment", setFile("domains/extra.yaml", "schema_version: 1\ncommands: [unclosed\n"), []string{"domains/extra.yaml: decode"}},
		{"empty fragment", setFile("domains/extra.yaml", ""), []string{"domains/extra.yaml: decode: empty document"}},
		{"verbs in a fragment", setFile("domains/extra.yaml", "schema_version: 1\nverbs: [a]\ncommands: {}\n"), []string{"domains/extra.yaml: decode"}},
		{"entry rule names the file", setFile("domains/extra.yaml", "schema_version: 1\ncommands:\n  oops: {side_effect: read}\n"), []string{`domains/extra.yaml: commands["oops"]: canon is required`}},
		{"bad row paths", row("breakouts", "from: ' a', plugin: Bad, to: '', since: x"), []string{"must be a command path", "plugin", "vX.Y.Z"}},
		{"removed without message", row("removed", "from: q, since: v1.5.0"), []string{"message is required"}},
		{"hub without summary", row("hubs", "path: zz"), []string{"summary is required"}},
		{"to equals from", row("moves", "from: a, to: a, "+ok), []string{"to must differ from from"}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			_, err := LoadFS(movedFS(t, c.mutate))
			if err == nil {
				t.Fatal("LoadFS succeeded, want an error")
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error lacks %q:\n%v", w, err)
				}
			}
		})
	}
}

func TestLoadFSRootErrors(t *testing.T) {
	cases := map[string]fs.FS{
		"missing canon.yaml": fstest.MapFS{},
		"commands in root":   fstest.MapFS{"canon.yaml": {Data: []byte("schema_version: 1\nverbs: [a]\ncommands: {}\n")}},
		"empty root":         fstest.MapFS{"canon.yaml": {Data: []byte("")}},
	}
	for name, fsys := range cases {
		if _, err := LoadFS(fsys); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	_, err := LoadFS(fstest.MapFS{"canon.yaml": {Data: []byte("schema_version: 9\nverbs: []\n")}})
	if err == nil || !strings.Contains(err.Error(), "schema_version: 9") || !strings.Contains(err.Error(), "verbs") {
		t.Errorf("bad root header: %v", err)
	}
}

func TestLoadFSUnreadableFragment(t *testing.T) {
	fsys := brokenFS{fstest.MapFS{
		"canon.yaml":          {Data: []byte("schema_version: 1\nverbs: [a]\n")},
		"domains/broken.yaml": {Data: []byte("x")},
	}}
	_, err := LoadFS(fsys)
	if err == nil || !strings.Contains(err.Error(), "domains/broken.yaml: read") {
		t.Errorf("unreadable fragment: %v", err)
	}
}

// brokenFS fails every read of a file named broken.yaml.
type brokenFS struct{ fstest.MapFS }

func (b brokenFS) ReadFile(name string) ([]byte, error) {
	if strings.HasSuffix(name, "broken.yaml") {
		return nil, fs.ErrPermission
	}
	return b.MapFS.ReadFile(name)
}

// TestMissingFragment: the embedded canon must carry every contract fragment.
func TestMissingFragment(t *testing.T) {
	full := fstest.MapFS{}
	for _, n := range RequiredFragments {
		full["domains/"+n+".yaml"] = &fstest.MapFile{}
	}
	if p := checkRequiredFragments(full); len(p) != 0 {
		t.Fatalf("complete set reported missing: %v", p)
	}
	delete(full, "domains/observe.yaml")
	p := checkRequiredFragments(full)
	if len(p) != 1 || !strings.Contains(p[0], "domains/observe.yaml: missing") {
		t.Fatalf("missing fragment not reported: %v", p)
	}
}

func TestParseValidatesRows(t *testing.T) {
	_, err := Parse([]byte("schema_version: 1\nverbs: [a]\ncommands:\n  x: {canon: pending, side_effect: read}\nmoves:\n  - {from: a, to: b, since: v2, removal_at: v1.0.0}\n"))
	if err == nil || !strings.Contains(err.Error(), "vX.Y.Z") {
		t.Fatalf("Parse accepted a bad row: %v", err)
	}
}
