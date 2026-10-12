package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/upgrade/stack"
	"github.com/spf13/cobra"
)

// TestStackUpdateSafetyDocumentation pins the recovery and environment contract.
func TestStackUpdateSafetyDocumentation(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "wiki", "cmd-update.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{
		"`reconcile.EnvClass`", "`prod`, `production`, `staging`, and `stage`",
		"same classifier used by", "30-minute timeout",
		"between the dump and count", "update-stack.<run_id>.json",
		"`rollback-running` and `rollback-failed` remain blocked",
		"fsyncs the state directory", "verified backup",
	} {
		if !strings.Contains(string(data), phrase) {
			t.Errorf("stack safety documentation omits %q", phrase)
		}
	}
}

func stackTestCommand(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	c := newUpdateStackCommand()
	c.Flags().Bool("json", false, "")
	c.SetContext(context.Background())
	var out bytes.Buffer
	old := pilotWriter
	oldLock := stackLock
	stackLock = func(*cobra.Command, string) (func() error, error) { return func() error { return nil }, nil }
	pilotWriter = func() output.Writer { return output.Writer{Out: &out, Err: io.Discard} }
	t.Cleanup(func() { pilotWriter = old; stackLock = oldLock })
	return c, &out
}

// TestStackUpdateDryRunJSON proves previews use the registered schema and no backend.
func TestStackUpdateDryRunJSON(t *testing.T) {
	for _, flag := range []string{"dry-run", "plan"} {
		t.Run(flag, func(t *testing.T) {
			c, out := stackTestCommand(t)
			old := stackBackend
			stackBackend = func(string, *config.Config, io.Writer) stack.Backend { t.Fatal("preview reached runtime"); return nil }
			t.Cleanup(func() { stackBackend = old })
			if err := c.Flags().Set(flag, "true"); err != nil {
				t.Fatal(err)
			}
			if err := c.Flags().Set("json", "true"); err != nil {
				t.Fatal(err)
			}
			if err := runUpdateStack(c, nil); err != nil {
				t.Fatal(err)
			}
			var env struct {
				Command string            `json:"command"`
				Data    StackUpdateResult `json:"data"`
			}
			dec := json.NewDecoder(out)
			if err := dec.Decode(&env); err != nil {
				t.Fatal(err)
			}
			if err := dec.Decode(new(any)); err != io.EOF {
				t.Fatalf("more than one envelope: %v", err)
			}
			if env.Command != "update stack" || !env.Data.DryRun || len(env.Data.Steps) != 7 {
				t.Fatalf("unexpected envelope: %+v", env)
			}
			for i, s := range env.Data.Steps {
				if s.Name != stack.ExecutionPlan()[i].Name || s.Result != "pending" || s.DurationMS != 0 {
					t.Fatalf("step: %+v", s)
				}
			}
		})
	}
	c, err := canon.Load()
	if err != nil {
		t.Fatal(err)
	}
	if e := c.Commands["update stack"]; e.SideEffect != "destructive" {
		t.Fatalf("unsafe canon row: %+v", e)
	}
	for _, flag := range []string{"dry-run", "plan"} {
		if e := c.Commands["update stack"]; e.Output != canon.OutputDocument || e.Flags[flag].Output != "" {
			t.Fatalf("%s must inherit document output: %+v", flag, e)
		}
	}
}

