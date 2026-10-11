package secrets

// Purpose: verify recipient grants and node identity isolation.
// Inputs: fake stores and ephemeral operator and node identities.
// Outputs: assertions on re-encryption, guards and audit records.
// Constraints: no committed private identities.
// SPORT: P7-TRUST-03.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// trustEncryptStore supports isolated secret contract checks.
func trustEncryptStore(t *testing.T, root, env, pub, plaintext string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".secrets"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("age", "-r", pub, "-o", filepath.Join(root, ".secrets", env+".age"))
	cmd.Stdin = strings.NewReader(plaintext)
	if err := cmd.Run(); err != nil {
		t.Fatal("encrypt fixture:", err)
	}
}

// TestRecipientsAddReencrypt verifies the secret contract.
func TestRecipientsAddReencrypt(t *testing.T) {
	root, home, key := trustFixture(t)
	trustStore(t, root, "ci", "FOO")
	nodePath := filepath.Join(home, "node.key")
	node := trustKey(t, nodePath)
	operator, err := GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".secrets", "ci.age")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	sink := func(event string, fields map[string]string) error {
		calls++
		if event != "secret-recipients" || fields["file"] != "ci" || fields["recipient"] != node || fields["project"] != root || fields["user"] == "" || len(fields) != 5 {
			t.Errorf("bad audit %s %+v", event, fields)
		}
		return nil
	}
	req := RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}
	rows, changed, err := Recipients(req, sink)
	if err != nil || !changed || !slices.Contains(rows, node) || !slices.Contains(rows, operator) || calls != 1 {
		t.Fatalf("add: changed=%v calls=%d err=%v", changed, calls, err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(before, after) {
		t.Fatal("ciphertext unchanged")
	}
	_, changed, err = Recipients(req, sink)
	if err != nil || changed || calls != 1 {
		t.Fatal("duplicate add rewrote or audited")
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, again) {
		t.Fatal("duplicate changed ciphertext")
	}
	rows, changed, err = Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "list"}, nil)
	if err != nil || changed || !slices.Contains(rows, node) {
		t.Fatal("list failed", err)
	}
	req.Action = "remove"
	req.Recipient = operator
	_, _, err = Recipients(req, sink)
	trustCode(t, err, "E401")
	req.Recipient = node
	rows, changed, err = Recipients(req, sink)
	if err != nil || !changed || slices.Contains(rows, node) || calls != 2 {
		t.Fatal("remove failed", err)
	}
	t.Setenv("SECRETS_AGE_KEY_PATH", nodePath)
	_, err = Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: []string{"project/FOO"}}, func(string, map[string]string) error { return nil })
	trustCode(t, err, "E543")
}

// TestReservedNames verifies the secret contract.
func TestReservedNames(t *testing.T) {
	for _, env := range []string{"ci", "release", "team", "ci-prod", "ci-", "", "../prod", "Prod"} {
		for _, class := range []string{"deploy", "environment"} {
			_, err := ParseRef(class + "/" + env + "/X")
			trustCode(t, err, "E540")
		}
	}
	for _, file := range []string{"team", "ci-ci", "ci-release", "ci-team", "ci-ci-prod", "ci-", "../prod", "prod.age", "PROD"} {
		_, _, err := Recipients(RecipientsRequest{ProjectRoot: t.TempDir(), File: file, Action: "add", Recipient: "-invalid"}, nil)
		trustCode(t, err, "E540")
	}
	for _, file := range []string{"prod", "release"} {
		_, _, err := Recipients(RecipientsRequest{ProjectRoot: t.TempDir(), File: file, Action: "add", Recipient: "-invalid"}, nil)
		trustCode(t, err, "E544")
	}
	_, _, err := Recipients(RecipientsRequest{ProjectRoot: t.TempDir(), File: "ci", Action: "add", Recipient: "--evil"}, nil)
	trustCode(t, err, "E401")
}

// TestNodeIdentityMatrix verifies the secret contract.
func TestNodeIdentityMatrix(t *testing.T) {
	root, home, _ := trustFixture(t)
	trustStore(t, root, "ci", "FOO")
	trustStore(t, root, "prod", "X")
	nodePath := filepath.Join(home, "node.key")
	node := trustKey(t, nodePath)
	req := RecipientsRequest{ProjectRoot: root, File: "prod", Action: "add", Recipient: node}
	_, _, err := Recipients(req, func(string, map[string]string) error { return nil })
	trustCode(t, err, "E544")
	req.File = "ci"
	_, changed, err := Recipients(req, func(string, map[string]string) error { return nil })
	if err != nil || !changed {
		t.Fatal(err)
	}
	t.Setenv("SECRETS_AGE_KEY_PATH", nodePath)
	for _, tc := range []struct {
		ref  string
		code string
	}{
		{ref: "project/FOO"},
		{ref: "deploy/prod/X", code: "E543"},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			v, err := Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: []string{tc.ref}}, func(string, map[string]string) error { return nil })
			if tc.code != "" {
				trustCode(t, err, tc.code)
				if v != nil {
					t.Fatal("node received release values")
				}
				return
			}
			if err != nil || len(v) != 1 || v[0].Value != trustFakeValue {
				t.Fatalf("node project access failed: %v", err)
			}
		})
	}
}

// TestRecipientsEmptySeedAndMissing verifies the secret contract.
func TestRecipientsEmptySeedAndMissing(t *testing.T) {
	root, home, key := trustFixture(t)
	node := trustKey(t, filepath.Join(home, "node.key"))
	req := RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}
	_, _, err := Recipients(req, func(string, map[string]string) error { return nil })
	trustCode(t, err, "E541")
	// saveStore normally populates recipients, so encrypt an old empty-list store.
	pub, err := GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	trustEncryptStore(t, root, "ci", pub, `{"secrets":{"FOO":{"value":"fake-old-store-trust03"}},"recipients":[]}`)
	rows, changed, err := Recipients(req, func(string, map[string]string) error { return nil })
	if err != nil || !changed || !slices.Contains(rows, pub) || !slices.Contains(rows, node) {
		t.Fatal("operator not seeded", err)
	}
}

// TestRecipientsLastRecipient verifies the secret contract.
func TestRecipientsLastRecipient(t *testing.T) {
	store := &SecretStore{Recipients: []string{"fake-node-trust03"}}
	_, err := changeRecipient(store, RecipientsRequest{Action: "remove", Recipient: "fake-node-trust03"}, "fake-operator-trust03")
	trustCode(t, err, "E401")
}
