package main

// policy_test.go — table-driven tests for tag selection and the text rewrite.

import (
	"reflect"
	"strings"
	"testing"
)

func mustRule(t *testing.T, regex, track string) rule {
	t.Helper()
	p, err := loadPolicy([]byte("schema_version: 1\nservices:\n  x: {regex: '" + regex + "', track: " + track + "}\n"))
	if err != nil {
		t.Fatal(err)
	}
	return p.rules["x"]
}

func TestSelectTag(t *testing.T) {
	v3 := `^v(\d+)\.(\d+)\.(\d+)$`
	opt := `^v(\d+)\.(\d+)(?:\.(\d+))?$`
	cases := []struct {
		name, regex, track, cur string
		tags                    []string
		want                    string
	}{
		{"patch stays in minor", v3, "patch", "v1.4.2", []string{"v1.4.3", "v1.5.0", "v2.0.0"}, "v1.4.3"},
		{"numeric not lexical", v3, "patch", "v1.4.2", []string{"v1.4.9", "v1.4.10"}, "v1.4.10"},
		{"minor stays in major", v3, "minor", "v1.9.0", []string{"v1.10.0", "v1.9.5", "v2.0.0"}, "v1.10.0"},
		{"v1.10.0 beats v1.9.0", v3, "minor", "v1.8.0", []string{"v1.9.0", "v1.10.0"}, "v1.10.0"},
		{"pre-release filtered", v3, "minor", "v2.44.0", []string{"v2.45.0-rc1", "v2.44.1"}, "v2.44.1"},
		{"never lower", v3, "patch", "v1.4.5", []string{"v1.4.1", "v1.4.4"}, "v1.4.5"},
		{"floating unchanged", `^latest$`, "patch", "latest", []string{"latest", "edge"}, "latest"},
		{"two groups fixed on patch", `^(\d+)\.(\d+)$`, "patch", "1.36", []string{"1.37", "1.36"}, "1.36"},
		{"one group fixed", `^(\d+)-alpine$`, "patch", "16-alpine", []string{"17-alpine"}, "16-alpine"},
		{"optional third group", opt, "patch", "v1.6", []string{"v1.6.3", "v1.7"}, "v1.6.3"},
		{"no tags", v3, "patch", "v1.0.0", nil, "v1.0.0"},
		{"huge group skipped", v3, "patch", "v1.0.0", []string{"v1.0.99999999999999999999999"}, "v1.0.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := selectTag(mustRule(t, c.regex, c.track), c.cur, c.tags)
			if err != nil || got != c.want {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

func TestSelectTagCurrentMismatch(t *testing.T) {
	_, err := selectTag(mustRule(t, `^v(\d+)\.(\d+)\.(\d+)$`, "patch"), "latest", nil)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b []int
		want int
	}{
		{[]int{1, 10}, []int{1, 9}, 1}, {[]int{1, 9}, []int{1, 10}, -1}, {[]int{1, 6}, []int{1, 6, 0}, 0},
		{[]int{1, 6, 1}, []int{1, 6}, 1}, {nil, nil, 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compare(%v,%v) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestRewriteVersions(t *testing.T) {
	src := "# head\nschema_version: 1\nimages:\n" +
		"  - {name: a, role: core, repository: r/a, version: v1.0.0, license: MIT, source: \"https://x/y\"}\n" +
		"  - {name: b, role: core, repository: r/b, version: \"4.48\", license: MIT}\n" +
		"  - {name: c, role: core, repository: r/c, version: '1.36', license: MIT}\n" +
		"  - {name: d, role: core, repository: r/d, version: 7.0.0, license: MIT}\n" +
		"  - {name: ab, role: core, repository: r/ab, version: v9, license: MIT}\n"
	bumps := []Bump{{"a", "v1.0.0", "v1.0.3"}, {"b", "4.48", "4.49"}, {"c", "1.36", "1.37"}, {"d", "7.0.0", "7.1"}}
	got, err := rewriteVersions([]byte(src), bumps)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer("version: v1.0.0", "version: v1.0.3", `version: "4.48"`, `version: "4.49"`,
		"version: '1.36'", "version: '1.37'", "version: 7.0.0", `version: "7.1"`).Replace(src)
	if string(got) != want {
		t.Fatalf("rewrite\n got: %s\nwant: %s", got, want)
	}
}

func TestRewriteVersionsErrors(t *testing.T) {
	one := "images:\n  - {name: a, role: core, version: v1}\n"
	if _, err := rewriteVersions([]byte(one), []Bump{{"zzz", "v1", "v2"}}); err == nil {
		t.Error("missing entry must fail")
	}
	dup := one + "  - {name: a, role: core, version: v1}\n"
	if _, err := rewriteVersions([]byte(dup), []Bump{{"a", "v1", "v2"}}); err == nil {
		t.Error("duplicate entry must fail")
	}
}

func TestRenderVersion(t *testing.T) {
	cases := []struct{ old, nv, want string }{
		{"v1", "v2", "v2"}, {`"1"`, "2", `"2"`}, {"'1'", "2", "'2'"}, {"7.0.0", "7.1", `"7.1"`}, {"v1", "v2 x", `"v2 x"`},
	}
	for _, c := range cases {
		if got := renderVersion(c.old, c.nv); got != c.want {
			t.Errorf("renderVersion(%s,%s) = %s, want %s", c.old, c.nv, got, c.want)
		}
	}
}

func TestPlanBumpsSkipsPluginAndManual(t *testing.T) {
	pol, err := loadPolicy([]byte("schema_version: 1\nservices:\n  m: {regex: '^v(\\d+)$', track: manual}\n  s: {regex: '^v(\\d+)\\.(\\d+)$', track: patch}\n"))
	if err != nil {
		t.Fatal(err)
	}
	imgs := []Image{{"p", "plugin", "r/p", "v1"}, {"m", "core", "r/m", "v1"}, {"s", "core", "r/s", "v1.2"}}
	var note strings.Builder
	got, probs := planBumps(imgs, pol, stubLister{"r/s": {"v1.2"}}, &note)
	if len(probs) != 0 || len(got) != 0 || !reflect.DeepEqual(pol.names(), []string{"m", "s"}) {
		t.Fatalf("bumps %v problems %v", got, probs)
	}
	if !strings.Contains(note.String(), "m: track manual, skipped") {
		t.Fatalf("note %q", note.String())
	}
}

func TestListTimeout(t *testing.T) {
	t.Setenv("IMAGEBUMP_LIST_TIMEOUT", "")
	if got := listTimeout(); got != defaultListTimeout {
		t.Errorf("default = %v", got)
	}
	t.Setenv("IMAGEBUMP_LIST_TIMEOUT", "90s")
	if got := listTimeout().Seconds(); got != 90 {
		t.Errorf("override = %v", got)
	}
	t.Setenv("IMAGEBUMP_LIST_TIMEOUT", "bogus")
	if got := listTimeout(); got != defaultListTimeout {
		t.Errorf("bogus = %v", got)
	}
}
