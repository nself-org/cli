package load

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

const sampleTOC = `; Archive created
215; 1259 16389 TABLE public notes -
4100; 3256 16400 POLICY public notes notes_select -
4101; 0 0 ROW SECURITY public notes -
4102; 0 0 SCHEMA - public -`

func TestDropEntriesComment(t *testing.T) {
	got := dropEntries(sampleTOC, []string{"POLICY", "ROW SECURITY"})
	for _, want := range []string{";4100; 3256 16400 POLICY", ";4101; 0 0 ROW SECURITY"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, ";215;") || strings.Contains(got, ";4102;") {
		t.Fatal("table and schema entries must stay")
	}
	if dropEntries(sampleTOC, nil) != sampleTOC {
		t.Fatal("no kinds must change nothing")
	}
}

func TestSkipKindsRefusesOtherKinds(t *testing.T) {
	for body, wantErr := range map[string]bool{"POLICY\nROW SECURITY\n": false, "": false, "TABLE\n": true, "POLICY\nFUNCTION\n": true, "POLICY\n" + strings.Repeat(" ", 5000) + "TABLE\n": true} {
		dir := filepath.Join(t.TempDir(), "stage")
		w, err := portable.NewWriter(dir)
		if err != nil {
			t.Fatal(err)
		}
		if body != "" {
			if _, err = w.WriteFile(RestoreSkipMember, strings.NewReader(body)); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = w.Finish(portable.Manifest{}); err != nil {
			t.Fatal(err)
		}
		r, err := portable.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		_, err = SkipKinds(r)
		if (err != nil) != wantErr {
			t.Fatalf("%q: err=%v want error %v", body, err, wantErr)
		}
	}
}
