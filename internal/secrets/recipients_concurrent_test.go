package secrets

// Purpose: prevent a concurrent grant from restoring a revoked recipient.
// Inputs: an encrypted store and recipient changes interleaved at audit.
// Outputs: the final store keeps the grant and the revocation.
// Constraints: real age encryption, no private identities in fixtures.
// SPORT: P7-TRUST-03, D-0579.

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestRecipientsRefusesUnsafeLockPath(t *testing.T) {
	root, home, _ := trustFixture(t)
	trustStore(t, root, "ci", "FOO")
	node := trustKey(t, filepath.Join(home, "node.key"))
	path := filepath.Join(root, ".secrets", "ci.age.lock")
	if err := os.Symlink(filepath.Join(root, ".secrets", "ci.age"), path); err != nil {
		t.Skip("symlink unavailable:", err)
	}
	_, _, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: node}, nil)
	trustCode(t, err, "E543")
}

func TestRecipientsConcurrentGrantPreservesRevocation(t *testing.T) {
	root, home, _ := trustFixture(t)
	trustStore(t, root, "ci", "FOO")
	revoked := trustKey(t, filepath.Join(home, "revoked.key"))
	granted := trustKey(t, filepath.Join(home, "granted.key"))
	sink := func(string, map[string]string) error { return nil }
	req := RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: revoked}
	if _, _, err := Recipients(req, sink); err != nil {
		t.Fatal(err)
	}
	paused, release := make(chan struct{}), make(chan struct{})
	addDone, removeDone := make(chan error, 1), make(chan error, 1)
	go func() {
		req := RecipientsRequest{ProjectRoot: root, File: "ci", Action: "add", Recipient: granted}
		_, _, err := Recipients(req, func(string, map[string]string) error {
			close(paused)
			<-release
			return nil
		})
		addDone <- err
	}()
	select {
	case <-paused:
	case err := <-addDone:
		t.Fatalf("grant did not reach audit: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("grant did not reach audit")
	}
	go func() {
		req := RecipientsRequest{ProjectRoot: root, File: "ci", Action: "remove", Recipient: revoked}
		_, _, err := Recipients(req, sink)
		removeDone <- err
	}()
	// Without the lock, removal finishes while the grant holds its stale copy.
	// With the lock, it must wait until the grant's audit and save finish.
	var removeErr error
	removed := false
	select {
	case removeErr = <-removeDone:
		removed = true
	case <-time.After(250 * time.Millisecond):
	}
	close(release)
	if err := <-addDone; err != nil {
		t.Fatal(err)
	}
	if !removed {
		removeErr = <-removeDone
	}
	if removeErr != nil {
		t.Fatal(removeErr)
	}
	rows, _, err := Recipients(RecipientsRequest{ProjectRoot: root, File: "ci", Action: "list"}, nil)
	if err != nil || slices.Contains(rows, revoked) || !slices.Contains(rows, granted) {
		t.Fatalf("concurrent changes lost: revoked=%v granted=%v err=%v", slices.Contains(rows, revoked), slices.Contains(rows, granted), err)
	}
}
