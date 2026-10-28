package secrets

// Purpose: verify recipient grants and node identity isolation.
// Inputs: fake stores and ephemeral operator and node identities.
// Outputs: assertions on re-encryption, guards and audit records.
// Constraints: no committed private identities.
// SPORT: P7-TRUST-03.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
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

// TestRecipientsAddReencrypt proves grants preserve owner access and repeats do not rewrite.
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
	values, err := Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: []string{"project/FOO"}}, func(string, map[string]string) error { return nil })
	if err != nil || len(values) != 1 || values[0].Value != trustFakeValue {
		t.Fatal("owner lost decryption access after grant", err)
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
	trustCode(t, err, "E544")
	req.Recipient = node
	rows, changed, err = Recipients(req, sink)
	if err != nil || !changed || slices.Contains(rows, node) || calls != 2 {
		t.Fatal("remove failed", err)
	}
	t.Setenv("SECRETS_AGE_KEY_PATH", nodePath)
	_, err = Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: []string{"project/FOO"}}, func(string, map[string]string) error { return nil })
	trustCode(t, err, "E543")
}

// TestReservedNames proves reserved store names and unsafe recipient argv are refused.
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

// TestNodeIdentityMatrix proves a ci grant allows project reads but no deploy reads.
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

// TestRecipientsEmptySeedAndMissing proves missing stores and empty metadata are refused.
func TestRecipientsEmptySeedAndMissing(t *testing.T) {
	root, home, key := trustFixture(t)
	node := trustKey(t, filepath.Join(home, "node.key"))
	req := RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}
	_, _, err := Recipients(req, func(string, map[string]string) error { return nil })
	trustCode(t, err, "E541")
	// Encrypt legacy empty metadata directly; recipient changes must not seed it.
	pub, err := GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	trustEncryptStore(t, root, "ci", pub, `{"secrets":{"FOO":{"value":"fake-old-store-trust03"}},"recipients":[]}`)
	rows, changed, err := Recipients(req, func(string, map[string]string) error { return nil })
	trustCode(t, err, "E543")
	if rows != nil || changed {
		t.Fatal("empty metadata was seeded")
	}
}

// TestRecipientsLastRecipient proves even the sole owner cannot be removed.
func TestRecipientsLastRecipient(t *testing.T) {
	store := &SecretStore{Recipients: []string{"fake-node-trust03"}}
	_, err := changeRecipient(store, RecipientsRequest{Action: "remove", Recipient: "fake-node-trust03"}, "fake-node-trust03")
	trustCode(t, err, "E544")
}

// TestRecipientsMalformedMetadata proves all actions refuse every invalid stored recipient.
func TestRecipientsMalformedMetadata(t *testing.T) {
	root, home, key := trustFixture(t)
	owner, err := GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	node := trustKey(t, filepath.Join(home, "node.key"))
	for _, metadata := range []string{`[]`, `null`, `["` + trustFakeValue + `"]`, `["` + owner + `","` + trustFakeValue + `"]`} {
		trustEncryptStore(t, root, "ci", owner, `{"recipients":`+metadata+`}`)
		for _, action := range []string{"list", "add", "remove"} {
			req := RecipientsRequest{ProjectRoot: root, File: "ci", Action: action, Recipient: owner}
			if action == "remove" {
				req.Recipient = node
			}
			rows, changed, err := Recipients(req, func(string, map[string]string) error {
				t.Fatal("invalid metadata reached audit")
				return nil
			})
			trustCode(t, err, "E543")
			if rows != nil || changed || err.Error() != decryptError().Error() {
				t.Fatal("invalid metadata escaped or changed the store")
			}
		}
	}
}