// TestStackCommandPreviewExistingJournal proves previews describe the actual resume.
func TestStackCommandPreviewExistingJournal(t *testing.T) {
	for _, flag := range []string{"dry-run", "plan"} {
		t.Run(flag, func(t *testing.T) {
			c, out := stackTestCommand(t)
			dir := t.TempDir()
			state := filepath.Join(dir, ".nself", "state")
			if err := os.MkdirAll(state, 0700); err != nil {
				t.Fatal(err)
			}
			j := stack.Journal{RunID: "preview-existing", StartedAt: time.Now(), Status: "running", Prod: true}
			for i, s := range stack.ExecutionPlan() {
				result := "pending"
				if i < 3 {
					result = "ok"
				}
				if i == 3 {
					result = "running"
				}
				j.Steps = append(j.Steps, stack.Step{Name: s.Name, Result: result, DurationMS: int64(i)})
			}
			data, err := json.Marshal(j)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(state, "update-stack.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			oldDir, oldBackend := stackProjectDir, stackBackend
			stackProjectDir = func() (string, error) { return dir, nil }
			stackBackend = func(string, *config.Config, io.Writer) stack.Backend { t.Fatal("preview reached backend"); return nil }
			stackLock = func(*cobra.Command, string) (func() error, error) { t.Fatal("preview acquired lock"); return nil, nil }
			t.Cleanup(func() { stackProjectDir, stackBackend = oldDir, oldBackend })
			if err := c.Flags().Set(flag, "true"); err != nil {
				t.Fatal(err)
			}
			if err := c.Flags().Set("json", "true"); err != nil {
				t.Fatal(err)
			}
			if err := runUpdateStack(c, nil); err != nil {
				t.Fatal(err)
			}
			var env struct {
				Data StackUpdateResult `json:"data"`
			}
			if err := json.Unmarshal(out.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			if !env.Data.DryRun || env.Data.RunID != j.RunID || env.Data.Status != j.Status || env.Data.ResumeStep != "start" || env.Data.Steps[2].Result != "ok" || env.Data.Steps[3].DurationMS != 3 {
				t.Fatalf("preview lost resume evidence: %+v", env.Data)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, after) || !before.ModTime().Equal(info.ModTime()) {
				t.Fatal("preview changed journal")
			}
		})
	}
}

// TestStackCommandNoConsent prevents backend construction before refusal.
func TestStackCommandNoConsent(t *testing.T) {
	c, _ := stackTestCommand(t)
	old := stackProjectDir
	stackProjectDir = func() (string, error) { t.Fatal("unconfirmed execution resolved project"); return "", nil }
	t.Cleanup(func() { stackProjectDir = old })
	if err := runUpdateStack(c, nil); errs.ExitCodeFor(err) != 4 {
		t.Fatalf("want E403 exit 4: %v", err)
	}
}

// TestStackCommandFailureJSON proves one E403 envelope and matching exit in both modes.
func TestStackCommandFailureJSON(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("NSELF_V15", mode)
			c, out := stackTestCommand(t)
			c.SetIn(bytes.NewReader(nil))
			if err := c.Flags().Set("json", "true"); err != nil {
				t.Fatal(err)
			}
			err := runUpdateStack(c, nil)
			var coder errs.ExitCoder
			if !errors.As(err, &coder) || coder.ExitCode() != 4 || errs.ExitCodeFor(err) != 4 {
				t.Fatalf("failure exit class: %v", err)
			}
			var silent errs.Silencer
			if !errors.As(err, &silent) || !silent.Silent() {
				t.Fatalf("failure not marked reported: %v", err)
			}
			var coded *errs.CLIError
			if !errors.As(err, &coded) {
				t.Fatalf("lost structured error: %v", err)
			}
			var envelope output.ErrorEnvelope
			decoder := json.NewDecoder(out)
			if err := decoder.Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Command != "update stack" || envelope.Error == nil || envelope.Error.Code != "E403" || envelope.Error.ExitCode != 4 {
				t.Fatalf("incorrect failure envelope: %+v", envelope)
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				t.Fatalf("more than one failure envelope: %v", err)
			}
		})
	}
}

// TestStackCommandReleaseFailureJSON proves cleanup errors replace staged success.
func TestStackCommandReleaseFailureJSON(t *testing.T) {
	c, out := stackTestCommand(t)
	dir := t.TempDir()
	oldDir, oldCfg, oldBackend := stackProjectDir, stackLoadConfig, stackBackend
	stackProjectDir = func() (string, error) { return dir, nil }
	stackLoadConfig = func(string) (*config.Config, error) { return &config.Config{Env: "dev"}, nil }
	stackBackend = func(string, *config.Config, io.Writer) stack.Backend { return stackCommandBackend{} }
	t.Cleanup(func() { stackProjectDir, stackLoadConfig, stackBackend = oldDir, oldCfg, oldBackend })
	releaseFailure := errors.New("injected lock-release failure")
	stackLock = func(*cobra.Command, string) (func() error, error) { return func() error { return releaseFailure }, nil }
	for _, flag := range []string{"yes", "json"} {
		if err := c.Flags().Set(flag, "true"); err != nil {
			t.Fatal(err)
		}
	}
	err := runUpdateStack(c, nil)
	if !errors.Is(err, releaseFailure) {
		t.Fatalf("lost lock-release error: %v", err)
	}
	var silent errs.Silencer
	if !errors.As(err, &silent) || !silent.Silent() {
		t.Fatalf("cleanup error was not reported: %v", err)
	}
	var envelope map[string]json.RawMessage
	decoder := json.NewDecoder(out)
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["error"] == nil || envelope["data"] != nil {
		t.Fatalf("cleanup failure emitted success: %s", out.String())
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("more than one cleanup envelope: %v", err)
	}
}

