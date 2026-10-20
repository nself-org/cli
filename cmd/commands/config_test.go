package commands

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/sysexec"
	"github.com/spf13/cobra"
)

// newConfigCmd returns a fresh cobra.Command tree with only the config command
// and its subcommands wired in, so tests run in isolation from global state.
func newConfigCmd() *cobra.Command {
	root := &cobra.Command{Use: "nself", RunE: func(cmd *cobra.Command, args []string) error { return nil }}

	cc := &cobra.Command{
		Use:   "config",
		Short: "Manage project configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cc.PersistentFlags().String("env", "", "Target environment")
	cc.PersistentFlags().Bool("reveal", false, "Show secret values in plaintext")
	cc.PersistentFlags().Bool("json", false, "JSON output")

	showCmd := &cobra.Command{Use: "show", Short: "Show all config key=value pairs", RunE: runConfigShow}
	getCmd := &cobra.Command{Use: "get <key>", Short: "Get a single configuration value", Args: cobra.ExactArgs(1), RunE: runConfigGet}
	setCmd := &cobra.Command{Use: "set <key> <value>", Short: "Update a configuration value", Args: cobra.ExactArgs(2), RunE: runConfigSet}
	listCmd := &cobra.Command{Use: "list", Short: "List all known config keys", RunE: runConfigList}
	validateCmd := &cobra.Command{Use: "validate", Short: "Validate configuration", RunE: runConfigValidate}
	exportCmd := &cobra.Command{Use: "export <file>", Short: "Export config to file", Args: cobra.ExactArgs(1), RunE: runConfigExport}
	importCmd := &cobra.Command{Use: "import <file>", Short: "Import config from file", Args: cobra.ExactArgs(1), RunE: runConfigImport}
	importCmd.Flags().Bool("force", false, "Skip confirmation prompt")

	cc.AddCommand(showCmd)
	cc.AddCommand(getCmd)
	cc.AddCommand(setCmd)
	cc.AddCommand(listCmd)
	cc.AddCommand(validateCmd)
	cc.AddCommand(exportCmd)
	cc.AddCommand(importCmd)
	root.AddCommand(cc)
	return root
}

// TestConfigCmd_Registered verifies that the config subcommand is registered on
// the global RootCmd.
func TestConfigCmd_Registered(t *testing.T) {
	found := false
	for _, c := range RootCmd.Commands() {
		if c.Name() == "config" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'config' to be registered on RootCmd")
	}
}

// TestConfigCmd_ShowSubcommandRegistered verifies that 'config show' is wired
// as a subcommand of 'config'.
func TestConfigCmd_ShowSubcommandRegistered(t *testing.T) {
	var configParent *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "config" {
			configParent = c
			break
		}
	}
	if configParent == nil {
		t.Fatal("config command not found on RootCmd")
	}

	found := false
	for _, sub := range configParent.Commands() {
		if sub.Name() == "show" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'config show' subcommand to be registered")
	}
}

// TestConfigCmd_GetSubcommandRegistered verifies that 'config get' is wired as
// a subcommand of 'config'.
func TestConfigCmd_GetSubcommandRegistered(t *testing.T) {
	var configParent *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "config" {
			configParent = c
			break
		}
	}
	if configParent == nil {
		t.Fatal("config command not found on RootCmd")
	}

	found := false
	for _, sub := range configParent.Commands() {
		if sub.Name() == "get" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'config get' subcommand to be registered")
	}
}

// TestConfigCmd_SetSubcommandRegistered verifies that 'config set' is wired as
// a subcommand of 'config'.
func TestConfigCmd_SetSubcommandRegistered(t *testing.T) {
	var configParent *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "config" {
			configParent = c
			break
		}
	}
	if configParent == nil {
		t.Fatal("config command not found on RootCmd")
	}

	found := false
	for _, sub := range configParent.Commands() {
		if sub.Name() == "set" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'config set' subcommand to be registered")
	}
}

// TestConfigGet_RequiresArgument verifies that 'config get' returns an error
// when called without the required key argument.
func TestConfigGet_RequiresArgument(t *testing.T) {
	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"config", "get"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'config get' is called without arguments, got nil")
	}
}

// TestConfigGet_TooManyArguments verifies that 'config get' rejects more than
// one positional argument.
func TestConfigGet_TooManyArguments(t *testing.T) {
	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"config", "get", "KEY1", "KEY2"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'config get' is called with too many arguments, got nil")
	}
}

