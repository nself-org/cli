package exporter

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func recordedSteps(log *[]string, failAt string) bundleSteps {
	step := func(name string) func() error {
		return func() error {
			*log = append(*log, name)
			if name == failAt {
				return errors.New("FAKE-SOURCE-DETAIL")
			}
			return nil
		}
	}
	return bundleSteps{database: step("database"), auth: step("auth"), commit: step("commit"), storage: step("storage"), metadata: step("metadata")}
}

// The snapshot transaction ends right after the last read that needs it and
// before the object download and the Hasura call.
func TestSnapshotCommitsBeforeStorageAndMetadata(t *testing.T) {
	var log []string
	if err := runBundleSteps(recordedSteps(&log, "")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(log, ","); got != "database,auth,commit,storage,metadata" {
		t.Fatalf("order %s", got)
	}
}

func TestBundleStepFailuresStopAndWithholdDetails(t *testing.T) {
	for _, tc := range []struct{ failAt, want, ran string }{
		{"database", "FAKE-SOURCE-DETAIL", "database"},
		{"auth", "read auth", "database,auth"},
		{"commit", "finish snapshot", "database,auth,commit"},
		{"storage", "read storage", "database,auth,commit,storage"},
		{"metadata", "read metadata", "database,auth,commit,storage,metadata"},
	} {
		var log []string
		err := runBundleSteps(recordedSteps(&log, tc.failAt))
		if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Join(log, ",") != tc.ran {
			t.Errorf("%s: err=%v ran=%v", tc.failAt, err, log)
		}
		if tc.failAt != "database" && strings.Contains(err.Error(), "FAKE-SOURCE-DETAIL") {
			t.Errorf("%s: source detail leaked", tc.failAt)
		}
	}
}

func TestStaleStageWarning(t *testing.T) {
	parent := t.TempDir()
	var quiet bytes.Buffer
	warnStaleStages(&quiet, parent)
	if quiet.Len() != 0 {
		t.Fatal("warned without stale stages")
	}
	stale := stagePrefix + "FAKE1234"
	for _, name := range []string{stale, "unrelated"} {
		if err := os.Mkdir(filepath.Join(parent, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(parent, stale, "FAKE-CONTENT-SECRET"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	warnStaleStages(&out, parent)
	msg := out.String()
	if !strings.Contains(msg, stale) || strings.Contains(msg, "unrelated") || strings.Contains(msg, "FAKE-CONTENT-SECRET") || !strings.Contains(msg, "rm -rf "+parent+"/"+stagePrefix) {
		t.Fatalf("warning %q", msg)
	}
	warnStaleStages(&out, filepath.Join(parent, "missing")) // unreadable parent: silent
}

func TestExportWarnsAboutStaleStageBeforeStarting(t *testing.T) {
	fakeUID(t)
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, stagePrefix+"FAKE5678"), 0o700); err != nil {
		t.Fatal(err)
	}
	var warn bytes.Buffer
	t.Chdir(parent)
	// No project is started, so Export fails after the warning, before any write.
	_, err := Export(context.Background(), &config.Config{}, Options{Out: filepath.Join(parent, "bundle"), Warn: &warn})
	if err == nil || !strings.Contains(warn.String(), stagePrefix+"FAKE5678") {
		t.Fatalf("err=%v warn=%q", err, warn.String())
	}
}

func TestExportRefusesPlatformWithoutUID(t *testing.T) {
	old := currentUID
	currentUID = func() int { return -1 }
	t.Cleanup(func() { currentUID = old })
	parent := t.TempDir()
	_, err := Export(context.Background(), &config.Config{}, Options{Out: filepath.Join(parent, "bundle")})
	if err == nil || !strings.Contains(err.Error(), "Linux and macOS only") {
		t.Fatalf("err=%v", err)
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 0 {
		t.Fatal("unsupported platform wrote files")
	}
}

func TestDiskEstimateIncludesScanRendering(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	free := func(n uint64) func(string) (uint64, error) { return func(string) (uint64, error) { return n, nil } }
	// 1000 relation bytes: 1200 archive + 2500 plain-SQL scan + 100 objects.
	assertCode(t, diskCheck(out, 1000, 100, free(3799)), "E525")
	if err := diskCheck(out, 1000, 100, free(3800)); err != nil {
		t.Fatal(err)
	}
	// Odd sizes round the scan allowance up.
	assertCode(t, diskCheck(out, 3, 0, free(3+1+7)), "E525")
	if err := diskCheck(out, 3, 0, free(3+1+8)); err != nil {
		t.Fatal(err)
	}
}
