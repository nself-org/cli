package commands

// service_add_upgrade_dotenv_test.go — D-0444/D-0558 and D-0552 for the service
// env helpers: readEnvValues keeps file text out of its error, and
// setEnvKeyInFile stores a '$' value so the loader reads it back unchanged.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestReadEnvValuesDotenvSyntaxError(t *testing.T) {
	for _, tc := range dotenvLeakCases {
		t.Run(tc.name, func(t *testing.T) {
			path := dotenvPlantFile(t, tc.content)
			_, err := readEnvValues(path)
			if err == nil {
				t.Fatal("want a syntax error")
			}
			assertDotenvNoLeak(t, err.Error(), path, 2)
		})
	}
}

func TestSetEnvKeyInFileDollarValueRoundTrips(t *testing.T) {
	const hash = "$2a$10$fakefakefakefakefake"
	for _, seed := range []string{"", "A=1\n# note\nHASH=old\nB=2\n"} {
		path := filepath.Join(t.TempDir(), ".env.secrets")
		if seed != "" {
			if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := setEnvKeyInFile(path, "HASH", hash); err != nil {
			t.Fatal(err)
		}
		got, err := config.ReadEnvFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got["HASH"] != hash {
			t.Errorf("seed %q: HASH read back as %q, want %q", seed, got["HASH"], hash)
		}
		if seed != "" && (got["A"] != "1" || got["B"] != "2") {
			t.Errorf("other keys changed: %v", got)
		}
		if found := config.UnquotedDollarFindings(filepath.Dir(path), "dev"); len(found) != 0 {
			t.Errorf("writer output still trips the validator: %+v", found)
		}
	}
}
