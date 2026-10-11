package lock

// lock_test.go — tests for the project lock types (P7-LIVE-09): canonical
// bytes, strict parsing, schema conformance, the read API, Compare and Pin.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const digA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const digB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func sample() Lock {
	l := New()
	l.CLI = "1.5"
	l.Images = map[string]string{"redis": "docker.io/library/redis:7-alpine@" + digA, "hasura": "hasura/graphql-engine:v2.44.0@" + digB}
	l.Unpinned = map[string]string{"minio": "docker.io/pgsty/minio:latest"}
	l.Plugins = map[string]PluginPin{"ai": {Version: "2.0.0", SHA256: strings.Repeat("c", 64), Tier: TierLicensed}, "cron": {Version: "1.1.0", Tier: TierFree}}
	l.Bundles = []string{"nclaw", "ai"}
	return l
}

func schema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "schemas", "lockfile.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("lock.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("lock.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func asJSON(t *testing.T, l Lock) any {
	t.Helper()
	l.normalize()
	raw, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLockMarshalCanonical(t *testing.T) {
	a, err := sample().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := sample().Marshal()
	if string(a) != string(b) {
		t.Fatal("two marshals of one lock differ")
	}
	s := string(a)
	if !strings.HasPrefix(s, Header+"\nschema_version: 1\ncli: \"1.5\"\nimages:\n") {
		t.Fatalf("unexpected start:\n%s", s)
	}
	if strings.Index(s, "  hasura:") > strings.Index(s, "  redis:") {
		t.Fatalf("images are not sorted:\n%s", s)
	}
	if strings.Index(s, "- ai") > strings.Index(s, "- nclaw") {
		t.Fatalf("bundles are not sorted:\n%s", s)
	}
	back, err := Parse(a)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := back.Marshal()
	if string(c) != s {
		t.Fatal("parse then marshal is not the identity")
	}
	for _, key := range []string{"installed_at", "generated_at", "updated_at"} {
		if strings.Contains(s, key) {
			t.Fatalf("a lock carries no timestamps:\n%s", s)
		}
	}
}

func TestLockSchemaConformance(t *testing.T) {
	sc := schema(t)
	if err := sc.Validate(asJSON(t, sample())); err != nil {
		t.Fatalf("sample lock fails the schema: %v", err)
	}
	if err := sc.Validate(asJSON(t, New())); err != nil {
		t.Fatalf("empty lock fails the schema: %v", err)
	}
	bad := map[string]func(*Lock){
		"tag in images":    func(l *Lock) { l.Images["redis"] = "redis:7" },
		"digest in unpin":  func(l *Lock) { l.Unpinned["minio"] = "minio@" + digA },
		"bad cli":          func(l *Lock) { l.CLI = "1.5.2" },
		"bad tier":         func(l *Lock) { l.Plugins["ai"] = PluginPin{Version: "1", Tier: "pro"} },
		"bad sha":          func(l *Lock) { l.Plugins["ai"] = PluginPin{Version: "1", SHA256: "XYZ", Tier: TierFree} },
		"bad service name": func(l *Lock) { l.Images["Bad Name"] = "x@" + digA },
		"dup bundle":       func(l *Lock) { l.Bundles = []string{"a", "a"} },
	}
	for name, mut := range bad {
		t.Run(name, func(t *testing.T) {
			l := sample()
			mut(&l)
			if err := schema(t).Validate(asJSON(t, l)); err == nil {
				t.Error("the schema accepted it")
			}
			if _, err := l.Marshal(); err == nil {
				t.Error("Marshal accepted it")
			}
		})
	}
}

func TestLockParseRejects(t *testing.T) {
	good, _ := sample().Marshal()
	cases := map[string]string{
		"empty":       "",
		"unknown key": strings.Replace(string(good), Trailer, "ci_toolchain: x\n"+Trailer, 1),
		"version 2":   strings.Replace(string(good), "schema_version: 1", "schema_version: 2", 1),
		"not yaml":    "images: [",
	}
	for name, in := range cases {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: Parse accepted it", name)
		}
	}
}