// TestRecipientsOwnerOnly proves nodes and inconsistent metadata cannot rewrite ownership.
func TestRecipientsOwnerOnly(t *testing.T) {
	root, home, key := trustFixture(t)
	trustStore(t, root, "ci", "FOO")
	owner, err := GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	nodePath := filepath.Join(home, "node.key")
	node := trustKey(t, nodePath)
	third := trustKey(t, filepath.Join(home, "third.key"))
	sink := func(string, map[string]string) error { return nil }
	if _, _, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}, sink); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, identity, action, recipient string }{
		{"node removes owner", nodePath, "remove", owner},
		{"node grants third", nodePath, "add", third},
		{"owner removes owner", key, "remove", owner},
		{"inconsistent owner", key, "add", third},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SECRETS_AGE_KEY_PATH", tc.identity)
			if tc.name == "inconsistent owner" {
				trustEncryptStore(t, root, "ci", owner, `{"recipients":["`+node+`"]}`)
			}
			path := filepath.Join(root, ".secrets", "ci.age")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			rows, changed, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: tc.action, Recipient: tc.recipient}, func(string, map[string]string) error {
				t.Fatal("unauthorized change reached audit")
				return nil
			})
			trustCode(t, err, "E544")
			after, readErr := os.ReadFile(path)
			if readErr != nil || rows != nil || changed || !reflect.DeepEqual(before, after) {
				t.Fatal("unauthorized change altered ciphertext", readErr)
			}
		})
	}
}

// TestRecipientsAuditBeforeSave proves failed audit cannot rewrite and save errors stay fixed.
func TestRecipientsAuditBeforeSave(t *testing.T) {
	root, home, _ := trustFixture(t)
	trustStore(t, root, "ci", "FOO")
	node := trustKey(t, filepath.Join(home, "node.key"))
	path := filepath.Join(root, ".secrets", "ci.age")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	req := RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}
	for _, failSink := range []bool{true, false} {
		if !failSink && runtime.GOOS == "windows" {
			continue // a read-only directory does not refuse file creation on Windows
		}
		calls := 0
		rows, changed, err := Recipients(req, func(event string, fields map[string]string) error {
			calls++
			atAudit, err := os.ReadFile(path)
			if err != nil || !reflect.DeepEqual(before, atAudit) || event != "secret-recipients" || fields["action"] != "add" {
				t.Fatal("audit did not precede save", err)
			}
			if failSink {
				return errors.New(trustFakeValue)
			}
			// Deny the subsequent temp-file create after the audit succeeds.
			if os.Geteuid() == 0 {
				t.Fatal("write refusal proof must run as non-root")
			}
			return os.Chmod(filepath.Dir(path), 0500)
		})
		code := "E543"
		if failSink {
			code = "E401"
		}
		trustCode(t, err, code)
		if !failSink && err.Error() != decryptError().Error() {
			t.Fatal("save error was not fixed")
		}
		after, readErr := os.ReadFile(path)
		if readErr != nil || calls != 1 || rows != nil || changed || !reflect.DeepEqual(before, after) {
			t.Fatal("failed audit or save altered ciphertext", readErr)
		}
		if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
	}
}

// TestRecipientsForgedOwnerComment proves comments cannot authorize node grants.
func TestRecipientsForgedOwnerComment(t *testing.T) {
	root, home, key := trustFixture(t)
	trustStore(t, root, "ci", "FOO")
	owner, err := GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	nodePath := filepath.Join(home, "node.key")
	node := trustKey(t, nodePath)
	third := trustKey(t, filepath.Join(home, "third.key"))
	sink := func(string, map[string]string) error { return nil }
	if _, _, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}, sink); err != nil {
		t.Fatal(err)
	}
	identity, err := os.ReadFile(nodePath)
	if err != nil {
		t.Fatal(err)
	}
	identity = []byte(strings.Replace(string(identity), "# public key: "+node, "# public key: "+owner, 1))
	if err := os.WriteFile(nodePath, identity, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SECRETS_AGE_KEY_PATH", nodePath)
	path := filepath.Join(root, ".secrets", "ci.age")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, changed, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: third}, sink)
	trustCode(t, err, "E544")
	after, readErr := os.ReadFile(path)
	if readErr != nil || rows != nil || changed || !reflect.DeepEqual(before, after) {
		t.Fatal("forged comment altered ciphertext", readErr)
	}
}