// TestConfigSet_RequiresTwoArguments verifies that 'config set' returns an
// error when called with fewer than two positional arguments.
func TestConfigSet_RequiresTwoArguments(t *testing.T) {
	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"config", "set", "KEY_ONLY"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'config set' is called with only one argument, got nil")
	}
}

// TestConfigSet_RequiresAtLeastOneArgument verifies that 'config set' with no
// arguments returns an error.
func TestConfigSet_RequiresAtLeastOneArgument(t *testing.T) {
	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"config", "set"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'config set' is called without arguments, got nil")
	}
}

// TestConfigSet_TooManyArguments verifies that 'config set' rejects more than
// two positional arguments.
func TestConfigSet_TooManyArguments(t *testing.T) {
	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"config", "set", "K", "V", "EXTRA"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'config set' is called with three arguments, got nil")
	}
}

// TestConfigShow_NoProject verifies that 'config show' returns a meaningful
// error when not inside an nself project directory.
func TestConfigShow_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"config", "show"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'config show' is run outside an nself project, got nil")
	}
	if !strings.Contains(err.Error(), "nself init") && !strings.Contains(err.Error(), "nself project") {
		t.Errorf("expected error to mention project or nself init, got: %v", err)
	}
}

// TestConfigCmd_EnvFlagAccepted verifies that --env is a recognised flag for
// config subcommands (persistent flag, so available to all children).
func TestConfigCmd_EnvFlagAccepted(t *testing.T) {
	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	// Even though this will fail (no project), the flag must be recognised.
	root.SetArgs([]string{"config", "show", "--env", "dev"})
	err := root.Execute()

	if err != nil && strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--env flag was not recognised: %v", err)
	}
}

// TestIsSecretKey verifies the internal secret-key detection helper used by
// config show / config get for masking sensitive values.
func TestIsSecretKey(t *testing.T) {
	cases := []struct {
		key      string
		expected bool
	}{
		{"POSTGRES_PASSWORD", true},
		{"JWT_SECRET", true},
		{"API_KEY", true},
		{"ACCESS_TOKEN", true},
		{"PROJECT_NAME", false},
		{"BASE_DOMAIN", false},
		{"REDIS_ENABLED", false},
	}

	for _, tc := range cases {
		got := config.IsSecretKey(tc.key)
		if got != tc.expected {
			t.Errorf("IsSecretKey(%q) = %v, want %v", tc.key, got, tc.expected)
		}
	}
}

// TestMaskValue verifies that maskValue returns "***" for secret keys when
// reveal is false, and the plaintext value when reveal is true.
func TestMaskValue(t *testing.T) {
	if got := maskValue("POSTGRES_PASSWORD", "secret123", false); got != "***" {
		t.Errorf("maskValue(secret key, reveal=false) = %q, want %q", got, "***")
	}
	if got := maskValue("POSTGRES_PASSWORD", "secret123", true); got != "secret123" {
		t.Errorf("maskValue(secret key, reveal=true) = %q, want %q", got, "secret123")
	}
	if got := maskValue("PROJECT_NAME", "myapp", false); got != "myapp" {
		t.Errorf("maskValue(non-secret key) = %q, want %q", got, "myapp")
	}
	// Empty values should not be masked even for secret keys.
	if got := maskValue("POSTGRES_PASSWORD", "", false); got != "" {
		t.Errorf("maskValue(secret key, empty value) = %q, want empty string", got)
	}
}

// TestSetEnvFileLine_NewFile verifies that setEnvFileLine creates the file when
// it does not yet exist and writes the key=value pair.
func TestSetEnvFileLine_NewFile(t *testing.T) {
	dir := t.TempDir()
	envFile := dir + "/.env"

	updated, err := setEnvFileLine(envFile, "MY_KEY", "my_value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated {
		t.Error("expected updated=false for a newly created file, got true")
	}

	content, readErr := os.ReadFile(envFile)
	if readErr != nil {
		t.Fatalf("could not read created file: %v", readErr)
	}
	if !strings.Contains(string(content), "MY_KEY=my_value") {
		t.Errorf("expected MY_KEY=my_value in file, got: %s", string(content))
	}
}

