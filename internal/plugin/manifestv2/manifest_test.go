package manifestv2_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

func TestValidFixtures(t *testing.T) {
	for _, f := range fixtures(t, "v2") {
		if _, err := manifestv2.Load(f); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func TestForbiddenKeysE111(t *testing.T) {
	for _, key := range manifestv2.ForbiddenKeys {
		data := mutate(t, "v2/full.json", func(d map[string]any) { d[key] = "x" })
		_, err := manifestv2.Parse(data)
		wantCode(t, err, "E111", key)
	}
}

func TestManifestVersion3E114(t *testing.T) {
	for _, v := range []any{3, "2", 2.5, "v2", true, "0"} {
		data := mutate(t, "v2/full.json", func(d map[string]any) { d["manifest_version"] = v })
		_, err := manifestv2.Parse(data)
		wantCode(t, err, "E114")
	}
}

func TestCompatDriftE112(t *testing.T) {
	tamper := map[string]any{"status": "experimental", "tier": "pro", "binaryName": "nself-evil", "pluginType": "service",
		"isCommercial": true, "licenseType": "pro", "requires_license": true, "minNselfVersion": "9.9.9"}
	for key, val := range tamper {
		data := mutate(t, "v2/tenant.json", func(d map[string]any) { d[key] = val })
		_, err := manifestv2.Parse(data)
		wantCode(t, err, "E112", key)
	}
	data := mutate(t, "v2/tenant.json", func(d map[string]any) { delete(d, "tier") })
	_, err := manifestv2.Parse(data)
	wantCode(t, err, "E112", "tier")
	// A stale file is repaired by -compat, which rewrites only compatibility keys.
	stale := mutate(t, "v2/tenant.json", func(d map[string]any) { d["status"] = "planned"; d["maturity"] = "experimental" })
	fixed, err := manifestv2.ApplyCompat(stale)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifestv2.Parse(fixed); err != nil {
		t.Fatalf("regenerated file rejected: %v", err)
	}
}

func TestCommandVerbCollisionE113(t *testing.T) {
	for _, verb := range manifestv2.CoreVerbs {
		data := mutate(t, "v2/surface.json", func(d map[string]any) { d["commands"].(map[string]any)["command"] = verb })
		_, err := manifestv2.Parse(data)
		wantCode(t, err, "E113", verb)
	}
}

func TestLoadV1V2Equivalent(t *testing.T) {
	for _, f := range fixtures(t, "v1") {
		v2 := strings.Replace(f, "v1", "v2", 1)
		a, err := manifestv2.Load(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		b, err := manifestv2.Load(v2)
		if err != nil {
			t.Fatalf("%s: %v", v2, err)
		}
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s and %s load to different manifests:\n%+v\n%+v", f, v2, a, b)
		}
	}
}

func TestDeprecationOncePerProcess(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		var buf bytes.Buffer
		manifestv2.ResetWarn(&buf)
		for i := 0; i < 3; i++ {
			if _, err := manifestv2.Load("testdata/v1/tenant.json"); err != nil {
				t.Fatal(err)
			}
			if _, err := manifestv2.Load("testdata/v1/ci.json"); err != nil {
				t.Fatal(err)
			}
			if _, err := manifestv2.Load("testdata/v2/full.json"); err != nil {
				t.Fatal(err)
			}
		}
		lines := strings.Count(buf.String(), "\n")
		if v15 := t.Name()[strings.LastIndex(t.Name(), "/")+1:] == "v1.5"; v15 && lines != 1 {
			t.Fatalf("v1.5: want exactly one deprecation line, got %d: %q", lines, buf.String())
		} else if !v15 && lines != 0 {
			t.Fatalf("v1.4: want no output, got %q", buf.String())
		}
	})
}

func TestPostgresExtensionsAccessor(t *testing.T) {
	m, err := manifestv2.Load("testdata/v2/surface.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := manifestv2.PostgresExtensions(m); !reflect.DeepEqual(got, []string{"pgvector"}) {
		t.Fatalf("got %v", got)
	}
	full, _ := manifestv2.Load("testdata/v2/full.json")
	if got := manifestv2.PostgresExtensions(full); got == nil || len(got) != 0 {
		t.Fatalf("absent must be empty, got %#v", got)
	}
	if got := manifestv2.PostgresExtensions(nil); len(got) != 0 {
		t.Fatalf("nil manifest must be empty, got %v", got)
	}
}

func TestCoreVerbsAndRoundTrip(t *testing.T) {
	for _, f := range fixtures(t, "v2") {
		m, err := manifestv2.Load(f)
		if err != nil {
			t.Fatal(err)
		}
		out, err := manifestv2.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		again, err := manifestv2.Parse(out)
		if err != nil || !reflect.DeepEqual(m, again) {
			t.Errorf("%s does not round-trip: %v", f, err)
		}
	}
}

func FuzzLoad(f *testing.F) {
	for _, dir := range []string{"v1", "v2"} {
		files, err := filepath.Glob(filepath.Join("testdata", dir, "*.json"))
		if err != nil || len(files) == 0 {
			f.Fatalf("no fixtures in testdata/%s", dir)
		}
		for _, file := range files {
			b, err := os.ReadFile(file)
			if err != nil {
				f.Fatal(err)
			}
			f.Add(b)
		}
	}
	f.Add([]byte(`{"manifest_version":2}`))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := manifestv2.ParseQuiet(data)
		if err != nil {
			return
		}
		out, err := manifestv2.Marshal(m)
		if err != nil {
			t.Fatalf("a manifest that parsed must marshal: %v", err)
		}
		// A v1 file the released CLI accepts may lack keys v2 requires (the
		// normalizer is no stricter than v1.4.12); only a valid v2 manifest
		// must reparse.
		if manifestv2.Validate(m) != nil {
			return
		}
		if _, err := manifestv2.ParseQuiet(out); err != nil {
			t.Fatalf("canonical output of a valid manifest must parse: %v\n%s", err, out)
		}
	})
}
