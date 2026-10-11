package commands

// Purpose: verify recipient command argument and class guards.
// Inputs: fake recipient arguments.
// Outputs: assertions on typed refusals.
// Constraints: no secret store access before guards.
// SPORT: P7-TRUST-03.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/secrets"
)

// TestSecretsRecipientsGuards proves invalid file names and release changes fail before reads.
func TestSecretsRecipientsGuards(t *testing.T) {
	hub := newSecretsRecipientsCmd()
	for _, action := range []string{"add", "remove", "list"} {
		c, _, err := hub.Find([]string{action})
		if err != nil || c.Name() != action {
			t.Fatal("missing recipient leaf")
		}
		if err = c.Flags().Set("file", "team"); err != nil {
			t.Fatal(err)
		}
		c.SetContext(context.Background())
		c.SetOut(&bytes.Buffer{})
		args := []string{"fake-recipient-trust03"}
		if action == "list" {
			args = nil
		}
		resolveErrorCode(t, runSecretsRecipients(c, args), "E540")
		if action != "list" {
			if err = c.Flags().Set("file", "prod"); err != nil {
				t.Fatal(err)
			}
			resolveErrorCode(t, runSecretsRecipients(c, args), "E544")
		}
	}
}

// recipientCommand executes the real Cobra tree from a directory outside the project.
func recipientCommand(t *testing.T, bin, home, key, mode string, args ...string) ([]byte, error) {
	t.Helper()
	argv := append([]string{"secrets", "recipients"}, args...)
	if mode == "1" {
		argv = append([]string{"config"}, argv...)
	}
	cmd := exec.Command(bin, argv...)
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "NSELF_V15="+mode, "HOME="+home, "SECRETS_AGE_KEY_PATH="+key, "NSELF_NONINTERACTIVE=1", "NSELF_NO_UPDATE_CHECK=1")
	return cmd.CombinedOutput()
}

// TestSecretsRecipientsCommand proves add, list, unchanged repeats and project/class guards through Cobra.
func TestSecretsRecipientsCommand(t *testing.T) {
	bin := pilotBinary(t)
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			root, home, key := resolveCommandFixture(t)
			nodePath := filepath.Join(home, "node.key")
			if err := exec.Command("age-keygen", "-o", nodePath).Run(); err != nil {
				t.Fatal(err)
			}
			node, err := secrets.GetPublicKey(nodePath)
			if err != nil {
				t.Fatal(err)
			}
			owner, err := secrets.GetPublicKey(key)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, ".secrets", "ci.age")
			for i, action := range []string{"add", "list", "add"} {
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				args := []string{action, "--project", root, "--file", "ci"}
				if action == "add" {
					args = append(args, node)
				}
				b, err := recipientCommand(t, bin, home, key, mode, args...)
				if err != nil || bytes.Contains(b, []byte(resolveFakeValue)) {
					t.Fatalf("%s command failed: %v %s", action, err, b)
				}
				after, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if action == "list" && (!bytes.Contains(b, []byte(owner)) || !bytes.Contains(b, []byte(node))) {
					t.Fatal("list omitted owner or node")
				}
				if i == 2 && (!bytes.Contains(b, []byte("unchanged")) || !bytes.Equal(before, after)) {
					t.Fatal("repeat add rewrote ciphertext")
				}
			}
			b, err := os.ReadFile(filepath.Join(home, ".nself", "audit.log"))
			if err != nil || bytes.Count(b, []byte(`"event":"secret-recipients"`)) != 1 {
				t.Fatal("add/list/repeat did not produce exactly one change audit", err)
			}
			for _, file := range []string{"prod", "release"} {
				for _, action := range []string{"add", "remove"} {
					b, err := recipientCommand(t, bin, home, key, mode, action, "--project", root, "--file", file, node)
					if err == nil || !bytes.Contains(b, []byte("E544")) {
						t.Fatal("release class accepted without consent", err)
					}
				}
			}
			for _, project := range []string{filepath.Join(home, "missing"), key} {
				b, err := recipientCommand(t, bin, home, key, mode, "list", "--project", project, "--file", "ci")
				if err == nil || !bytes.Contains(b, []byte("E401")) {
					t.Fatal("invalid project accepted", err)
				}
			}
			b, err = recipientCommand(t, bin, home, key, mode, "list", "--project", filepath.Join("..", filepath.Base(root)), "--file", "ci")
			if err != nil || !bytes.Contains(b, []byte(owner)) {
				t.Fatal("relative project was not resolved", err)
			}
		})
	}
}

// TestSecretsRecipientsMalformedList proves planted metadata never prints in either mode or format.
func TestSecretsRecipientsMalformedList(t *testing.T) {
	root, home, key := resolveCommandFixture(t)
	owner, err := secrets.GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	store, err := json.Marshal(map[string]any{"recipients": []string{resolveFakeValue}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".secrets", "ci.age")
	encrypt := exec.Command("age", "-r", owner, "-o", path)
	encrypt.Stdin = strings.NewReader(string(store))
	if err := encrypt.Run(); err != nil {
		t.Fatal(err)
	}
	bin := pilotBinary(t)
	for _, mode := range []string{"0", "1"} {
		for _, jsonMode := range []bool{false, true} {
			args := []string{"list", "--project", root, "--file", "ci"}
			if jsonMode {
				args = append(args, "--json")
			}
			b, err := recipientCommand(t, bin, home, key, mode, args...)
			if err == nil || !bytes.Contains(b, []byte("E543")) || bytes.Contains(b, []byte(resolveFakeValue)) {
				t.Fatalf("mode=%s json=%v leaked or accepted metadata: %v", mode, jsonMode, err)
			}
		}
	}
}