// TestSetEnvFileLine_UpdateExisting verifies that setEnvFileLine updates an
// existing key in-place and returns updated=true.
func TestSetEnvFileLine_UpdateExisting(t *testing.T) {
	dir := t.TempDir()
	envFile := dir + "/.env"

	// Seed the file with an initial value.
	if _, err := setEnvFileLine(envFile, "MY_KEY", "old_value"); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	updated, err := setEnvFileLine(envFile, "MY_KEY", "new_value")
	if err != nil {
		t.Fatalf("unexpected error on update: %v", err)
	}
	if !updated {
		t.Error("expected updated=true for an existing key, got false")
	}

	content, readErr := os.ReadFile(envFile)
	if readErr != nil {
		t.Fatalf("could not read updated file: %v", readErr)
	}
	if strings.Contains(string(content), "old_value") {
		t.Errorf("old_value should have been replaced, file contents: %s", string(content))
	}
	if !strings.Contains(string(content), "new_value") {
		t.Errorf("expected new_value in file, got: %s", string(content))
	}
}

// TestSetEnvFileLine_PreservesComments verifies that setEnvFileLine preserves
// comment lines and blank lines when updating an existing key.
func TestSetEnvFileLine_PreservesComments(t *testing.T) {
	dir := t.TempDir()
	envFile := dir + "/.env"

	initial := "# This is a comment\nMY_KEY=old_value\n# End comment\n"
	if err := os.WriteFile(envFile, []byte(initial), 0644); err != nil {
		t.Fatalf("could not seed file: %v", err)
	}

	if _, err := setEnvFileLine(envFile, "MY_KEY", "new_value"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, readErr := os.ReadFile(envFile)
	if readErr != nil {
		t.Fatalf("could not read file: %v", readErr)
	}
	s := string(content)
	if !strings.Contains(s, "# This is a comment") {
		t.Error("comment at top was not preserved")
	}
	if !strings.Contains(s, "# End comment") {
		t.Error("comment at bottom was not preserved")
	}
	if !strings.Contains(s, "MY_KEY=new_value") {
		t.Errorf("expected MY_KEY=new_value in file, got: %s", s)
	}
}

// TestEnvFileName verifies that envFileName returns the expected file path for
// both the default (empty) and named environment cases. Uses filepath.Join
// in the expectation so Windows (\) and Unix (/) path separators are both
// accepted natively — envFileName itself uses filepath.Join.
func TestEnvFileName(t *testing.T) {
	want := filepath.Join("/project", ".env")
	got := envFileName("/project", "")
	if got != want {
		t.Errorf("envFileName('/project', '') = %q, want %q", got, want)
	}

	want = filepath.Join("/project", ".env.dev")
	got = envFileName("/project", "dev")
	if got != want {
		t.Errorf("envFileName('/project', 'dev') = %q, want %q", got, want)
	}

	want = filepath.Join("/project", ".env.prod")
	got = envFileName("/project", "prod")
	if got != want {
		t.Errorf("envFileName('/project', 'prod') = %q, want %q", got, want)
	}
}

// ── functional test helpers ──────────────────────────────────────────────────

// makeNSelfProject creates a temp directory with a minimal .env file, changes
// the working directory to it, and returns the directory path.
// FindNSelfRoot discovers a project by checking for .env in cwd.
func makeNSelfProject(t *testing.T, envContent string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatalf("makeNSelfProject: write .env: %v", err)
	}
	t.Chdir(dir)
	return dir
}

// captureStdout redirects os.Stdout to a pipe while fn runs, then returns the
// captured bytes and the error returned by fn. Tests in this package are not
// run in parallel, so the global os.Stdout redirect is safe.
//
// The read end is drained in a goroutine so that the write side never blocks
// on a full pipe buffer — a deadlock that manifests on Windows where the
// default anonymous-pipe capacity (~4 KB) is much smaller than on Linux.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("captureStdout: pipe: %v", err)
	}

	// Drain the read end concurrently so the writer never blocks.
	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&buf, r)
		_ = r.Close()
		done <- copyErr
	}()

	old := os.Stdout
	os.Stdout = w
	fnErr := fn()
	_ = w.Close()
	os.Stdout = old

	if copyErr := <-done; copyErr != nil {
		t.Fatalf("captureStdout: copy: %v", copyErr)
	}
	return buf.String(), fnErr
}

// ── config show — functional tests ───────────────────────────────────────────

