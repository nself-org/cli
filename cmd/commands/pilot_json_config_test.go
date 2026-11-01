package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/spf13/cobra"
)

// pilotConfigCmd is a fresh command with the config flags, parsed from args.
func pilotConfigCmd(t *testing.T, args ...string) (*cobra.Command, []string) {
	t.Helper()
	c := &cobra.Command{Use: "x"}
	c.Flags().String("env", "", "")
	c.Flags().Bool("reveal", false, "")
	c.Flags().Bool("json", false, "")
	c.Flags().String("format", "table", "")
	if err := c.Flags().Parse(args); err != nil {
		t.Fatal(err)
	}
	return c, c.Flags().Args()
}

// inDir runs fn with the working directory set to dir (the schema and golden
// paths of this package are relative, so t.Chdir for the whole test is not an
// option).
func inDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
	}()
	fn()
}

func TestPilotJSONConfig(t *testing.T) {
	proj := pilotProject(t)

	t.Run("in-process", func(t *testing.T) {
		compattest.Both(t, func(t *testing.T) {
			v15 := os.Getenv("NSELF_V15") == "1"
			run := func(fn func(*cobra.Command, []string) error, args ...string) (jsonOut, human string) {
				out, _ := pilotBuffers(t)
				c, pos := pilotConfigCmd(t, args...)
				var err error
				inDir(t, proj, func() {
					human, err = captureStdout(t, func() error { return fn(c, pos) })
				})
				if err != nil {
					t.Fatal(err)
				}
				return out.String(), human
			}

			// get: masked, then revealed.
			j, h := run(runConfigGet, "POSTGRES_PASSWORD", "--json")
			if !v15 {
				if j != "" || h != "***\n" {
					t.Fatalf("v1.4 config get --json must print the human value: json=%q human=%q", j, h)
				}
			} else {
				data := validateEnvelope(t, j, "config get")
				if h != "" || data["value"] != "***" || data["masked"] != true || data["key"] != "POSTGRES_PASSWORD" || data["file"] != ".env" {
					t.Fatalf("config get data = %v (human %q)", data, h)
				}
				j, _ = run(runConfigGet, "POSTGRES_PASSWORD", "--json", "--reveal")
				data = validateEnvelope(t, j, "config get")
				if data["value"] != pilotSecret || data["masked"] != false {
					t.Fatalf("config get --reveal data = %v", data)
				}
				j, _ = run(runConfigGet, "PROJECT_NAME", "--json")
				if data = validateEnvelope(t, j, "config get"); data["masked"] != false || data["value"] != "e01" {
					t.Fatalf("non-secret config get data = %v", data)
				}
			}

			// list: --reveal never reveals, exactly like the table.
			j, h = run(runConfigList, "--json", "--reveal")
			if !v15 {
				if j != "" || !strings.HasPrefix(h, "KEY ") {
					t.Fatalf("v1.4 config list --json must print the table: json=%q", j)
				}
			} else {
				data := validateEnvelope(t, j, "config list")
				if strings.Contains(j, pilotSecret) || h != "" {
					t.Fatalf("config list leaked the secret or printed the table: %s", j)
				}
				seen := map[string]string{}
				for _, k := range data["keys"].([]any) {
					row := k.(map[string]any)
					seen[row["key"].(string)] = row["source"].(string) + ":" + row["value"].(string)
				}
				if seen["POSTGRES_PASSWORD"] != "file:***" || seen["PROJECT_NAME"] != "file:e01" ||
					seen["BASE_DOMAIN"] != "default:local.nself.org" || seen["PROJECT_DOMAIN"] != "unset:" {
					t.Fatalf("config list rows = %v", seen)
				}
			}

			// show: --json and --format json are the same envelope in v1.5;
			// --format json is the bare map in v1.4 (--json ignored).
			j, h = run(runConfigShow, "--json")
			jf, hf := run(runConfigShow, "--format", "json")
			if !v15 {
				// --format json is the bare map (written through the pilot writer).
				if j != "" || !strings.Contains(h, "POSTGRES_PASSWORD=***") || hf != "" || !strings.HasPrefix(jf, "{\n") {
					t.Fatalf("v1.4 config show: --json=%q/%q --format json=%q/%q", j, h, jf, hf)
				}
				return
			}
			if j != jf || h != "" || hf != "" {
				t.Fatalf("v1.5 config show --json and --format json differ:\n%s\n%s", j, jf)
			}
			data := validateEnvelope(t, j, "config show")
			if data["POSTGRES_PASSWORD"] != "***" || data["PROJECT_NAME"] != "e01" || strings.Contains(j, pilotSecret) {
				t.Fatalf("config show data = %v", data)
			}
		})
	})

	t.Run("live", func(t *testing.T) {
		skipLiveUnderV15Guard(t)
		p := pilotProject(t)
		// v1.5: schema-valid envelopes, secrets masked, --reveal honoured.
		for _, tc := range []struct {
			cmd  string
			args []string
		}{
			{"config get", []string{"config", "get", "POSTGRES_PASSWORD", "--json"}},
			{"config list", []string{"config", "list", "--json"}},
			{"config show", []string{"config", "show", "--json"}},
			{"config show", []string{"config", "show", "--format", "json"}},
		} {
			r := runPilot(t, p, true, nil, tc.args...)
			validateEnvelope(t, r.stdout, tc.cmd)
			if r.code != 0 || strings.Contains(r.stdout+r.stderr, pilotSecret) {
				t.Fatalf("%v: exit %d or secret leaked\n%s%s", tc.args, r.code, r.stdout, r.stderr)
			}
		}
		rev := runPilot(t, p, true, nil, "config", "get", "POSTGRES_PASSWORD", "--json", "--reveal")
		if validateEnvelope(t, rev.stdout, "config get")["value"] != pilotSecret {
			t.Fatalf("--reveal did not reveal: %s", rev.stdout)
		}
		legacy := runPilot(t, p, true, []string{"NSELF_JSON_LEGACY=1"}, "config", "show", "--format", "json")
		if _, has := singleDoc(t, legacy.stdout)["schema_version"]; has {
			t.Fatalf("NSELF_JSON_LEGACY must restore the bare config show --format json: %s", legacy.stdout)
		}
		// Error envelope for a missing key (v1.5); plain stderr text in v1.4.
		miss := runPilot(t, p, true, nil, "config", "get", "NOPE", "--json")
		doc := singleDoc(t, miss.stdout)
		errObj, _ := doc["error"].(map[string]any)
		if miss.code != 1 || doc["command"] != "config get" || errObj["code"] != "E400" || errObj["exit_code"] != float64(1) {
			t.Fatalf("v1.5 config get NOPE --json: exit %d, doc %v", miss.code, doc)
		}
		if err := contractValidate(contractResolve(t, "envelope.v1.schema.json"), []byte(miss.stdout)); err != nil {
			t.Fatalf("error envelope invalid: %v", err)
		}
		old := runPilot(t, p, false, nil, "config", "get", "NOPE", "--json")
		if old.code != 1 || old.stdout != "" || old.stderr != "Error: key not found: NOPE\n" {
			t.Fatalf("v1.4 config get NOPE --json: exit %d stdout=%q stderr=%q", old.code, old.stdout, old.stderr)
		}
	})

	// Human output (and v1.4 --json, which is accepted and ignored) is byte
	// identical to the base binary's, captured in testdata/json/human/.
	t.Run("human goldens", func(t *testing.T) {
		p := pilotProject(t)
		for _, tc := range []struct {
			golden string
			args   []string
		}{
			{"config-show.txt", []string{"config", "show"}},
			{"config-show.txt", []string{"config", "show", "--json"}},
			{"config-show-reveal.txt", []string{"config", "show", "--reveal"}},
			{"config-show-format-yaml.txt", []string{"config", "show", "--format", "yaml"}},
			{"config-show-format-json.txt", []string{"config", "show", "--format", "json"}},
			{"config-get-secret.txt", []string{"config", "get", "POSTGRES_PASSWORD"}},
			{"config-get-secret.txt", []string{"config", "get", "POSTGRES_PASSWORD", "--json"}},
			{"config-list.txt", []string{"config", "list"}},
			{"config-list.txt", []string{"config", "list", "--json"}},
		} {
			parts := []string{tc.golden}
			if tc.golden == "config-list.txt" {
				parts = []string{"config-list.part1.txt", "config-list.part2.txt"}
			}
			var want []byte
			for _, part := range parts {
				data, err := os.ReadFile(filepath.Join("testdata", "json", "human", part))
				if err != nil {
					t.Fatal(err)
				}
				want = append(want, data...)
			}
			r := runPilot(t, p, false, nil, tc.args...)
			if r.code != 0 || r.stdout+r.stderr != string(want) {
				t.Fatalf("%v differs from %s (exit %d)\n--- got ---\n%s%s\n--- want ---\n%s", tc.args, tc.golden, r.code, r.stdout, r.stderr, want)
			}
		}
	})
}
