package main

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/tools/facts/model"
)

func testFacts(version string) model.Facts {
	return model.Facts{Version: version, ContractMode: model.ContractMode, Verbs: []string{"init", "start"}, Counts: model.Counts{TopLevel: 2}}
}

const fixture = `# nSelf

Version: <!-- BEGIN GENERATED:facts:version -->old<!-- END GENERATED:facts:version -->.

<!-- BEGIN GENERATED:facts:verbs -->
stale
<!-- END GENERATED:facts:verbs -->

We ship <!-- BEGIN GENERATED:facts:top-level --><!-- END GENERATED:facts:top-level -->.

<!-- BEGIN GENERATED:command-table -->
untouched
<!-- END GENERATED:command-table -->
`

func TestFillReadmeAndIdempotent(t *testing.T) {
	f := testFacts("1.4.14")
	got, err := FillReadme(fixture, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Version: <!-- BEGIN GENERATED:facts:version -->1.4.14<!-- END GENERATED:facts:version -->.",
		"<!-- BEGIN GENERATED:facts:verbs -->\n`init`, `start`\n<!-- END GENERATED:facts:verbs -->",
		"<!-- BEGIN GENERATED:facts:top-level -->2 top-level commands in v1.5 mode, `NSELF_V15=1`<!-- END GENERATED:facts:top-level -->",
		"untouched",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	again, err := FillReadme(got, f)
	if err != nil || again != got {
		t.Fatalf("second run must be a no-op (err %v)", err)
	}
}

func TestTopLevelTextAtV15(t *testing.T) {
	got, _ := FillReadme("<!-- BEGIN GENERATED:facts:top-level --><!-- END GENERATED:facts:top-level -->", testFacts("1.5.0"))
	if strings.Contains(got, "NSELF_V15") || !strings.Contains(got, "2 top-level commands") {
		t.Fatalf("v1.5.0 must not name the mode: %s", got)
	}
}

func TestNoBlocksIsNoOp(t *testing.T) {
	in := "# plain\n\n<!-- BEGIN GENERATED:other -->\nx\n<!-- END GENERATED:other -->\n"
	got, err := FillReadme(in, testFacts("1.4.14"))
	if err != nil || got != in {
		t.Fatalf("want unchanged, got err %v\n%s", err, got)
	}
}

func TestFillReadmeErrors(t *testing.T) {
	cases := map[string]string{
		"unknown key": "<!-- BEGIN GENERATED:facts:nope --><!-- END GENERATED:facts:nope -->",
		"no end":      "<!-- BEGIN GENERATED:facts:version -->",
		"nested":      "<!-- BEGIN GENERATED:facts:version --><!-- BEGIN GENERATED:facts:verbs --><!-- END GENERATED:facts:version -->",
		"open marker": "<!-- BEGIN GENERATED:facts:version",
	}
	for name, in := range cases {
		if _, err := FillReadme(in, testFacts("1.5.0")); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestFillReadmeStrayEnd(t *testing.T) {
	for name, in := range map[string]string{
		"end only":         "x <!-- END GENERATED:facts:version --> y",
		"end before begin": "<!-- END GENERATED:facts:verbs -->\n<!-- BEGIN GENERATED:facts:version --><!-- END GENERATED:facts:version -->",
	} {
		if _, err := FillReadme(in, testFacts("1.5.0")); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
