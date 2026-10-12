package commands

import (
	"context"
	"encoding/json"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/importer"
	"os"
	"testing"
)

func TestDBImportSourcesCommands(t *testing.T) {
	found := false
	for _, cmd := range migrateCmd.Commands() {
		if cmd.Name() == "postgres" {
			found = true
			for _, flag := range []string{"from-env", "plan", "keep-stage", "skip-storage", "json"} {
				if cmd.Flags().Lookup(flag) == nil {
					t.Fatal("missing flag " + flag)
				}
			}
			if cmd.RunE == nil {
				t.Fatal("source command does nothing")
			}
		}
	}
	if !found {
		t.Fatal("postgres source missing")
	}
	if _, ok := jsonDataTypes["db import postgres"].(importer.Report); !ok {
		t.Fatal("report type missing")
	}
}
func TestDBImportPlanJSON(t *testing.T) {
	old := importRun
	defer func() { importRun = old }()
	called := false
	importRun = func(_ context.Context, _ *config.Config, s importer.Source, in importer.Input, opts importer.RunOptions) (importer.Report, error) {
		called = true
		if !opts.Plan || in.Values["from-env"] != "SRC_URL" || s.Name() != "postgres" {
			t.Fatal("wrong inputs")
		}
		return importer.Report{Source: s.Name(), Result: "pass", Steps: []string{"extract snapshot"}}, nil
	}
	var source importer.Source
	for _, s := range importer.Sources() {
		if s.Name() == "postgres" {
			source = s
		}
	}
	cmd := importSourceCommand(source)
	cmd.SetContext(context.Background())
	_ = cmd.Flags().Set("plan", "true")
	_ = cmd.Flags().Set("json", "true")
	_ = cmd.Flags().Set("from-env", "SRC_URL")
	out, err := captureHasuraOutput(func() error { return cmd.RunE(cmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Command string          `json:"command"`
		Data    importer.Report `json:"data"`
	}
	if json.Unmarshal([]byte(out), &doc) != nil || doc.Command != "db import postgres" || doc.Data.Result != "pass" || !called {
		t.Fatalf("invalid plan JSON: %s", out)
	}
	if _, e := os.Stat(".nself/import"); e == nil {
		t.Fatal("plan created a stage")
	}
}
