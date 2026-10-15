package commands

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newServiceCmd returns a fresh cobra.Command tree containing only the service
// command and its subcommands so tests run in isolation.
func newServiceCmd() *cobra.Command {
	root := &cobra.Command{Use: "nself", RunE: func(cmd *cobra.Command, args []string) error { return nil }}

	sc := &cobra.Command{
		Use:   "service",
		Short: "Manage optional services",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	sc.PersistentFlags().String("env", "", "Target environment (reads .env.{env})")

	listCmd := &cobra.Command{Use: "list", Short: "List all optional services", RunE: runServiceList}
	listCmd.Flags().Bool("json", false, "Output as JSON array")

	enableCmd := &cobra.Command{Use: "enable <name>", Short: "Enable an optional service", Args: cobra.ExactArgs(1), RunE: runServiceEnable}
	disableCmd := &cobra.Command{Use: "disable <name>", Short: "Disable an optional service", Args: cobra.ExactArgs(1), RunE: runServiceDisable}

	sc.AddCommand(listCmd)
	sc.AddCommand(enableCmd)
	sc.AddCommand(disableCmd)
	root.AddCommand(sc)
	return root
}

// TestServiceCmd_Registered verifies that the service subcommand is registered
// on the global RootCmd.
func TestServiceCmd_Registered(t *testing.T) {
	found := false
	for _, c := range RootCmd.Commands() {
		if c.Name() == "service" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'service' to be registered on RootCmd")
	}
}

// TestServiceEnableCmd_Registered verifies that 'service enable' is wired as a
// subcommand of 'service'.
func TestServiceEnableCmd_Registered(t *testing.T) {
	var serviceParent *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "service" {
			serviceParent = c
			break
		}
	}
	if serviceParent == nil {
		t.Fatal("service command not found on RootCmd")
	}

	found := false
	for _, sub := range serviceParent.Commands() {
		if sub.Name() == "enable" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'service enable' subcommand to be registered")
	}
}

// TestServiceDisableCmd_Registered verifies that 'service disable' is wired as
// a subcommand of 'service'.
func TestServiceDisableCmd_Registered(t *testing.T) {
	var serviceParent *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "service" {
			serviceParent = c
			break
		}
	}
	if serviceParent == nil {
		t.Fatal("service command not found on RootCmd")
	}

	found := false
	for _, sub := range serviceParent.Commands() {
		if sub.Name() == "disable" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'service disable' subcommand to be registered")
	}
}

// TestServiceEnable_RequiresArgument verifies that 'service enable' returns an
// error when called without a service name argument.
func TestServiceEnable_RequiresArgument(t *testing.T) {
	root := newServiceCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"service", "enable"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'service enable' is called without arguments, got nil")
	}
}

// TestServiceDisable_RequiresArgument verifies that 'service disable' returns
// an error when called without a service name argument.
func TestServiceDisable_RequiresArgument(t *testing.T) {
	root := newServiceCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"service", "disable"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'service disable' is called without arguments, got nil")
	}
}

// TestServiceEnable_UnknownService verifies that 'service enable' with an
// unrecognised service name returns an error.
func TestServiceEnable_UnknownService(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newServiceCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"service", "enable", "notaservice"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error for an unknown service name, got nil")
	}
	if !strings.Contains(err.Error(), "unknown service") {
		t.Errorf("expected error to mention 'unknown service', got: %v", err)
	}
}

// TestServiceDisable_UnknownService verifies that 'service disable' with an
// unrecognised service name returns an error.
func TestServiceDisable_UnknownService(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newServiceCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"service", "disable", "notaservice"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error for an unknown service name, got nil")
	}
	if !strings.Contains(err.Error(), "unknown service") {
		t.Errorf("expected error to mention 'unknown service', got: %v", err)
	}
}

// TestServiceEnable_KnownService verifies that 'service enable redis' succeeds
// and writes REDIS_ENABLED=true to the .env file in the working directory.
// The service command does not require an nself project root — it just uses cwd.
func TestServiceEnable_KnownService(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newServiceCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"service", "enable", "redis"})
	err := root.Execute()

	if err != nil {
		t.Fatalf("unexpected error enabling redis: %v", err)
	}

	// Verify that REDIS_ENABLED=true was written to .env in the temp dir.
	content, readErr := os.ReadFile(".env")
	if readErr != nil {
		t.Fatalf("expected .env to be created, got: %v", readErr)
	}
	if !strings.Contains(string(content), "REDIS_ENABLED=true") {
		t.Errorf("expected REDIS_ENABLED=true in .env, got: %s", string(content))
	}
}