// TestMappedStoreSymlinks proves both operations refuse redirected real age stores.
func TestMappedStoreSymlinks(t *testing.T) {
	for _, kind := range []string{"file", "directory", "team file", "team directory"} {
		t.Run(kind, func(t *testing.T) {
			root, home, _ := trustFixture(t)
			trustStore(t, root, "release", "X")
			node := trustKey(t, filepath.Join(home, "node.key"))
			release := filepath.Join(root, ".secrets", "release.age")
			before, err := os.ReadFile(release)
			if err != nil {
				t.Fatal(err)
			}
			ref := "project/X"
			var link, target string
			switch kind {
			case "file":
				link, target = filepath.Join(root, ".secrets", "ci.age"), "release.age"
			case "directory":
				original := root
				root = t.TempDir()
				link, target = filepath.Join(root, ".secrets"), filepath.Join(original, ".secrets")
			case "team file", "team directory":
				ref = "team/X"
				dir := filepath.Join(home, ".config", "nself", "secrets")
				if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
					t.Fatal(err)
				}
				if kind == "team directory" {
					link, target = dir, filepath.Dir(release)
				} else {
					if err := os.Mkdir(dir, 0700); err != nil {
						t.Fatal(err)
					}
					link, target = filepath.Join(dir, "team.age"), release
				}
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			sink := func(string, map[string]string) error { return nil }
			if !strings.HasPrefix(kind, "team") {
				rows, changed, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}, sink)
				trustCode(t, err, "E543")
				if rows != nil || changed || err.Error() != decryptError().Error() {
					t.Fatal("redirected grant escaped fixed refusal")
				}
			}
			values, err := Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: []string{ref}}, sink)
			trustCode(t, err, "E543")
			if values != nil || err.Error() != decryptError().Error() {
				t.Fatal("redirected resolve returned values")
			}
			after, readErr := os.ReadFile(release)
			if readErr != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("redirected operation altered release ciphertext", readErr)
			}
		})
	}
}

// TestRecipientsSaveSymlinkRefusal proves the path is checked again before saving.
func TestRecipientsSaveSymlinkRefusal(t *testing.T) {
	root, home, _ := trustFixture(t)
	trustStore(t, root, "ci", "X")
	trustStore(t, root, "release", "X")
	node := trustKey(t, filepath.Join(home, "node.key"))
	path := filepath.Join(root, ".secrets", "ci.age")
	release := filepath.Join(root, ".secrets", "release.age")
	before, err := os.ReadFile(release)
	if err != nil {
		t.Fatal(err)
	}
	rows, changed, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}, func(string, map[string]string) error {
		if err := os.Rename(path, path+".original"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("release.age", path); err != nil {
			t.Fatal(err)
		}
		return nil
	})
	trustCode(t, err, "E543")
	after, readErr := os.ReadFile(release)
	if readErr != nil || rows != nil || changed || !reflect.DeepEqual(before, after) {
		t.Fatal("save followed replacement symlink", readErr)
	}
}

// TestMappedStoreNonregularAndMissing preserves fixed refusal and missing codes.
func TestMappedStoreNonregularAndMissing(t *testing.T) {
	for _, kind := range []string{"missing directory", "missing file", "directory file"} {
		t.Run(kind, func(t *testing.T) {
			root, home, _ := trustFixture(t)
			node := trustKey(t, filepath.Join(home, "node.key"))
			path := filepath.Join(root, ".secrets", "ci.age")
			code := "E541"
			if kind != "missing directory" {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "directory file" {
				code = "E543"
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			sink := func(string, map[string]string) error { return nil }
			rows, changed, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}, sink)
			trustCode(t, err, code)
			if rows != nil || changed {
				t.Fatal("invalid store granted access")
			}
			values, err := Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: []string{"project/X"}}, sink)
			trustCode(t, err, code)
			if values != nil {
				t.Fatal("invalid store returned values")
			}
		})
	}
}

// TestRecipientsConcurrentChanges holds the first writer at audit, after load.
func TestRecipientsConcurrentChanges(t *testing.T) {
	root, home, _ := trustFixture(t)
	trustStore(t, root, "ci", "FOO")
	a := trustKey(t, filepath.Join(home, "a.key"))
	b := trustKey(t, filepath.Join(home, "b.key"))
	audit := func(string, map[string]string) error { return nil }
	req := RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: a}
	if _, _, err := Recipients(req, audit); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	req.Action = "remove"
	go func() {
		_, _, err := Recipients(req, func(string, map[string]string) error {
			close(entered)
			<-release
			return nil
		})
		done <- err
	}()
	<-entered
	second := make(chan error, 1)
	go func() {
		_, _, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: b}, audit)
		second <- err
	}()
	select {
	case err := <-second:
		close(release)
		<-done
		t.Fatalf("second writer passed the first writer's audit: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	rows, _, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "list"}, nil)
	if err != nil || slices.Contains(rows, a) || !slices.Contains(rows, b) {
		t.Fatalf("lost update: %v %v", rows, err)
	}
}
