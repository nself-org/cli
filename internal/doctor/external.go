package doctor

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Purpose: the registration hook that lets the composition root add checks to
// `nself doctor --deep` without internal/doctor importing internal/plugin
// (P7-CI-53, Epic D27). The cmd layer registers a provider (the installed
// plugins' own `doctor --json`); PluginHealthChecks calls ExternalChecks at
// its end.
// Inputs: a name and a provider func at registration; a context and project
// dir at run time.
// Outputs: []CheckResult from every registered provider, in name order.
// Constraints: deep mode only (PluginHealthChecks is not in the base doctor);
// a provider owns its own per-item timeout, ctx is only the outer bound; a
// panicking provider becomes one warn row, never a crash.

// ExternalFunc is one provider of extra deep-doctor checks.
type ExternalFunc func(ctx context.Context, projectDir string) []CheckResult

var (
	externalMu    sync.Mutex
	externalFuncs = map[string]ExternalFunc{}
)

// RegisterExternal adds (or replaces) the provider registered under name.
func RegisterExternal(name string, fn ExternalFunc) {
	externalMu.Lock()
	defer externalMu.Unlock()
	if fn == nil {
		delete(externalFuncs, name)
		return
	}
	externalFuncs[name] = fn
}

// ExternalChecks runs every registered provider in name order and returns
// their results concatenated. It returns nil when none is registered.
func ExternalChecks(ctx context.Context, projectDir string) []CheckResult {
	externalMu.Lock()
	names := make([]string, 0, len(externalFuncs))
	for n := range externalFuncs {
		names = append(names, n)
	}
	sort.Strings(names)
	fns := make([]ExternalFunc, len(names))
	for i, n := range names {
		fns[i] = externalFuncs[n]
	}
	externalMu.Unlock()

	var out []CheckResult
	for i, fn := range fns {
		if ctx.Err() != nil {
			break
		}
		out = append(out, runExternal(ctx, names[i], fn, projectDir)...)
	}
	return out
}

// runExternal calls one provider, turning a panic into a warn row.
func runExternal(ctx context.Context, name string, fn ExternalFunc, projectDir string) (res []CheckResult) {
	defer func() {
		if r := recover(); r != nil {
			res = []CheckResult{{
				Section: "plugins",
				Name:    "EXTERNAL-" + name,
				Status:  "warn",
				Message: fmt.Sprintf("external doctor %q failed: %v", name, r),
			}}
		}
	}()
	return fn(ctx, projectDir)
}
