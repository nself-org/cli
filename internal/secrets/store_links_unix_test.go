//go:build unix

package secrets

// Purpose: prevent hardlinked release ciphertext from being treated as ci.
// Inputs: a release store linked as ci and ephemeral identities.
// Outputs: E543 from path checks, resolution and recipient operations.
// Constraints: Unix link counts; path check needs no age. SPORT: D-0580.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHardlinkedStoreRejectedBeforeIdentity(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".secrets")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(dir, "release.age")
	if err := os.WriteFile(release, []byte("release ciphertext"), 0600); err != nil {
		t.Fatal(err)
	}
	ci := filepath.Join(dir, "ci.age")
	if err := os.Link(release, ci); err != nil {
		t.Fatal(err)
	}
	trustCode(t, checkStorePath(dir, ci, nil), "E543")
}

func TestHardlinkedStoreOperationsRejected(t *testing.T) {
	root, home, _ := trustFixture(t)
	trustStore(t, root, "release", "X")
	dir := filepath.Join(root, ".secrets")
	if err := os.Link(filepath.Join(dir, "release.age"), filepath.Join(dir, "ci.age")); err != nil {
		t.Fatal(err)
	}
	node := trustKey(t, filepath.Join(home, "node.key"))
	for _, action := range []string{"add", "remove", "list"} {
		t.Run(action, func(t *testing.T) {
			rows, changed, err := Recipients(RecipientsRequest{
				ProjectRoot: root, File: "ci", Action: action,
				Recipient: node,
			}, func(string, map[string]string) error {
				t.Fatal("refused recipient operation was audited")
				return nil
			})
			trustCode(t, err, "E543")
			if rows != nil || changed {
				t.Fatal("refused store returned recipients or changed")
			}
		})
	}
	t.Run("resolve", func(t *testing.T) {
		values, err := Resolve(context.Background(), ResolveRequest{
			ProjectRoot: root, Refs: []string{"project/X"},
		}, func(string, map[string]string) error { return nil })
		trustCode(t, err, "E543")
		if values != nil {
			t.Fatal("refused store returned values")
		}
	})
}