// TestLockTruncatedNeverParses: a lock cut at any byte, or at any line boundary,
// is an error. Without the trailer a prefix ending on a line boundary parses as
// a valid lock with fewer images (the silent image move this file prevents).
func TestLockTruncatedNeverParses(t *testing.T) {
	good, err := sample().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(good); err != nil {
		t.Fatalf("the full file must parse: %v", err)
	}
	ran := 0
	// The last byte is the final newline: without it the file still ends with the
	// whole trailer and holds every pin, so it is complete, not truncated.
	for i := 0; i < len(good)-1; i++ {
		ran++
		if _, err := Parse(good[:i]); err == nil {
			t.Fatalf("a lock cut at byte %d of %d parsed as valid:\n%s", i, len(good), good[:i])
		}
	}
	lines := strings.SplitAfter(string(good), "\n")
	for n := 1; n < len(lines)-1; n++ {
		ran++
		if _, err := Parse([]byte(strings.Join(lines[:n], ""))); err == nil {
			t.Fatalf("a lock cut after line %d parsed as valid", n)
		}
	}
	if ran == 0 {
		t.Fatal("no truncation was tried")
	}
	if !strings.HasSuffix(string(good), Trailer+"\n") {
		t.Fatalf("Marshal does not end with the trailer: %q", good[len(good)-40:])
	}
}

func TestLockLoadAndPluginVersion(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); !errors.Is(err, ErrNotFound) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing lock: %v", err)
	}
	raw, _ := sample().Marshal()
	if err := os.WriteFile(Path(dir), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := l.PluginVersion("ai"); !ok || v != "2.0.0" {
		t.Fatalf("PluginVersion(ai) = %q %v", v, ok)
	}
	if _, ok := l.PluginVersion("ci"); ok {
		t.Fatal("PluginVersion found a plugin that is not pinned")
	}
	var nilLock *Lock
	if _, ok := nilLock.PluginVersion("ai"); ok {
		t.Fatal("nil lock has no plugins")
	}
	if got := l.Pins(); len(got) != 3 || got["minio"] == "" || got["redis"] == "" {
		t.Fatalf("Pins = %v", got)
	}
	if err := os.WriteFile(Path(dir), []byte("images: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), FileName) {
		t.Fatalf("damaged lock: %v", err)
	}
}

func TestLockCompare(t *testing.T) {
	base := sample()
	if d := Compare(base, base); len(d) != 0 {
		t.Fatalf("equal locks differ: %v", d)
	}
	cases := map[string]struct {
		mut  func(*Lock)
		want string
	}{
		"image digest":   {func(l *Lock) { l.Images["redis"] = "docker.io/library/redis:7-alpine@" + digB }, "image redis"},
		"image missing":  {func(l *Lock) { delete(l.Images, "redis") }, "image redis: lock has docker.io/library/redis:7-alpine@" + digA + ", stack has <absent>"},
		"image extra":    {func(l *Lock) { l.Images["nginx"] = "nginx@" + digA }, "image nginx"},
		"unpinned moved": {func(l *Lock) { l.Unpinned["minio"] = "docker.io/pgsty/minio:v2" }, "image minio"},
		"plugin version": {func(l *Lock) {
			l.Plugins["ai"] = PluginPin{Version: "2.1.0", SHA256: strings.Repeat("c", 64), Tier: TierLicensed}
		}, "plugin ai"},
		"plugin sha": {func(l *Lock) {
			l.Plugins["ai"] = PluginPin{Version: "2.0.0", SHA256: strings.Repeat("d", 64), Tier: TierLicensed}
		}, "plugin ai sha256"},
		"plugin gone": {func(l *Lock) { delete(l.Plugins, "cron") }, "plugin cron"},
		"cli minor":   {func(l *Lock) { l.CLI = "1.6" }, "cli: lock has 1.5, stack has 1.6"},
		"bundle":      {func(l *Lock) { l.Bundles = []string{"ai"} }, "bundle nclaw"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			act := sample()
			c.mut(&act)
			d := Compare(base, act)
			if len(d) != 1 || !strings.Contains(d[0].String(), c.want) {
				t.Fatalf("got %v, want one mismatch containing %q", d, c.want)
			}
		})
	}
	// An unknown checksum or CLI minor on either side is not compared.
	act := sample()
	act.CLI = ""
	act.Plugins["ai"] = PluginPin{Version: "2.0.0", Tier: TierLicensed}
	if d := Compare(base, act); len(d) != 0 {
		t.Fatalf("unknown values compared: %v", d)
	}
}