// TestConfigShow_PrintsKeyValuePairs verifies that 'config show' loads the
// .env file in the project root and prints each key=value pair.
func TestConfigShow_PrintsKeyValuePairs(t *testing.T) {
	makeNSelfProject(t, "PROJECT_NAME=myapp\nBASE_DOMAIN=example.com\n")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "show"})

	output, err := captureStdout(t, func() error { return root.Execute() })
	if err != nil {
		t.Fatalf("config show failed: %v", err)
	}
	if !strings.Contains(output, "PROJECT_NAME=myapp") {
		t.Errorf("expected PROJECT_NAME=myapp in output, got:\n%s", output)
	}
	if !strings.Contains(output, "BASE_DOMAIN=example.com") {
		t.Errorf("expected BASE_DOMAIN=example.com in output, got:\n%s", output)
	}
}

// TestConfigShow_MasksSecretValues verifies that secret keys (containing
// PASSWORD, SECRET, KEY, TOKEN) are masked as *** by default.
func TestConfigShow_MasksSecretValues(t *testing.T) {
	makeNSelfProject(t, "POSTGRES_PASSWORD=hunter2\n")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "show"})

	output, err := captureStdout(t, func() error { return root.Execute() })
	if err != nil {
		t.Fatalf("config show failed: %v", err)
	}
	if strings.Contains(output, "hunter2") {
		t.Errorf("secret value must not appear in masked output:\n%s", output)
	}
	if !strings.Contains(output, "POSTGRES_PASSWORD=***") {
		t.Errorf("expected POSTGRES_PASSWORD=*** in output, got:\n%s", output)
	}
}

// TestConfigShow_RevealFlagShowsSecrets verifies that --reveal causes secret
// values to be printed in plaintext.
func TestConfigShow_RevealFlagShowsSecrets(t *testing.T) {
	makeNSelfProject(t, "POSTGRES_PASSWORD=hunter2\n")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "show", "--reveal"})

	output, err := captureStdout(t, func() error { return root.Execute() })
	if err != nil {
		t.Fatalf("config show --reveal failed: %v", err)
	}
	if !strings.Contains(output, "hunter2") {
		t.Errorf("expected plaintext secret value with --reveal, got:\n%s", output)
	}
}

// ── config get — functional tests ────────────────────────────────────────────

// TestConfigGet_KnownKeyReturnsValue verifies that 'config get KEY' prints the
// value when the key exists in the .env file.
func TestConfigGet_KnownKeyReturnsValue(t *testing.T) {
	makeNSelfProject(t, "BASE_DOMAIN=myapp.dev\n")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "get", "BASE_DOMAIN"})

	output, err := captureStdout(t, func() error { return root.Execute() })
	if err != nil {
		t.Fatalf("config get BASE_DOMAIN failed: %v", err)
	}
	if !strings.Contains(output, "myapp.dev") {
		t.Errorf("expected 'myapp.dev' in output, got: %q", output)
	}
}

// TestConfigGet_UnknownKeyReturnsError verifies that 'config get KEY' returns
// an error that mentions the key when the key does not exist in the .env file.
func TestConfigGet_UnknownKeyReturnsError(t *testing.T) {
	makeNSelfProject(t, "BASE_DOMAIN=myapp.dev\n")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "get", "NONEXISTENT_KEY_XYZ"})

	_, err := captureStdout(t, func() error { return root.Execute() })
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "NONEXISTENT_KEY_XYZ") {
		t.Errorf("expected error to mention key name, got: %v", err)
	}
}

// ── config set — functional tests ────────────────────────────────────────────

// TestConfigSet_UpdatesExistingKey verifies that 'config set KEY VALUE'
// updates an existing key in-place in the .env file.
func TestConfigSet_UpdatesExistingKey(t *testing.T) {
	t.Cleanup(sysexec.Set(&sysexec.Fake{})) // D-0275: v1.5 config set builds; keep it off the host mkcert
	dir := makeNSelfProject(t, "BASE_DOMAIN=old.example.com\n")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "set", "BASE_DOMAIN", "new.example.com"})

	if _, err := captureStdout(t, func() error { return root.Execute() }); err != nil {
		t.Fatalf("config set failed: %v", err)
	}

	content, readErr := os.ReadFile(filepath.Join(dir, ".env"))
	if readErr != nil {
		t.Fatalf("reading .env: %v", readErr)
	}
	s := string(content)
	if !strings.Contains(s, "new.example.com") {
		t.Errorf("expected updated value in .env, got:\n%s", s)
	}
	if strings.Contains(s, "old.example.com") {
		t.Errorf("old value still present in .env:\n%s", s)
	}
}

