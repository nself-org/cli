package importer

import (
	"errors"
	"github.com/nself-org/cli/internal/errs"
	"strings"
	"testing"
)

func TestPublicErrorKeepsPortableBundleCodes(t *testing.T) {
	for _, code := range []string{"E515", "E516"} {
		got := publicError(errs.New(code, `bundle file "db/data.dump" checksum mismatch`), "E517", "source detection failed")
		var cli *errs.CLIError
		if !asCLI(got, &cli) || cli.Code != code || !strings.Contains(cli.What, "db/data.dump") {
			t.Fatalf("%s must keep its code and the member path: %v", code, got)
		}
	}
	for _, what := range []string{"bundle file \"a\x1b[31mb\" missing", "bundle file \"a‮b\" missing", strings.Repeat("x", 301), "bad \xff utf8"} {
		var cli *errs.CLIError
		got := publicError(errs.New("E516", what), "E517", "source detection failed")
		if !asCLI(got, &cli) || cli.Code != "E516" || cli.What != "portable bundle failed its check" {
			t.Fatalf("unsafe text %q must be replaced: %v", what, got)
		}
	}
	var cli *errs.CLIError
	if got := publicError(errs.New("E523", "anything"), "E517", "source detection failed"); !asCLI(got, &cli) || cli.Code != "E517" {
		t.Fatalf("other codes keep the generic mapping: %v", got)
	}
}

func asCLI(err error, target **errs.CLIError) bool { return errors.As(err, target) }
