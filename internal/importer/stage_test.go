package importer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer/load"
	"github.com/nself-org/cli/internal/portable"
)

// Purpose: exercise stage retention after extraction without a live target.
// Inputs: an empty valid bundle. Outputs: a stage passed to the load preflight.
// Constraints: all files stay in the test project; no database writes.
type stagedSource struct{ testSource }

func (*stagedSource) Extract(_ context.Context, _ Input, stage string) (Extracted, error) {
	w, err := portable.NewWriter(stage)
	if err != nil {
		return Extracted{}, err
	}
	_, err = w.Finish(portable.Manifest{})
	return Extracted{}, err
}

func TestImporterStageAfterLoadFailure(t *testing.T) {
	for _, tc := range []struct {
		name        string
		wrote, keep bool
	}{
		{name: "refused-preflight"},
		{name: "refused-preflight-keep", keep: true},
		{name: "written-failure", wrote: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			targetSeams(t)
			oldApply := applyBundle
			t.Cleanup(func() { applyBundle = oldApply })
			called := false
			applyBundle = func(_ context.Context, _ *config.Config, r *portable.Reader, opts load.Options) ([]portable.Exemption, error) {
				called = true
				if _, err := os.Stat(filepath.Join(r.Dir(), portable.ManifestName)); err != nil {
					t.Fatal(err)
				}
				opts.Progress.Wrote = tc.wrote
				if tc.wrote {
					return nil, errs.New("E521", "restore failed")
				}
				return nil, load.StoragePreflight(portable.Manifest{Storage: portable.Storage{Objects: []portable.Object{{Bucket: "bucket", Key: "key"}}}}, nil, false)
			}
			source := &stagedSource{testSource{name: "test-stage-" + tc.name}}
			Register(source)
			dir := t.TempDir()
			in := Input{Values: map[string]string{"from-env": "TEST_SOURCE"}, Env: func(string) string { return "in-memory" }}
			report, err := Run(context.Background(), &config.Config{}, source, in, RunOptions{ProjectDir: dir, KeepStage: tc.keep})
			code := "E529"
			if tc.wrote {
				code = "E521"
			}
			var cli *errs.CLIError
			if !called || !errors.As(err, &cli) || cli.Code != code || report.Wrote != tc.wrote {
				t.Fatalf("load failure: called=%v report=%+v err=%v", called, report, err)
			}
			root := filepath.Join(dir, ".nself", "import", report.RunID)
			data, err := os.ReadFile(filepath.Join(root, "report.json"))
			if err != nil {
				t.Fatal(err)
			}
			var saved Report
			if err = json.Unmarshal(data, &saved); err != nil || saved.Wrote != tc.wrote || saved.Result != "failed" {
				t.Fatalf("durable report: %+v err=%v", saved, err)
			}
			_, err = os.Stat(filepath.Join(root, "stage"))
			if tc.wrote || tc.keep {
				if err != nil {
					t.Fatalf("stage must remain: %v", err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("unwritten failed import retained stage: %v", err)
			}
		})
	}
}
