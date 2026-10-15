package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestD0614ArchitectureDeployKey(t *testing.T) {
	compattest.Set(t, false)
	dir := t.TempDir()
	log := filepath.Join(dir, "argv")
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > '"+log+"'\nprintf 'amd64\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("NSELF_DEPLOY_SSH_KEY", "")
	t.Setenv("NSELF_DEPLOY_KEY_PATH", filepath.Join(dir, "deploy-key"))
	if _, err := NewSSHProber(dir, true).Architecture(context.Background(), Server{Name: "ops", Host: "u@ops.example.test"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\n"+filepath.Join(dir, "deploy-key")+"\n") {
		t.Fatalf("wrong SSH key: %s", data)
	}
}