// TestConfigSet_AppendsNewKey verifies that 'config set KEY VALUE' appends a
// new key=value line to the .env file when the key does not already exist.
func TestConfigSet_AppendsNewKey(t *testing.T) {
	t.Cleanup(sysexec.Set(&sysexec.Fake{})) // D-0275: v1.5 config set builds; keep it off the host mkcert
	dir := makeNSelfProject(t, "PROJECT_NAME=myapp\n")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "set", "FRESH_KEY", "freshvalue"})

	if _, err := captureStdout(t, func() error { return root.Execute() }); err != nil {
		t.Fatalf("config set failed: %v", err)
	}

	content, readErr := os.ReadFile(filepath.Join(dir, ".env"))
	if readErr != nil {
		t.Fatalf("reading .env: %v", readErr)
	}
	s := string(content)
	if !strings.Contains(s, "FRESH_KEY=freshvalue") {
		t.Errorf("expected FRESH_KEY=freshvalue in .env, got:\n%s", s)
	}
	if !strings.Contains(s, "PROJECT_NAME=myapp") {
		t.Errorf("expected PROJECT_NAME=myapp preserved in .env, got:\n%s", s)
	}
}

// TestConfigSet_EnvFlagWritesToNamedFile verifies that --env dev causes
// 'config set' to write to .env.dev rather than the default .env.
func TestConfigSet_EnvFlagWritesToNamedFile(t *testing.T) {
	t.Cleanup(sysexec.Set(&sysexec.Fake{})) // D-0275: v1.5 config set builds; keep it off the host mkcert
	dir := makeNSelfProject(t, "PROJECT_NAME=myapp\n")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "set", "--env", "dev", "DEV_KEY", "devval"})

	if _, err := captureStdout(t, func() error { return root.Execute() }); err != nil {
		t.Fatalf("config set --env dev failed: %v", err)
	}

	content, readErr := os.ReadFile(filepath.Join(dir, ".env.dev"))
	if readErr != nil {
		t.Fatalf("expected .env.dev to be created, got: %v", readErr)
	}
	if !strings.Contains(string(content), "DEV_KEY=devval") {
		t.Errorf("expected DEV_KEY=devval in .env.dev, got:\n%s", string(content))
	}
}

// ── config list — functional tests ───────────────────────────────────────────

// TestConfigList_NoProject verifies that 'config list' returns a meaningful
// error when not inside an nself project.
func TestConfigList_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"config", "list"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'config list' is run outside an nself project, got nil")
	}
}

// TestConfigList_PrintsHeader verifies that 'config list' prints the KEY /
// VALUE / SOURCE header row when run inside an nself project.
func TestConfigList_PrintsHeader(t *testing.T) {
	makeNSelfProject(t, "PROJECT_NAME=myapp\n")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "list"})

	output, err := captureStdout(t, func() error { return root.Execute() })
	if err != nil {
		t.Fatalf("config list failed: %v", err)
	}
	if !strings.Contains(output, "KEY") {
		t.Errorf("expected 'KEY' header in output, got:\n%s", output)
	}
	if !strings.Contains(output, "VALUE") {
		t.Errorf("expected 'VALUE' header in output, got:\n%s", output)
	}
	if !strings.Contains(output, "SOURCE") {
		t.Errorf("expected 'SOURCE' header in output, got:\n%s", output)
	}
}

// TestConfigList_ShowsKnownKeyValue verifies that 'config list' displays a
// known key that is set in the .env file with its value.
func TestConfigList_ShowsKnownKeyValue(t *testing.T) {
	makeNSelfProject(t, "PROJECT_NAME=listtest\n")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "list"})

	output, err := captureStdout(t, func() error { return root.Execute() })
	if err != nil {
		t.Fatalf("config list failed: %v", err)
	}
	// PROJECT_NAME is a known env var — it should appear in the list output.
	if !strings.Contains(output, "PROJECT_NAME") {
		t.Errorf("expected PROJECT_NAME in list output, got:\n%s", output)
	}
}

// ── config export — functional tests ─────────────────────────────────────────

