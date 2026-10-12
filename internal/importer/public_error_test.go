package importer

import (
	"context"
	"errors"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/portable"
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

func TestPlanObjects(t *testing.T) {
	ctx := context.Background()
	objs := []portable.Object{{Bucket: "b", Key: "k"}}
	if err := planObjects(ctx, &config.Config{}, Detection{}, false); err != nil {
		t.Fatalf("a bundle without objects needs no store: %v", err)
	}
	if err := planObjects(ctx, &config.Config{}, Detection{Objects: objs}, true); err != nil {
		t.Fatalf("--skip-storage loads no object: %v", err)
	}
	err := planObjects(ctx, &config.Config{}, Detection{Objects: objs}, false)
	var cli *errs.CLIError
	if !errors.As(err, &cli) || cli.Code != "E529" {
		t.Fatalf("a plan must refuse objects with no object store like the real run (E529): %v", err)
	}
}
