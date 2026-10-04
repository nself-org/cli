package cmdregistry

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestExitCodesFollowMode(t *testing.T) {
	root := fixtureRoot(false)
	v14, _ := mustBuild(t, root, false).Lookup("nself status")
	if want := map[string]string{"2": "one or more services unhealthy"}; !reflect.DeepEqual(v14.ExitCodes, want) {
		t.Errorf("v1.4 exit_codes = %v, want %v", v14.ExitCodes, want)
	}
	v15, _ := mustBuild(t, root, true).Lookup("nself status")
	if want := map[string]string{"10": "one or more services unhealthy"}; !reflect.DeepEqual(v15.ExitCodes, want) {
		t.Errorf("v1.5 exit_codes = %v, want %v", v15.ExitCodes, want)
	}
	none, _ := mustBuild(t, root, true).Lookup("nself add")
	if none.ExitCodes == nil || len(none.ExitCodes) != 0 {
		t.Errorf("no exit codes must be an empty object, got %#v", none.ExitCodes)
	}
}

func TestV15OnlyEnvelope(t *testing.T) {
	root := fixtureRoot(false)
	old, _ := mustBuild(t, root, false).Lookup("nself hub get")
	if old.JSON != "none" || old.DataSchema != nil {
		t.Errorf("v1.4: json=%q data_schema=%v, want none/nil", old.JSON, old.DataSchema)
	}
	cur, _ := mustBuild(t, root, true).Lookup("nself hub get")
	if cur.JSON != "envelope" || cur.DataSchema == nil || *cur.DataSchema != "schemas/commands/hub-get.v1.schema.json" {
		t.Errorf("v1.5: json=%q data_schema=%v, want envelope + schema path", cur.JSON, cur.DataSchema)
	}
	st, _ := mustBuild(t, root, false).Lookup("nself status")
	if st.JSON != "envelope" {
		t.Errorf("a path outside V15OnlyEnvelope is envelope in both modes, got %q", st.JSON)
	}
}

func TestDefaultsAndDerivation(t *testing.T) {
	r := mustBuild(t, fixtureRoot(false), true)
	get, _ := r.Lookup("hub get")
	if get.Canon != "subcommand" || get.Output != "document" || get.Parent != "nself hub" {
		t.Errorf("hub get defaults wrong: %+v", get)
	}
	hub, _ := r.Lookup("nself hub")
	if hub.Runnable || hub.SideEffect != "read" || hub.Group == nil || *hub.Group != "core" {
		t.Errorf("hub (non-runnable) wrong: %+v", hub)
	}
	shim, _ := r.Lookup("nself oldstatus")
	if shim.Target == nil || *shim.Target != "nself status" || !shim.Hidden || shim.Deprecated == nil {
		t.Errorf("shim wrong: %+v", shim)
	}
	if !reflect.DeepEqual(shim.Aliases, []string{"os", "ost"}) {
		t.Errorf("aliases not sorted: %v", shim.Aliases)
	}
	logs, _ := r.Lookup("nself logs")
	if logs.Output != "stream" || logs.JSON != "legacy" {
		t.Errorf("logs wrong: %+v", logs)
	}
	c := r.Counts
	if c.Commands != len(r.Commands) || c.TopLevel != 6 || c.Core != 5 || c.Pending != 1 || c.DeprecatedShims != 1 || c.JSONEnvelope != 2 || c.JSONLegacy != 1 {
		t.Errorf("counts wrong: %+v", c)
	}
	if !reflect.DeepEqual(c.CoreMissing, []string{"ghost"}) {
		t.Errorf("core_missing = %v", c.CoreMissing)
	}
	if _, ok := r.Lookup("nself help"); !ok {
		t.Error("help must be in the registry")
	}
}

