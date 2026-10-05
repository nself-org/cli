package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// TestErrorCodesPageMatchesRegistry keeps .github/wiki/error-codes.md honest:
// every registered code has exactly one table row, its Exit column equals the
// registry's Exit ("classified" for 0), and the row carries the anchor the
// code's DocsPath points at. A code added without a row fails here, and so does
// a row for a code that does not exist.
func TestErrorCodesPageMatchesRegistry(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "wiki", "error-codes.md"))
	if err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile(`(?m)^\| <a id="(e\d{3})"></a>(E\d{3}) \| ([^|]+?) \|`)
	rows := map[string][2]string{}
	for _, m := range row.FindAllStringSubmatch(string(raw), -1) {
		if _, dup := rows[m[2]]; dup {
			t.Errorf("%s has more than one row", m[2])
		}
		if strings.ToLower(m[2]) != m[1] {
			t.Errorf("%s row has anchor %q", m[2], m[1])
		}
		rows[m[2]] = [2]string{m[1], m[3]}
	}

	codes := make([]string, 0, len(errs.Registry))
	for c := range errs.Registry {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for _, c := range codes {
		e := errs.Registry[c]
		got, ok := rows[c]
		if !ok {
			t.Errorf("%s is registered but has no row in error-codes.md", c)
			continue
		}
		want := "classified"
		if e.Exit != 0 {
			want = strconv.Itoa(e.Exit)
		}
		if got[1] != want {
			t.Errorf("%s: page says exit %q, registry says %q", c, got[1], want)
		}
		if !strings.HasSuffix(e.DocsPath, "#"+got[0]) {
			t.Errorf("%s: DocsPath %q does not point at anchor #%s", c, e.DocsPath, got[0])
		}
	}
	for c := range rows {
		if _, ok := errs.Registry[c]; !ok {
			t.Errorf("error-codes.md documents %s, which is not registered", c)
		}
	}
}
