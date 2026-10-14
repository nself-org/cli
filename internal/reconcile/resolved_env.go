package reconcile

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	nbuild "github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
)

// resolveBuildEnv chooses the cascade once, with config.Load's process-first
// precedence and the pending .env bytes standing in for the base file.
// It reads no other cascade layer: those layers depend on this selection.
func resolveBuildEnv(dir string, overlay map[string][]byte) string {
	if data, ok := overlay[".env"]; ok && os.Getenv("ENV") == "" {
		if values, err := godotenv.UnmarshalBytes(data); err == nil && values["ENV"] != "" {
			return values["ENV"]
		}
		return "dev"
	}
	env, _ := config.ResolveEnv(dir)
	return env
}

// buildWithEnv passes the selected cascade to a render and restores ENV on
// return. The loader overloads ENV from files, so its result cannot recover
// the selector. Apply reuses this selector through confirmation and writing.
func buildWithEnv(dir string, opts nbuild.BuildOptions, env string, build func(string, nbuild.BuildOptions) (*nbuild.BuildResult, error)) (*nbuild.BuildResult, error) {
	previous, present := os.LookupEnv("ENV")
	if err := os.Setenv("ENV", env); err != nil {
		return nil, fmt.Errorf("setting build environment: %w", err)
	}
	defer func() {
		if present {
			_ = os.Setenv("ENV", previous)
		} else {
			_ = os.Unsetenv("ENV")
		}
	}()
	return build(dir, opts)
}

// retainProdEnv keeps the existing confirmation for a prod ENV loaded from
// cascade files even when the selector and explicit prior ENV are dev.
func retainProdEnv(p *Plan, loaded string) {
	if EnvClass(p.EnvBefore) != ClassProd && EnvClass(loaded) == ClassProd {
		p.EnvBefore = loaded
	}
}