func TestLockPin(t *testing.T) {
	d := "sha256:" + strings.Repeat("e", 64)
	r := Resolver{
		LocalDigest: func(ref string) string {
			if ref == "redis:7-alpine" {
				return d
			}
			return ""
		},
		CLILocked: func(ref string) (string, bool) {
			if ref == "nginx:alpine" {
				return "docker.io/library/nginx:alpine@" + digA, true
			}
			return "", false
		},
	}
	images, unpinned := Pin(map[string]string{
		"redis": "redis:7-alpine", "nginx": "nginx:alpine", "minio": "pgsty/minio:latest",
		"auth": "nhost/hasura-auth:0.36.0@" + digB, "odd": "x:1@md5:abc", "empty": " ",
	}, r)
	want := map[string]string{"redis": "redis:7-alpine@" + d, "nginx": "docker.io/library/nginx:alpine@" + digA, "auth": "nhost/hasura-auth:0.36.0@" + digB}
	for k, v := range want {
		if images[k] != v {
			t.Errorf("images[%s] = %q, want %q", k, images[k], v)
		}
	}
	if len(images) != 3 || unpinned["minio"] != "pgsty/minio:latest" || unpinned["odd"] != "x:1" || len(unpinned) != 2 {
		t.Errorf("images %v unpinned %v", images, unpinned)
	}
	// A malformed local digest is ignored.
	bad := Resolver{LocalDigest: func(string) string { return "sha256:short" }}
	if im, un := Pin(map[string]string{"redis": "redis:7"}, bad); len(im) != 0 || un["redis"] != "redis:7" {
		t.Errorf("bad digest: %v %v", im, un)
	}
}

