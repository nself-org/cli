//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package secrets

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestRecipientsConcurrentChanges(t *testing.T) {
	for _, file := range []string{"ci", "release"} {
		t.Run(file, func(t *testing.T) {
			root, home, _ := trustFixture(t)
			trustStore(t, root, file, "FOO")
			a := trustKey(t, filepath.Join(home, "a.key"))
			b := trustKey(t, filepath.Join(home, "b.key"))
			entered, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			first := make(chan error, 1)
			req := RecipientsRequest{ProjectRoot: root, File: file, Action: "add", Recipient: a, AllowReleaseClass: true}
			go func() {
				_, _, err := Recipients(req, func(string, map[string]string) error { close(entered); <-release; return nil })
				first <- err
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("first mutation did not reach audit")
			}
			second := make(chan error, 1)
			req2 := req
			req2.Recipient = b
			go func() {
				_, _, err := Recipients(req2, func(string, map[string]string) error { return nil })
				second <- err
			}()
			select {
			case err := <-second:
				close(release)
				<-first
				t.Fatalf("second mutation passed blocked audit: %v", err)
			case <-time.After(150 * time.Millisecond):
			}
			close(release)
			for _, done := range []chan error{first, second} {
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("mutation remained blocked")
				}
			}
			rows, _, err := Recipients(RecipientsRequest{ProjectRoot: root, File: file, Action: "list"}, nil)
			if err != nil || !slices.Contains(rows, a) || !slices.Contains(rows, b) {
				t.Fatal("lost recipient update", err)
			}
		})
	}
}