func TestFlagCollection(t *testing.T) {
	r := mustBuild(t, fixtureRoot(false), true)
	st, _ := r.Lookup("nself status")
	byName := map[string]Flag{}
	for _, f := range st.Flags {
		byName[f.Name] = f
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "debugx,json,old,scope,watch" {
		t.Fatalf("status flags = %s (inherited --verbose must not appear)", got)
	}
	if !sort.SliceIsSorted(st.Flags, func(i, j int) bool { return st.Flags[i].Name < st.Flags[j].Name }) {
		t.Error("flags not sorted by name")
	}
	if f := byName["watch"]; f.Shorthand == nil || *f.Shorthand != "w" || f.Type != "bool" || f.Default != "false" {
		t.Errorf("watch: %+v", f)
	}
	if !byName["debugx"].Hidden || byName["old"].Deprecated == nil || !byName["scope"].Persistent || byName["json"].Persistent {
		t.Errorf("hidden/deprecated/persistent wrong: %+v", byName)
	}
	add, _ := r.Lookup("nself add")
	if !add.Flags[0].Required {
		t.Error("required flag not detected")
	}
	get, _ := r.Lookup("nself hub get")
	if get.Flags[0].Default != "text" || get.Flags[0].Type != "string" {
		t.Errorf("format flag: %+v", get.Flags[0])
	}
	doc, _ := r.Lookup("nself doctor")
	for _, f := range doc.Flags {
		if f.Name == "fix" && (f.SideEffect == nil || *f.SideEffect != "write") {
			t.Errorf("fix override missing: %+v", f)
		}
		if f.Name == "install-check" && (f.JSON == nil || *f.JSON != "legacy" || f.SideEffect != nil) {
			t.Errorf("install-check override wrong: %+v", f)
		}
	}
}

func TestStreamFlagOverride(t *testing.T) {
	r := mustBuild(t, fixtureRoot(false), true)
	dep, _ := r.Lookup("nself deploy")
	if dep.Flags[0].Output == nil || *dep.Flags[0].Output != "stream" || dep.Output != "document" {
		t.Errorf("deploy --stream: %+v", dep.Flags[0])
	}
	b := string(mustMarshal(t, r))
	if !strings.Contains(b, `"output": "stream"`) {
		t.Error("flag output stream not in the marshalled document")
	}
}

// TestHelpFlagIsNeverListed proves cobra's lazily added help flag cannot make
// two registries of the same tree differ.
func TestHelpFlagIsNeverListed(t *testing.T) {
	plain := mustMarshal(t, mustBuild(t, fixtureRoot(false), true))
	root := fixtureRoot(false)
	root.InitDefaultHelpFlag()
	var rec func(c *cobra.Command)
	rec = func(c *cobra.Command) {
		for _, ch := range c.Commands() {
			ch.InitDefaultHelpFlag()
			rec(ch)
		}
	}
	rec(root)
	if root.Flags().Lookup("help") == nil {
		t.Fatal("test setup: help flag not added")
	}
	if got := mustMarshal(t, mustBuild(t, root, true)); !bytes.Equal(plain, got) {
		t.Error("InitDefaultHelpFlag changed the registry bytes")
	}
	if strings.Contains(string(plain), `"name": "help",`) && strings.Contains(string(plain), `"usage": "help for`) {
		t.Error("a help flag leaked into the registry")
	}
}

func snapshot(root *cobra.Command) string {
	var b strings.Builder
	var rec func(c *cobra.Command)
	rec = func(c *cobra.Command) {
		fmt.Fprintf(&b, "%s|%s|%v|%s|%s|%v|", c.CommandPath(), c.Use, c.Hidden, c.Deprecated, c.GroupID, c.Aliases)
		fmt.Fprintf(&b, "run=%v runE=%v|", reflect.ValueOf(c.Run).Pointer(), reflect.ValueOf(c.RunE).Pointer())
		c.Flags().VisitAll(func(f *pflag.Flag) {
			fmt.Fprintf(&b, "F:%s=%s/%s/%v/%p;", f.Name, f.Value.String(), f.DefValue, f.Changed, f)
		})
		c.PersistentFlags().VisitAll(func(f *pflag.Flag) { fmt.Fprintf(&b, "P:%s=%s/%v/%p;", f.Name, f.Value.String(), f.Changed, f) })
		b.WriteString("\n")
		for _, ch := range c.Commands() {
			rec(ch)
		}
	}
	rec(root)
	return b.String()
}

func TestBuildIsPure(t *testing.T) {
	root := fixtureRoot(false)
	before := snapshot(root)
	for i := 0; i < 2; i++ {
		mustBuild(t, root, i == 1)
	}
	if after := snapshot(root); after != before {
		t.Errorf("Build mutated the tree\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestDeterministic(t *testing.T) {
	old := cobra.EnableCommandSorting
	cobra.EnableCommandSorting = false
	defer func() { cobra.EnableCommandSorting = old }()
	a := mustMarshal(t, mustBuild(t, fixtureRoot(false), true))
	if again := mustMarshal(t, mustBuild(t, fixtureRoot(false), true)); !bytes.Equal(a, again) {
		t.Error("two builds of the same tree differ")
	}
	if shuffled := mustMarshal(t, mustBuild(t, fixtureRoot(true), true)); !bytes.Equal(a, shuffled) {
		t.Error("AddCommand order changed the bytes")
	}
}

func TestLookupAndSubtree(t *testing.T) {
	r := mustBuild(t, fixtureRoot(false), true)
	if c, ok := r.Lookup("hub get"); !ok || c.Path != "nself hub get" {
		t.Errorf("Lookup(short form) = %v %v", c, ok)
	}
	if _, ok := r.Lookup("nself nope"); ok {
		t.Error("Lookup found a missing command")
	}
	sub := r.Subtree("nself hub")
	if sub == nil || len(sub.Commands) != 3 || sub.Counts.Commands != 3 || sub.Counts.Core != 1 {
		t.Fatalf("Subtree(hub) = %+v", sub)
	}
	if !reflect.DeepEqual(sub.Counts.CoreMissing, r.Counts.CoreMissing) {
		t.Errorf("subtree core_missing = %v", sub.Counts.CoreMissing)
	}
	if r.Subtree("nope") != nil {
		t.Error("Subtree of a missing path must be nil")
	}
	if leaf := r.Subtree("hub get"); leaf == nil || len(leaf.Commands) != 1 {
		t.Errorf("Subtree(leaf) = %+v", leaf)
	}
	if _, err := sub.Marshal(); err != nil {
		t.Error(err)
	}
}

func TestMarshalFormat(t *testing.T) {
	b := mustMarshal(t, mustBuild(t, fixtureRoot(false), true))
	if !bytes.HasSuffix(b, []byte("}\n")) || bytes.HasSuffix(b, []byte("\n\n")) {
		t.Error("want exactly one trailing newline")
	}
	if !bytes.HasPrefix(b, []byte("{\n  \"schema_version\": \"1\",\n")) {
		t.Errorf("not 2-space indented: %.40q", b)
	}
	r := mustBuild(t, fixtureRoot(false), true)
	r.Commands[0].Summary = "a < b & c"
	if out := mustMarshal(t, r); !bytes.Contains(out, []byte("a < b & c")) {
		t.Error("HTML escaping must be off")
	}
}

func TestParseArgs(t *testing.T) {
	cases := map[string][]Arg{
		"get":                        {},
		"get <key>":                  {{"key", true, false}},
		"status [SERVICE]":           {{"SERVICE", false, false}},
		"add <key> [key...]":         {{"key", true, false}, {"key", false, true}},
		"rm <a>... [flags]":          {{"a", true, true}},
		"x [a…]":                     {{"a", false, true}},
		"x [options] <first|second>": {{"options", false, false}, {"first|second", true, false}},
		"x bare":                     {{"bare", true, false}},
		"x [key value]":              {{"key value", false, false}},
	}
	for use, want := range cases {
		got := parseArgs(use)
		if len(got) == 0 && len(want) == 0 && got != nil {
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseArgs(%q) = %v, want %v", use, got, want)
		}
	}
	if parseArgs("get") == nil {
		t.Error("parseArgs must return a non-nil slice")
	}
}
