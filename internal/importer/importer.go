// Package importer coordinates portable imports without exposing source credentials.
// Purpose: shared source contract. Inputs: env-name flags and a started target.
// Outputs: a verified report. Constraints: sources own extraction only.
package importer

import (
	"context"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/portable"
)

type FlagSpec struct {
	Name, Usage string
	EnvVarName  bool
}
type Input struct {
	SkipStorage bool
	Values      map[string]string
	Env         func(string) string
	Target      *config.Config
}
type Detection struct {
	Kind    string   `json:"kind"`
	Version string   `json:"version"`
	Notes   []string `json:"notes"`
	Schemas []string `json:"schemas"`
	// SourceUsers counts the source auth users seen at detection; --plan uses it to run the
	// same auth.users preflight as the real run.
	SourceUsers int `json:"source_users,omitempty"`
	// Objects lists the bucket/key pairs a bundle source will load, for the --plan object-store check.
	// It never reaches the report: keys can be user file names.
	Objects []portable.Object `json:"-"`
}

// Extracted names the written bundle. UntranslatedPolicies lists the source access rules the
// source could not express as Hasura permissions; they reach the report unchanged.
type Extracted struct {
	Stage                string
	UntranslatedPolicies []Policy
}
type Source interface {
	Name() string
	Flags() []FlagSpec
	Detect(context.Context, Input) (Detection, error)
	Extract(context.Context, Input, string) (Extracted, error)
}
type RunOptions struct {
	ProjectDir                   string
	Plan, KeepStage, SkipStorage bool
}
