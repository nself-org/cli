package importer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer/scratch"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScratchRejectsInvalidInput(t *testing.T) {
	if _, err := scratch.Build(context.Background(), &config.Config{}, "", []scratch.CopyFile{{Schema: "public;drop", Table: "notes"}}, t.TempDir()); err == nil {
		t.Fatal("invalid scratch name accepted")
	}
}

type testSource struct {
	name    string
	extract bool
	failure error
	users   int
}

func (s *testSource) Name() string    { return s.name }
func (*testSource) Flags() []FlagSpec { return []FlagSpec{{Name: "from-env", EnvVarName: true}} }
func (s *testSource) Detect(_ context.Context, in Input) (Detection, error) {
	_, err := in.Secret("from-env")
	if err != nil {
		return Detection{}, err
	}
	return Detection{Kind: s.name, Version: "16", Schemas: []string{"public"}, SourceUsers: s.users}, s.failure
}
func (s *testSource) Extract(_ context.Context, _ Input, _ string) (Extracted, error) {
	s.extract = true
	return Extracted{}, s.failure
}
func targetSeams(t *testing.T) {
	t.Helper()
	oldStarted, oldChecks, oldLock := started, targetChecks, lockTarget
	started = func(context.Context, string) error { return nil }
	targetChecks = func(context.Context, *config.Config, Detection) error { return nil }
	lockTarget = func(context.Context, *config.Config) (func(), error) { return func() {}, nil }
	t.Cleanup(func() { started, targetChecks, lockTarget = oldStarted, oldChecks, oldLock })
}
func TestImporterPlanNoWrites(t *testing.T) {
	targetSeams(t)
	source := &testSource{name: "test-plan"}
	Register(source)
	dir := t.TempDir()
	report, err := Run(context.Background(), &config.Config{}, source, Input{Values: map[string]string{"from-env": "TEST_SOURCE"}, Env: func(string) string { return "in-memory" }}, RunOptions{ProjectDir: dir, Plan: true})
	if err != nil || report.Result != "pass" || len(report.Steps) == 0 || source.extract {
		t.Fatalf("plan: %+v %v", report, err)
	}
	if _, err = os.Stat(filepath.Join(dir, ".nself")); !os.IsNotExist(err) {
		t.Fatal("plan wrote state")
	}
}
func TestImporterRedaction(t *testing.T) {
	targetSeams(t)
	planted := fixtureSecret(t)
	source := &testSource{name: "test-redaction", failure: errors.New("postgresql://alice:" + planted + "@localhost/db")}
	Register(source)
	dir := t.TempDir()
	in := Input{Values: map[string]string{"from-env": "TEST_SOURCE"}, Env: func(string) string { return "postgresql://alice:" + planted + "@localhost/db" }}
	report, err := Run(context.Background(), &config.Config{}, source, in, RunOptions{ProjectDir: dir})
	if err == nil {
		t.Fatal("expected unreachable source")
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw)+err.Error(), planted) {
		t.Fatal("credential leaked")
	}
	source.failure = nil
	source2 := &extractFailureSource{testSource: *source, secret: planted}
	source2.name = "test-extract-redaction"
	Register(source2)
	report, err = Run(context.Background(), &config.Config{}, source2, in, RunOptions{ProjectDir: dir})
	if err == nil {
		t.Fatal("expected extraction failure")
	}
	if strings.Contains(err.Error(), planted) {
		t.Fatal("error leaked secret")
	}
	count := 0
	if e := filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			count++
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if strings.Contains(string(b), planted) {
				t.Fatal("stage/report leaked secret")
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if count == 0 || report.RunID == "" {
		t.Fatal("redaction search ran on no files")
	}
	t.Logf("redaction files read=%d", count)
}

type extractFailureSource struct {
	testSource
	secret string
}

