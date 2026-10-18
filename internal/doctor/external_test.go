package doctor

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// resetExternal empties the registry for one test and restores it after.
func resetExternal(t *testing.T) {
	t.Helper()
	externalMu.Lock()
	saved := externalFuncs
	externalFuncs = map[string]ExternalFunc{}
	externalMu.Unlock()
	t.Cleanup(func() {
		externalMu.Lock()
		externalFuncs = saved
		externalMu.Unlock()
	})
}

func rowProvider(names ...string) ExternalFunc {
	return func(context.Context, string) []CheckResult {
		var out []CheckResult
		for _, n := range names {
			out = append(out, CheckResult{Section: "x", Name: n, Status: "pass"})
		}
		return out
	}
}

func names(rs []CheckResult) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

func TestExternalChecks(t *testing.T) {
	t.Run("empty registry returns nothing", func(t *testing.T) {
		resetExternal(t)
		if got := ExternalChecks(context.Background(), "."); got != nil {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("providers run in name order", func(t *testing.T) {
		resetExternal(t)
		RegisterExternal("b", rowProvider("b1", "b2"))
		RegisterExternal("a", rowProvider("a1"))
		got := names(ExternalChecks(context.Background(), "."))
		if want := []string{"a1", "b1", "b2"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})
	t.Run("same name replaces, nil removes", func(t *testing.T) {
		resetExternal(t)
		RegisterExternal("a", rowProvider("old"))
		RegisterExternal("a", rowProvider("new"))
		if got := names(ExternalChecks(context.Background(), ".")); !reflect.DeepEqual(got, []string{"new"}) {
			t.Fatalf("replace: %v", got)
		}
		RegisterExternal("a", nil)
		if got := ExternalChecks(context.Background(), "."); got != nil {
			t.Fatalf("remove: %v", got)
		}
	})
	t.Run("provider gets ctx and project dir", func(t *testing.T) {
		resetExternal(t)
		var dir string
		RegisterExternal("a", func(_ context.Context, d string) []CheckResult { dir = d; return nil })
		ExternalChecks(context.Background(), "/proj")
		if dir != "/proj" {
			t.Fatalf("dir %q", dir)
		}
	})
	t.Run("panic becomes one warn row and later providers still run", func(t *testing.T) {
		resetExternal(t)
		RegisterExternal("a", func(context.Context, string) []CheckResult { panic("boom") })
		RegisterExternal("b", rowProvider("b1"))
		got := ExternalChecks(context.Background(), ".")
		if len(got) != 2 || got[0].Status != "warn" || !strings.Contains(got[0].Message, "boom") || got[1].Name != "b1" {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("cancelled ctx skips providers", func(t *testing.T) {
		resetExternal(t)
		RegisterExternal("a", rowProvider("a1"))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got := ExternalChecks(ctx, "."); got != nil {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("PluginHealthChecks appends external rows", func(t *testing.T) {
		resetExternal(t)
		RegisterExternal("a", rowProvider("EXT-ROW"))
		got := PluginHealthChecks(context.Background(), t.TempDir(), false)
		if last := got[len(got)-1]; last.Name != "EXT-ROW" {
			t.Fatalf("last row %+v of %d", last, len(got))
		}
	})
}