// TestConfigExport_NoProject verifies that 'config export' returns an error
// when not inside an nself project.
func TestConfigExport_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"config", "export", "output.env"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'config export' is run outside an nself project, got nil")
	}
}

// TestConfigExport_WritesFile verifies that 'config export <file>' creates the
// destination file containing all key=value pairs from the project .env.
func TestConfigExport_WritesFile(t *testing.T) {
	dir := makeNSelfProject(t, "PROJECT_NAME=exporttest\nBASE_DOMAIN=export.dev\n")
	destFile := filepath.Join(dir, "exported.env")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "export", destFile})

	if _, err := captureStdout(t, func() error { return root.Execute() }); err != nil {
		t.Fatalf("config export failed: %v", err)
	}

	content, readErr := os.ReadFile(destFile)
	if readErr != nil {
		t.Fatalf("expected exported file to exist at %s: %v", destFile, readErr)
	}
	s := string(content)
	if !strings.Contains(s, "PROJECT_NAME=exporttest") {
		t.Errorf("expected PROJECT_NAME=exporttest in exported file, got:\n%s", s)
	}
	if !strings.Contains(s, "BASE_DOMAIN=export.dev") {
		t.Errorf("expected BASE_DOMAIN=export.dev in exported file, got:\n%s", s)
	}
}

// TestConfigExport_RequiresArgument verifies that 'config export' without a
// file argument returns a cobra argument error.
func TestConfigExport_RequiresArgument(t *testing.T) {
	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"config", "export"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'config export' is called without a file argument, got nil")
	}
}

// ── config import — functional tests ─────────────────────────────────────────

// TestConfigImport_ForceFlag verifies that 'config import --force' merges
// key=value pairs from the source file into .env without prompting, even when
// existing keys would be overwritten.
func TestConfigImport_ForceFlag(t *testing.T) {
	dir := makeNSelfProject(t, "PROJECT_NAME=original\n")
	srcFile := filepath.Join(dir, "import.env")
	if err := os.WriteFile(srcFile, []byte("PROJECT_NAME=imported\nNEW_KEY=newval\n"), 0644); err != nil {
		t.Fatalf("writing import file: %v", err)
	}

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "import", "--force", srcFile})

	if _, err := captureStdout(t, func() error { return root.Execute() }); err != nil {
		t.Fatalf("config import --force failed: %v", err)
	}

	content, readErr := os.ReadFile(filepath.Join(dir, ".env"))
	if readErr != nil {
		t.Fatalf("reading .env after import: %v", readErr)
	}
	s := string(content)
	if !strings.Contains(s, "PROJECT_NAME=imported") {
		t.Errorf("expected PROJECT_NAME=imported after import, got:\n%s", s)
	}
	if !strings.Contains(s, "NEW_KEY=newval") {
		t.Errorf("expected NEW_KEY=newval after import, got:\n%s", s)
	}
}

// TestConfigImport_MissingSourceFile verifies that 'config import' returns an
// error when the source file does not exist.
func TestConfigImport_MissingSourceFile(t *testing.T) {
	makeNSelfProject(t, "PROJECT_NAME=myapp\n")

	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"config", "import", "--force", "nonexistent-import.env"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when import source file does not exist, got nil")
	}
}

// ── config validate — functional tests ───────────────────────────────────────

// TestConfigValidate_NoProject verifies that 'config validate' returns an error
// when not inside an nself project.
func TestConfigValidate_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newConfigCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"config", "validate"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'config validate' is run outside an nself project, got nil")
	}
}

// TestConfigValidate_ValidProject verifies that 'config validate' runs without
// a "no nself project" error when a valid .env file is present.
func TestConfigValidate_ValidProject(t *testing.T) {
	makeNSelfProject(t, "PROJECT_NAME=validatetest\nBASE_DOMAIN=validate.dev\n")

	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "validate"})

	output, err := captureStdout(t, func() error { return root.Execute() })

	// Acceptable: either all validators pass (nil error) or some fail (non-nil).
	// Unacceptable: "no nself project" error.
	if err != nil && strings.Contains(err.Error(), "no nself project") {
		t.Fatalf("unexpected project-not-found error: %v", err)
	}
	// If it passes, the output should contain at least one [PASS] line.
	if err == nil && !strings.Contains(output, "[PASS]") {
		t.Errorf("expected at least one [PASS] line in validate output, got:\n%s", output)
	}
	t.Logf("config validate output: %s err: %v", output, err)
}