func (s *extractFailureSource) Extract(context.Context, Input, string) (Extracted, error) {
	return Extracted{}, errors.New(s.secret)
}
func TestImporterRegistry(t *testing.T) {
	before := len(RegistryErrors())
	source := &testSource{name: "test-registry"}
	Register(source)
	Register(source)
	Register(nil)
	var nilSource *testSource
	Register(nilSource)
	Register(&testSource{name: "bad name"})
	if len(RegistryErrors()) != before+4 {
		t.Fatal("registration failures not retained")
	}
	sources := Sources()
	for i := 1; i < len(sources); i++ {
		if sources[i-1].Name() >= sources[i].Name() {
			t.Fatal("unsorted sources")
		}
	}
	if _, err := Run(context.Background(), nil, &testSource{name: "unregistered"}, Input{}, RunOptions{}); err == nil {
		t.Fatal("unregistered source accepted")
	}
}
func TestImporterLockHeldRefusesBeforeExtract(t *testing.T) {
	targetSeams(t)
	lockTarget = func(context.Context, *config.Config) (func(), error) {
		return nil, errs.New("E519", "another import is running against this target")
	}
	source := &testSource{name: "test-lock"}
	Register(source)
	_, err := Run(context.Background(), &config.Config{}, source, Input{}, RunOptions{ProjectDir: t.TempDir()})
	if err == nil || source.extract || !strings.Contains(err.Error(), "another import is running") {
		t.Fatalf("a held lock must refuse before extraction: %v", err)
	}
}
func TestImporterPreflightRefusesWrites(t *testing.T) {
	targetSeams(t)
	started = func(context.Context, string) error { return errs.New("E521", "not started") }
	source := &testSource{name: "test-preflight"}
	Register(source)
	dir := t.TempDir()
	if _, err := Run(context.Background(), &config.Config{}, source, Input{}, RunOptions{ProjectDir: dir}); err == nil || source.extract {
		t.Fatal("preflight not enforced")
	}
}

// fixtureSecret returns FIXTURE_PASSWORD when set, else a random value; tests never carry literal credentials.
func fixtureSecret(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("FIXTURE_PASSWORD"); v != "" {
		return v
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func TestWithReportHint(t *testing.T) {
	for _, code := range []string{"E521", "E522", "E523"} {
		for _, wrote := range []bool{false, true} {
			err := withReportHint(errs.New(code, "x"), "run1", wrote)
			var cli *errs.CLIError
			if !errors.As(err, &cli) || !strings.Contains(cli.Fix, filepath.Join(".nself", "import", "run1", "report.json")) {
				t.Fatalf("%s: fix missing path: %+v", code, err)
			}
			reset := strings.Contains(cli.Fix, "nself db reset --yes")
			untouched := strings.Contains(cli.Fix, "Nothing was written")
			if reset != wrote || untouched == wrote {
				t.Fatalf("%s wrote=%v: reset=%v untouched=%v: %s", code, wrote, reset, untouched, cli.Fix)
			}
		}
	}
	other := errs.New("E517", "x")
	if got := withReportHint(other, "run1", true); got != error(other) {
		t.Fatal("other codes must pass through unchanged")
	}
	if withReportHint(nil, "run1", true) != nil {
		t.Fatal("nil must stay nil")
	}
}

func TestPublicErrorKeepsSafeMessages(t *testing.T) {
	for _, msg := range []string{"source requires an environment variable name", "source environment variable is empty", "source connection configuration invalid"} {
		got := publicError(errs.New("E517", msg), "E517", "source detection failed")
		var cli *errs.CLIError
		if !errors.As(got, &cli) || cli.What != msg {
			t.Fatalf("%q was replaced by %v", msg, got)
		}
	}
	var cli *errs.CLIError
	old := publicError(errs.New("E518", "source Postgres major 11 is older than 12; upgrade the source to Postgres 12 or newer"), "E517", "x")
	if !errors.As(old, &cli) || !strings.Contains(cli.What, "major 11 is older than 12") || strings.Contains(cli.What, "POSTGRES_VERSION") {
		t.Fatalf("old-source message: %v", old)
	}
	part := publicError(errs.New("E520", PartitionedPrefix+"public.events"), "E520", "source extraction failed")
	if !errors.As(part, &cli) || !strings.Contains(cli.What, "public.events") {
		t.Fatalf("partition message: %v", part)
	}
	hidden := publicError(errs.New("E520", PartitionedPrefix+"x; password=hunter2"), "E520", "source extraction failed")
	if !errors.As(hidden, &cli) || strings.Contains(cli.What, "hunter2") {
		t.Fatalf("unsafe name leaked: %v", hidden)
	}
}

// --plan must run the same bundle-free target preflights as the real run, with the source's
// user count, so a plan cannot pass and the real run then fail with E519.
func TestImporterPlanRunsTargetPreflights(t *testing.T) {
	targetSeams(t)
	var seen Detection
	targetChecks = func(_ context.Context, _ *config.Config, d Detection) error {
		seen = d
		return errs.New("E519", "target auth.users is not empty")
	}
	source := &testSource{name: "test-plan-auth", users: 3}
	Register(source)
	in := Input{Values: map[string]string{"from-env": "TEST_SOURCE"}, Env: func(string) string { return "in-memory" }}
	_, err := Run(context.Background(), &config.Config{}, source, in, RunOptions{ProjectDir: t.TempDir(), Plan: true})
	if err == nil || !strings.Contains(err.Error(), "auth.users is not empty") || seen.SourceUsers != 3 {
		t.Fatalf("plan skipped the auth preflight: err=%v detection=%+v", err, seen)
	}
}
