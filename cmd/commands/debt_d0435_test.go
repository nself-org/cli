package commands

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func debtDocker(t *testing.T, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX docker stub")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	prependPath(t, bin)
}

func TestD0435DetectJSON(t *testing.T) {
	debtDocker(t, fakeDockerSQL[strings.Index(fakeDockerSQL, "\n")+1:])
	proj := dataProject(t)
	mig := filepath.Join(proj, "migs")
	if err := os.Mkdir(mig, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mig, "20260102_next.sql"), []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runTypedBody(t, dbMigrateStatusCmd, map[string]string{"migration-dir": mig, "detect": "true"})
	if err != nil {
		t.Fatal(err)
	}
	var d dbMigrateStatusData
	dataOf(t, out, "db migrate status", &d)
	if len(d.Migrations) != 1 || d.Migrations[0].Name != "20260102_next.sql" || d.Migrations[0].Status != "UNKNOWN" {
		t.Fatalf("detect data: %+v", d)
	}
}

func TestD0435HealthRejectsInvalidRetries(t *testing.T) {
	old := healthRetries
	t.Cleanup(func() { healthRetries = old })
	for _, n := range []int{0, -1} {
		healthRetries = n
		if healthCmd.PersistentPreRunE == nil {
			t.Fatal("health lacks retry validation")
		}
		if err := healthCmd.PersistentPreRunE(healthCmd, nil); err == nil || !strings.Contains(err.Error(), "--retries") {
			t.Fatalf("retries=%d: %v", n, err)
		}
	}
}

func TestD0435HealthCommandRejectsInvalidRetries(t *testing.T) {
	compattest.Set(t, true)
	t.Cleanup(prepareTree(RootCmd, true, false))
	retriesChanged := healthCmd.PersistentFlags().Lookup("retries").Changed
	jsonChanged := healthCmd.PersistentFlags().Lookup("json").Changed
	t.Cleanup(func() {
		healthCmd.PersistentFlags().Lookup("retries").Changed = retriesChanged
		healthCmd.PersistentFlags().Lookup("json").Changed = jsonChanged
	})
	oldRetries, oldJSON := healthRetries, healthJSON
	oldPre := RootCmd.PersistentPreRunE
	rootCalled := false
	RootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error { rootCalled = true; return nil }
	t.Cleanup(func() {
		healthRetries, healthJSON = oldRetries, oldJSON
		RootCmd.PersistentPreRunE = oldPre
		RootCmd.SetArgs(nil)
	})
	for _, sub := range [][]string{nil, {"check"}, {"service", "postgres"}, {"endpoint", "http://127.0.0.1:1"}} {
		for _, n := range []int{0, -1} {
			for _, jsonOn := range []bool{false, true} {
				args := append([]string{"status", "health"}, sub...)
				args = append(args, "--retries", strconv.Itoa(n), "--json="+strconv.FormatBool(jsonOn))
				RootCmd.SetArgs(args)
				_, err := RootCmd.ExecuteC()
				if err == nil || !strings.Contains(err.Error(), "--retries") {
					t.Fatalf("%v: %v", args, err)
				}
			}
		}
	}
	healthRetries = 1
	if err := healthCmd.PersistentPreRunE(healthCmd, nil); err != nil || !rootCalled {
		t.Fatalf("root pre-run not preserved: %v", err)
	}
}

func TestD0435LintJSONViolations(t *testing.T) {
	debtDocker(t, `printf '%s\n' '[{"schema":"public","table":"np_orders","has_rls":false,"policy_count":0,"has_tenant_id":true}]'
`)
	for _, full := range []string{"false", "true"} {
		t.Run(full, func(t *testing.T) {
			dataProject(t)
			out, err := runTypedBody(t, dbLintCmd, map[string]string{"rls": full})
			if err == nil {
				t.Fatal("violations returned success")
			}
			if !strings.Contains(out, "np_orders") {
				t.Fatalf("missing findings: %s", out)
			}
			var exit *errs.ExitError
			if !errors.As(err, &exit) || exit.Code != 1 || !exit.Silent() {
				t.Fatalf("want silent exit 1, got %v", err)
			}
			dec := json.NewDecoder(strings.NewReader(out))
			var doc map[string]any
			if err := dec.Decode(&doc); err != nil {
				t.Fatal(err)
			}
			if err := dec.Decode(&doc); err != io.EOF {
				t.Fatalf("extra JSON output: %v", err)
			}
		})
	}
}

func TestD0435MetricJSONRejected(t *testing.T) {
	dataProject(t)
	debtDocker(t, "exit 0\n")
	_, err := runTypedBody(t, dbLintCmd, map[string]string{"metric": "true", "format": "json"})
	if err == nil || !strings.Contains(err.Error(), "--metric") {
		t.Fatalf("want explicit metric/json rejection, got %v", err)
	}
}

func TestD0435RemoteJSONRejected(t *testing.T) {
	compattest.Set(t, true)
	withLocalVersion(t, "dev")
	writeRemoteInventory(t)
	sent := 0
	withStubbedSSH(t, func(context.Context, []string) (string, error) { sent++; return "ok", nil })
	_, err := jsonV15(t, dbMigrateStatusCmd, map[string]string{"json": "true", "env": "staging"}, func() error { return runDBMigrateStatus(dbMigrateStatusCmd, nil) })
	if err == nil || !strings.Contains(err.Error(), "remote") || sent != 0 {
		t.Fatalf("remote JSON: err=%v SSH calls=%d", err, sent)
	}
}
