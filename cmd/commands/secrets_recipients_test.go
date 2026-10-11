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

// symlinkStoreFixture plants links only inside isolated, real-age stores.
func symlinkStoreFixture(t *testing.T, kind string) (root, home, key, target, ref string) {
	t.Helper()
	root, home, key = resolveCommandFixture(t)
	storeDir := filepath.Join(root, ".secrets")
	ref = "project/FOO"
	if err := secrets.Set(root, "release", "FOO", resolveFakeValue); err != nil {
		t.Fatal(err)
	}
	target = filepath.Join(storeDir, "release.age")
	link := filepath.Join(storeDir, "ci.age")
	switch kind {
	case "file":
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
	case "directory":
		actual := filepath.Join(root, "trusted-secrets")
		if err := os.Rename(storeDir, actual); err != nil {
			t.Fatal(err)
		}
		link, target = storeDir, actual
	case "team-file", "team-directory":
		teamDir := filepath.Join(home, ".config", "nself", "secrets")
		if err := os.MkdirAll(teamDir, 0700); err != nil {
			t.Fatal(err)
		}
		link, ref = filepath.Join(teamDir, "team.age"), "team/FOO"
		if kind == "team-directory" {
			if err := os.Remove(teamDir); err != nil {
				t.Fatal(err)
			}
			link, target = teamDir, storeDir
		}
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return
}

// refusedResolveCommand checks the real process stdout, stderr and audit for values.
func refusedResolveCommand(t *testing.T, bin, root, home, key, mode, ref string) {
	t.Helper()
	args := []string{"secrets", "resolve", "--project", root, "--for-attempt", "a1", "--json", ref}
	if mode == "1" {
		args = append([]string{"config"}, args...)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "NSELF_V15="+mode, "HOME="+home, "SECRETS_AGE_KEY_PATH="+key, "NSELF_NONINTERACTIVE=1", "NSELF_NO_UPDATE_CHECK=1")
	b, err := cmd.CombinedOutput()
	if err == nil || !bytes.Contains(b, []byte("E543")) || bytes.Contains(b, []byte(resolveFakeValue)) {
		t.Fatal("symlink resolve accepted or emitted a value", err)
	}
	audit, err := os.ReadFile(filepath.Join(home, ".nself", "audit.log"))
	if err != nil || bytes.Contains(audit, []byte(resolveFakeValue)) {
		t.Fatal("symlink resolve audit missing or leaked value", err)
	}
}

// TestSecretsSymlinkStoresRealCommand proves both CLI modes refuse links before age I/O.
func TestSecretsSymlinkStoresRealCommand(t *testing.T) {
	bin := pilotBinary(t)
	for _, mode := range []string{"0", "1"} {
		for _, kind := range []string{"file", "directory", "team-file", "team-directory"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				root, home, key, target, ref := symlinkStoreFixture(t, kind)
				if kind == "directory" {
					target = filepath.Join(target, "release.age")
				} else if kind == "team-directory" {
					target = filepath.Join(target, "release.age")
				}
				before, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "file" || kind == "directory" {
					nodeKey := filepath.Join(home, "node.key")
					if err := exec.Command("age-keygen", "-o", nodeKey).Run(); err != nil {
						t.Fatal(err)
					}
					node, err := secrets.GetPublicKey(nodeKey)
					if err != nil {
						t.Fatal(err)
					}
					b, err := recipientCommand(t, bin, home, key, mode, "add", "--project", root, "--file", "ci", node)
					if err == nil || !bytes.Contains(b, []byte("E543")) || bytes.Contains(b, []byte(resolveFakeValue)) {
						t.Fatal("symlink grant accepted or leaked value", err)
					}
				}
				refusedResolveCommand(t, bin, root, home, key, mode, ref)
				after, err := os.ReadFile(target)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("symlink refusal changed release ciphertext", err)
				}
			})
		}
	}
}

// TestSecretsResolveInvalidAuditRealCommand proves pasted values never reach command audit logs.
func TestSecretsResolveInvalidAuditRealCommand(t *testing.T) {
	bin := pilotBinary(t)
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			root, home, key := resolveCommandFixture(t)
			badRef, raw, attempt := "project/fake-adv-value-xyz", "fake-raw-value-trust03", "fake/attempt-value-trust03"
			args := []string{"secrets", "resolve", "--project", root, "--for-attempt", attempt, "--json", "project/FOO", badRef, raw}
			if mode == "1" {
				args = append([]string{"config"}, args...)
			}
			cmd := exec.Command(bin, args...)
			cmd.Dir = home
			cmd.Env = append(os.Environ(), "NSELF_V15="+mode, "HOME="+home, "SECRETS_AGE_KEY_PATH="+key, "NSELF_NONINTERACTIVE=1", "NSELF_NO_UPDATE_CHECK=1")
			b, err := cmd.CombinedOutput()
			if err == nil || !bytes.Contains(b, []byte("E401")) {
				t.Fatal("invalid attempt was accepted", err)
			}
			audit, err := os.ReadFile(filepath.Join(home, ".nself", "audit.log"))
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{badRef, raw, attempt, resolveFakeValue} {
				if bytes.Contains(audit, []byte(forbidden)) {
					t.Fatal("invalid input or value entered audit")
				}
			}
			if !bytes.Contains(audit, []byte(`"invalid_refs":"2"`)) || !bytes.Contains(audit, []byte("invalid_ref_hashes")) || !bytes.Contains(audit, []byte("attempt_id_sha256")) || !bytes.Contains(audit, []byte("project/FOO")) {
				t.Fatal("audit omitted valid reference or invalid-input metadata")
			}
		})
	}
}
