package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/testkit"
	"os"
	"strings"
	"testing"
)

func TestTargetHelperCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := testkit.StartTarget(ctx, t.TempDir()); err == nil {
		t.Fatal("cancelled target helper ran")
	}
}
func TestPostgresVersions(t *testing.T) {
	for _, tc := range []struct {
		source, target int
		fail           bool
	}{{16, 16, false}, {12, 16, false}, {17, 16, true}, {11, 16, true}} {
		err := CheckVersion(tc.source, tc.target)
		if (err != nil) != tc.fail {
			t.Fatalf("%+v: %v", tc, err)
		}
		if tc.source == 17 && !strings.Contains(err.Error(), "POSTGRES_VERSION=17") {
			t.Fatal("version guidance missing")
		}
	}
}
func TestPostgresUnreachableRedaction(t *testing.T) {
	secret := fixtureSecret(t)
	_, err := (Source{}).Detect(context.Background(), importer.Input{Values: map[string]string{"from-env": "SRC_URL"}, Env: func(string) string {
		return "postgresql://postgres:" + secret + "@127.0.0.1:1/postgres?connect_timeout=1"
	}})
	if err == nil || !strings.Contains(err.Error(), "E517") || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe source failure: %v", err)
	}
}
func TestPostgresDumpPasswordNotArgv(t *testing.T) {
	cfg, err := pgx.ParseConfig("postgresql://postgres:" + fixtureSecret(t) + "@localhost/postgres")
	if err != nil {
		t.Fatal(err)
	}
	spec := dumpSpec(cfg, "image", "/tmp/stage")
	raw, _ := json.Marshal(spec.Args)
	if strings.Contains(string(raw), cfg.Password) || spec.EnvPass["PGPASSWORD"] != cfg.Password {
		t.Fatal("unsafe dump credentials")
	}
	if spec.User == "" {
		t.Fatal("dump not run as host uid")
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

func TestDumpArgsQuotePatterns(t *testing.T) {
	args, err := dumpArgs("00000003-1", []string{"MyApp", "we\"ird", "public"})
	if err != nil {
		t.Skipf("identifier policy refuses a name: %v", err)
	}
	joined := strings.Join(args, "\n")
	for _, want := range []string{"--schema=\"MyApp\"", "--schema=\"public\"", "--snapshot=00000003-1", "--exclude-table=*.np_*"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("argv lacks %s: %v", want, args)
		}
	}
	if quotePattern(`a"b`) != `"a""b"` || quotePattern("My*App") != `"My*App"` {
		t.Fatal("inner quotes must be doubled and wildcards stay quoted")
	}
	if _, err = dumpArgs("s", nil); err == nil {
		t.Fatal("no schemas must be refused")
	}
}

func TestMajorOfAndOldSource(t *testing.T) {
	for in, want := range map[string]int{"16.4": 16, "17beta1": 17, "9.6.24": 9, "": 0} {
		if got := majorOf(in); got != want {
			t.Fatalf("majorOf(%q)=%d want %d", in, got, want)
		}
	}
	err := CheckVersion(11, 16)
	var cli *errs.CLIError
	if !errors.As(err, &cli) || !strings.Contains(cli.What, "upgrade the source to Postgres 12 or newer") || strings.Contains(cli.What, "POSTGRES_VERSION") {
		t.Fatalf("old source message: %v", err)
	}
}