// TestServiceDisable_KnownService verifies that 'service disable minio' sets
// MINIO_ENABLED=false in the .env file.
func TestServiceDisable_KnownService(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newServiceCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"service", "disable", "minio"})
	err := root.Execute()

	if err != nil {
		t.Fatalf("unexpected error disabling minio: %v", err)
	}

	content, readErr := os.ReadFile(".env")
	if readErr != nil {
		t.Fatalf("expected .env to be created, got: %v", readErr)
	}
	if !strings.Contains(string(content), "MINIO_ENABLED=false") {
		t.Errorf("expected MINIO_ENABLED=false in .env, got: %s", string(content))
	}
}

// TestServiceEnable_Alias verifies that a service alias (e.g. "storage") is
// resolved to its canonical name (minio) and writes the correct env var.
func TestServiceEnable_Alias(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newServiceCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"service", "enable", "storage"})
	err := root.Execute()

	if err != nil {
		t.Fatalf("unexpected error enabling storage (alias for minio): %v", err)
	}

	content, readErr := os.ReadFile(".env")
	if readErr != nil {
		t.Fatalf("expected .env to be created, got: %v", readErr)
	}
	if !strings.Contains(string(content), "MINIO_ENABLED=true") {
		t.Errorf("expected MINIO_ENABLED=true when enabling via alias 'storage', got: %s", string(content))
	}
}

// TestServiceEnable_TooManyArguments verifies that 'service enable' rejects
// more than one positional argument.
func TestServiceEnable_TooManyArguments(t *testing.T) {
	root := newServiceCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"service", "enable", "redis", "minio"})
	err := root.Execute()

	if err == nil {
		t.Fatal("expected an error when 'service enable' is called with two arguments, got nil")
	}
}

// TestCanonicalServiceName_Valid verifies that known service names resolve
// correctly to their canonical entry.
func TestCanonicalServiceName_Valid(t *testing.T) {
	cases := []struct {
		input    string
		wantName string
		wantVar  string
	}{
		{"redis", "redis", "REDIS_ENABLED"},
		{"minio", "minio", "MINIO_ENABLED"},
		{"storage", "minio", "MINIO_ENABLED"},   // alias
		{"email", "email", "MAILPIT_ENABLED"},   // canonical name (was "mailpit" before T05 rename)
		{"mailpit", "email", "MAILPIT_ENABLED"}, // alias
		{"mail", "email", "MAILPIT_ENABLED"},    // alias
		{"search", "search", "SEARCH_ENABLED"},
		{"meilisearch", "search", "SEARCH_ENABLED"}, // alias
		{"monitoring", "monitoring", "MONITORING_ENABLED"},
		{"admin", "admin", "NSELF_ADMIN_ENABLED"},
	}

	for _, tc := range cases {
		name, entry, err := canonicalServiceName(tc.input)
		if err != nil {
			t.Errorf("canonicalServiceName(%q) returned error: %v", tc.input, err)
			continue
		}
		if name != tc.wantName {
			t.Errorf("canonicalServiceName(%q) name = %q, want %q", tc.input, name, tc.wantName)
		}
		if entry.EnvVar != tc.wantVar {
			t.Errorf("canonicalServiceName(%q) EnvVar = %q, want %q", tc.input, entry.EnvVar, tc.wantVar)
		}
	}
}

// TestCanonicalServiceName_Invalid verifies that an unrecognised service name
// returns an error.
func TestCanonicalServiceName_Invalid(t *testing.T) {
	_, _, err := canonicalServiceName("notaservice")
	if err == nil {
		t.Fatal("expected an error for unknown service name, got nil")
	}
	if !strings.Contains(err.Error(), "unknown service") {
		t.Errorf("expected 'unknown service' in error, got: %v", err)
	}
}

// TestCanonicalServiceName_StoragePlugin guards D-0451: once the project's
// compose file declares the storage plugin, `storage` no longer toggles MinIO.
func TestCanonicalServiceName_StoragePlugin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/docker-compose.yml", []byte("services:\n  storage:\n    image: nself/storage\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if _, _, err := canonicalServiceName("storage"); err == nil || !strings.Contains(err.Error(), "storage plugin") {
		t.Fatalf("storage with a declared plugin must be refused, got %v", err)
	}
	if name, _, err := canonicalServiceName("minio"); err != nil || name != "minio" {
		t.Fatalf("minio must still resolve, got %q, %v", name, err)
	}
}