func TestLockComposeImages(t *testing.T) {
	got, err := ComposeImages([]byte("# GENERATED\nservices:\n  a:\n    image: x:1\n  b:\n    build: .\n  c:\n    image: y:2@" + digA + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["a"] != "x:1" || got["c"] != "y:2@"+digA {
		t.Fatalf("got %v", got)
	}
	if _, err := ComposeImages([]byte("services: [")); err == nil {
		t.Fatal("broken YAML accepted")
	}
}

// TestLockCompareStrict: --frozen treats an empty cli or sha256 on one side as
// a difference when the other recorded it; the lenient Compare does not.
func TestLockCompareStrict(t *testing.T) {
	base := sample()
	blank := sample()
	blank.CLI = ""
	pin := blank.Plugins["ai"]
	pin.SHA256 = ""
	blank.Plugins["ai"] = pin
	for name, pair := range map[string][2]Lock{"lock blanked": {blank, base}, "stack blanked": {base, blank}} {
		if d := Compare(pair[0], pair[1]); len(d) != 0 {
			t.Errorf("%s: lenient Compare must skip empty values, got %v", name, d)
		}
		d := CompareStrict(pair[0], pair[1])
		if len(d) != 2 || d[0].Kind != KindCLI || !strings.Contains(d[1].String(), "plugin ai sha256") {
			t.Errorf("%s: CompareStrict want cli and ai sha256 mismatches, got %v", name, d)
		}
	}
	both := sample()
	both.CLI = ""
	if d := CompareStrict(both, both); len(d) != 0 {
		t.Errorf("both sides empty is equal: %v", d)
	}
}

// TestLockParseSchemaVersionAndDocuments: a lock must state schema_version
// (not defaulted) and hold exactly one YAML document (adversarial R6).
func TestLockParseSchemaVersionAndDocuments(t *testing.T) {
	l := New()
	good, err := l.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(good); err != nil {
		t.Fatalf("a valid lock must parse: %v", err)
	}
	noVersion := strings.Replace(string(good), "schema_version: 1\n", "", 1)
	if noVersion == string(good) {
		t.Fatal("fixture has no schema_version line")
	}
	if _, err := Parse([]byte(noVersion)); err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("a missing schema_version must be rejected, got %v", err)
	}
	zero := strings.Replace(string(good), "schema_version: 1\n", "schema_version: 0\n", 1)
	if _, err := Parse([]byte(zero)); err == nil {
		t.Fatal("schema_version 0 must be rejected")
	}
	withVar := strings.Replace(string(good), "unpinned: {}", "unpinned:\n  postgres: $PGIMG", 1)
	if withVar == string(good) {
		t.Fatal("fixture has no empty unpinned map")
	}
	if _, err := Parse([]byte(withVar)); err == nil {
		t.Fatal("an image containing a variable must not parse as a lock entry")
	}
	body := strings.TrimSuffix(string(good), Trailer+"\n")
	two := body + "---\nimages:\n  hasura: evil/hasura:1@sha256:" + strings.Repeat("a", 64) + "\n" + Trailer + "\n"
	if _, err := Parse([]byte(two)); err == nil || !strings.Contains(err.Error(), "more than one") {
		t.Fatalf("a second YAML document must be rejected, got %v", err)
	}
}

// TestMergeImagesUnverifiable: variables, extends, build and include on a
// CLI-owned service cannot be read as an image, so they are reported (R1, R2).
func TestMergeImagesUnverifiable(t *testing.T) {
	owned := func(s string) bool { return s == "hasura" || s == "postgres" }
	cases := []struct{ name, yaml, want string }{
		{"variable", "services:\n  postgres:\n    image: ${PGIMG:-postgres:16}\n", "contains a variable"},
		{"bare variable", "services:\n  postgres:\n    image: $PGIMG\n", "contains a variable"},
		{"extends", "services:\n  hasura:\n    extends:\n      file: x.yml\n      service: y\n", "extends"},
		{"build", "services:\n  hasura:\n    build: ./evil\n", "build"},
		{"include", "include:\n  - other.yml\nservices:\n  custom:\n    image: mine/app:1\n", "include"},
	}
	for _, c := range cases {
		_, bad, err := MergeImages([]SourceFile{{Name: "o.yml", Data: []byte(c.yaml)}}, owned, true)
		if err != nil || len(bad) != 1 || !strings.Contains(bad[0], c.want) || !strings.Contains(bad[0], "cannot be verified") {
			t.Errorf("%s: bad=%v err=%v, want one entry naming %q", c.name, bad, err, c.want)
		}
	}
	// A service the CLI does not own may use all of them; plain images are fine.
	_, bad, err := MergeImages([]SourceFile{{Name: "o.yml", Data: []byte("services:\n  custom:\n    build: .\n    image: $X\n  hasura:\n    image: a/b:1\n")}}, owned, true)
	if err != nil || len(bad) != 0 {
		t.Errorf("user services and literal images must pass: %v %v", bad, err)
	}
}

// TestMergeDetailedServices: the unverifiable CLI-owned services are named; an
// include or a user-owned service adds none.
func TestMergeDetailedServices(t *testing.T) {
	owned := func(s string) bool { return s == "hasura" || s == "postgres" }
	yml := "include:\n  - x.yml\nservices:\n  hasura:\n    image: $H\n  postgres:\n    extends:\n      service: p\n  custom:\n    build: .\n"
	m, err := MergeDetailed([]SourceFile{{Name: "o.yml", Data: []byte(yml)}}, owned, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.Services, ","); got != "hasura,postgres" {
		t.Fatalf("services = %q, want hasura,postgres", got)
	}
	if len(m.Unverifiable) != 3 {
		t.Fatalf("want include + two services, got %v", m.Unverifiable)
	}
}

// TestPruneMirror: an entry stays only while its digest equals the rendered
// image's digest; with none left the output is nil.
func TestPruneMirror(t *testing.T) {
	d1 := "sha256:" + strings.Repeat("1", 64)
	d2 := "sha256:" + strings.Repeat("2", 64)
	rendered := map[string]string{"a": "r/a:1@" + d1, "b": "r/b:1@" + d1, "c": "r/c:1"}
	file := MirrorHeader + "services:\n    a:\n        image: m/a@" + d1 + "\n    b:\n        image: m/b@" + d2 + "\n    c:\n        image: m/c@" + d1 + "\n    z:\n        image: m/z@" + d1 + "\n"
	out, dropped, err := PruneMirror([]byte(file), rendered)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(dropped, ",") != "b,c,z" {
		t.Fatalf("dropped = %v, want b,c,z", dropped)
	}
	want := MirrorHeader + "services:\n    a:\n        image: m/a@" + d1 + "\n"
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
	same, dropped, err := PruneMirror(out, rendered)
	if err != nil || len(dropped) != 0 || string(same) != want {
		t.Fatalf("a clean file must come back unchanged: %v %v", dropped, err)
	}
	if out, dropped, _ := PruneMirror([]byte(file), map[string]string{}); out != nil || len(dropped) != 4 {
		t.Fatalf("nothing left must give nil bytes: %q %v", out, dropped)
	}
	if _, _, err := PruneMirror([]byte("services: [\n"), rendered); err == nil {
		t.Fatal("unparseable input must be an error")
	}
}
