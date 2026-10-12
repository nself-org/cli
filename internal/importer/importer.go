// Package importer coordinates portable imports without exposing source credentials.
// Purpose: shared source contract. Inputs: env-name flags and a started target.
// Outputs: a verified report. Constraints: sources own extraction only.
package importer

import (
	"context"
	"github.com/nself-org/cli/internal/config"
)

type FlagSpec struct {
	Name, Usage string
	EnvVarName  bool
}
type Input struct {
	Values map[string]string
	Env    func(string) string
	Target *config.Config
}
type Detection struct {
	Kind    string   `json:"kind"`
	Version string   `json:"version"`
	Notes   []string `json:"notes"`
	Schemas []string `json:"schemas"`
}
type Extracted struct{ Stage string }
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