// TestSetEnvKeyInFile_NewFile verifies that setEnvKeyInFile creates a file and
// writes the key=value line when the file does not exist.
func TestSetEnvKeyInFile_NewFile(t *testing.T) {
	dir := t.TempDir()
	envFile := dir + "/.env"

	if err := setEnvKeyInFile(envFile, "REDIS_ENABLED", "true"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, readErr := os.ReadFile(envFile)
	if readErr != nil {
		t.Fatalf("could not read created file: %v", readErr)
	}
	if !strings.Contains(string(content), "REDIS_ENABLED=true") {
		t.Errorf("expected REDIS_ENABLED=true in file, got: %s", string(content))
	}
}

// TestSetEnvKeyInFile_UpdateExisting verifies that setEnvKeyInFile updates an
// existing line in place.
func TestSetEnvKeyInFile_UpdateExisting(t *testing.T) {
	dir := t.TempDir()
	envFile := dir + "/.env"

	if err := os.WriteFile(envFile, []byte("REDIS_ENABLED=false\n"), 0644); err != nil {
		t.Fatalf("could not seed file: %v", err)
	}

	if err := setEnvKeyInFile(envFile, "REDIS_ENABLED", "true"); err != nil {
		t.Fatalf("unexpected error on update: %v", err)
	}

	content, readErr := os.ReadFile(envFile)
	if readErr != nil {
		t.Fatalf("could not read file: %v", readErr)
	}
	s := string(content)
	if strings.Contains(s, "REDIS_ENABLED=false") {
		t.Errorf("old value should have been replaced, got: %s", s)
	}
	if !strings.Contains(s, "REDIS_ENABLED=true") {
		t.Errorf("expected REDIS_ENABLED=true in file, got: %s", s)
	}
}

// TestReadEnvValues_ExistingFile verifies that readEnvValues correctly parses
// an existing env file and returns all key=value pairs.
func TestReadEnvValues_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	envFile := dir + "/.env"
	if err := os.WriteFile(envFile, []byte("FOO=bar\nBAZ=qux\n"), 0644); err != nil {
		t.Fatalf("writing .env: %v", err)
	}

	values, err := readEnvValues(envFile)
	if err != nil {
		t.Fatalf("readEnvValues: %v", err)
	}
	if values["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", values["FOO"], "bar")
	}
	if values["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want %q", values["BAZ"], "qux")
	}
}

// TestReadEnvValues_NonExistentFile verifies that readEnvValues returns an
// empty map (not an error) when the file does not exist.
func TestReadEnvValues_NonExistentFile(t *testing.T) {
	values, err := readEnvValues(t.TempDir() + "/does-not-exist.env")
	if err != nil {
		t.Fatalf("readEnvValues on non-existent file: %v", err)
	}
	if len(values) != 0 {
		t.Errorf("expected empty map for non-existent file, got %d entries", len(values))
	}
}

// TestServiceEnable_PreexistingEnvFile verifies that 'service enable' correctly
// updates an existing .env file that already contains other key=value pairs,
// covering the godotenv.Read branch of readEnvValues.
func TestServiceEnable_PreexistingEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := dir + "/.env"
	if err := os.WriteFile(envFile, []byte("PROJECT_NAME=test\nREDIS_ENABLED=false\n"), 0644); err != nil {
		t.Fatalf("writing seed .env: %v", err)
	}
	t.Chdir(dir)

	root := newServiceCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"service", "enable", "redis"})
	if err := root.Execute(); err != nil {
		t.Fatalf("service enable: %v", err)
	}

	content, readErr := os.ReadFile(envFile)
	if readErr != nil {
		t.Fatalf("reading .env after enable: %v", readErr)
	}
	s := string(content)
	if !strings.Contains(s, "REDIS_ENABLED=true") {
		t.Errorf("expected REDIS_ENABLED=true in .env, got: %s", s)
	}
	// Unrelated keys must be preserved.
	if !strings.Contains(s, "PROJECT_NAME=test") {
		t.Errorf("expected PROJECT_NAME=test to be preserved, got: %s", s)
	}
}

// TestServiceList_NoEnvFile verifies that 'service list' succeeds even when no
// .env file exists — all services should appear as "disabled".
func TestServiceList_NoEnvFile(t *testing.T) {
	t.Chdir(t.TempDir())

	root := newServiceCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)

	root.SetArgs([]string{"service", "list"})
	err := root.Execute()

	if err != nil {
		t.Fatalf("expected 'service list' to succeed without a .env file, got: %v", err)
	}
}