// TestStackDecoratedPreviewDoesNotWrite proves the actual invocation lock is bypassed.
func TestStackDecoratedPreviewDoesNotWrite(t *testing.T) {
	for _, flag := range []string{"--dry-run", "--plan"} {
		t.Run(flag, func(t *testing.T) {
			c, _ := stackTestCommand(t)
			dir := t.TempDir()
			t.Chdir(dir)
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fixture\nENV=prod\n"), 0600); err != nil {
				t.Fatal(err)
			}
			root := &cobra.Command{Use: "nself"}
			parent := &cobra.Command{Use: "update"}
			root.AddCommand(parent)
			parent.AddCommand(c)
			installInvocationDecorator(root)
			root.SetArgs([]string{"update", "stack", flag, "--json"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != ".env" {
				t.Fatalf("preview wrote project files: %v", entries)
			}
		})
	}
}

type stackCommandBackend struct{}

type stackFailedBackup struct{ stackCommandBackend }

func (stackFailedBackup) Backup(context.Context, *stack.Journal) error {
	return errors.New("injected backup creation failure")
}

// TestStackCommandProdClassGuard proves configuration determines production policy.
func TestStackCommandProdClassGuard(t *testing.T) {
	for _, env := range []string{"prod", "production", "stage", "staging"} {
		t.Run(env, func(t *testing.T) {
			c, _ := stackTestCommand(t)
			dir := t.TempDir()
			oldDir, oldCfg, oldBackend := stackProjectDir, stackLoadConfig, stackBackend
			stackProjectDir = func() (string, error) { return dir, nil }
			stackLoadConfig = func(string) (*config.Config, error) { return &config.Config{Env: env}, nil }
			stackBackend = func(string, *config.Config, io.Writer) stack.Backend { return stackFailedBackup{} }
			t.Cleanup(func() { stackProjectDir, stackLoadConfig, stackBackend = oldDir, oldCfg, oldBackend })
			if err := c.Flags().Set("yes", "true"); err != nil {
				t.Fatal(err)
			}
			err := runUpdateStack(c, nil)
			if errs.ExitCodeFor(err) != 4 || errs.Describe(err).Code != "E403" {
				t.Fatalf("production guard: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, ".nself", "state", "update-stack.json"))
			if err != nil {
				t.Fatal(err)
			}
			var j stack.Journal
			if err := json.Unmarshal(data, &j); err != nil {
				t.Fatal(err)
			}
			if !j.Prod || j.Status != "awaiting-operator" || j.Steps[1].Result != "pending" {
				t.Fatalf("unsafe production transition: %+v", j)
			}
		})
	}
}

func (stackCommandBackend) Backup(_ context.Context, j *stack.Journal) error {
	j.BackupID = "fixture.dump"
	j.BackupCreatedAt = time.Now()
	j.PreviousCompose = "fixture-previous.json"
	return nil
}
func (stackCommandBackend) VerifyBackup(_ context.Context, j *stack.Journal) error {
	j.BackupVerifiedAt = time.Now()
	return nil
}
func (stackCommandBackend) Run(context.Context, string, *stack.Journal) error     { return nil }
func (stackCommandBackend) Restore(context.Context, *stack.Journal) error         { return nil }
func (stackCommandBackend) RestartPrevious(context.Context, *stack.Journal) error { return nil }
func (stackCommandBackend) CountRows(context.Context) (map[string]int64, error) {
	return map[string]int64{"public.np_rows": 3}, nil
}

// TestStackCommandSuccessJSON proves the complete command emits measured step facts.
func TestStackCommandSuccessJSON(t *testing.T) {
	c, out := stackTestCommand(t)
	dir := t.TempDir()
	oldDir, oldCfg, oldBackend := stackProjectDir, stackLoadConfig, stackBackend
	stackProjectDir = func() (string, error) { return dir, nil }
	stackLoadConfig = func(string) (*config.Config, error) { return &config.Config{Env: "dev"}, nil }
	stackBackend = func(string, *config.Config, io.Writer) stack.Backend { return stackCommandBackend{} }
	t.Cleanup(func() { stackProjectDir, stackLoadConfig, stackBackend = oldDir, oldCfg, oldBackend })
	for _, f := range []string{"yes", "json"} {
		if err := c.Flags().Set(f, "true"); err != nil {
			t.Fatal(err)
		}
	}
	if err := runUpdateStack(c, nil); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Data StackUpdateResult `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Steps) != 7 {
		t.Fatalf("missing steps: %s", out.String())
	}
	for _, s := range env.Data.Steps {
		if s.Result != "ok" || s.DurationMS < 0 {
			t.Fatalf("bad result: %+v", s)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".nself", "state", "update-stack.json")); err != nil {
		t.Fatal(err)
	}
}
