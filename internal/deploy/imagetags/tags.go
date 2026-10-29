// Package imagetags snapshots and restores mutable Docker image references.
package imagetags

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// Runner executes a shell command on one named host.
type Runner interface {
	Run(context.Context, string) ([]byte, error)
	Label() string
}

// Tag records the image ID a mutable reference pointed to.
type Tag struct{ ID, Ref string }

// IDRe accepts only full Docker content IDs.
var IDRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func quoteAll(refs []string) []string {
	out := make([]string, len(refs))
	for i, ref := range refs {
		out[i] = remote.ShellQuote(ref)
	}
	return out
}

// Save records the image id each ref points at on the host now. A ref
// the host does not have is absent from the result.
func Save(ctx context.Context, r Runner, refs []string) ([]Tag, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	// Docker normalizes missing references (for example web -> web:latest).
	// Accept only its single-line missing-image diagnostic, with a nonempty name.
	script := fmt.Sprintf(`for img in %s; do
if id=$(docker image inspect --format '{{.Id}}' "$img" 2>&1); then
  printf '%%s %%s\n' "$id" "$img"
else
  case "$id" in
    *'
'*) exit 1 ;;
    "Error response from daemon: No such image: "?*|"Error: No such image: "?*) ;;
    *) printf 'cannot inspect image %%s\n' "$img" >&2; exit 1 ;;
  esac
fi
done`, strings.Join(quoteAll(refs), " "))
	out, err := r.Run(ctx, "sh -c "+remote.ShellQuote(script))
	if err != nil {
		return nil, fmt.Errorf("reading the image ids on %s before the pull: %w", r.Label(), err)
	}
	want := map[string]bool{}
	for _, ref := range refs {
		want[ref] = true
	}
	var saved []Tag
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 || !IDRe.MatchString(f[0]) || !want[f[1]] {
			return nil, fmt.Errorf("reading the image ids on %s before the pull: invalid inspect response", r.Label())
		}
		saved = append(saved, Tag{ID: f[0], Ref: f[1]})
	}
	return saved, nil
}

// Restore points every saved ref back at its old image id. It runs on
// a context that survives the caller's cancellation, bounded by a minute.
func Restore(ctx context.Context, r Runner, saved []Tag) error {
	if len(saved) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	parts := []string{"rc=0"}
	for _, t := range saved {
		parts = append(parts, fmt.Sprintf("docker tag %s %s || rc=1", remote.ShellQuote(t.ID), remote.ShellQuote(t.Ref)))
	}
	if _, err := r.Run(ctx, "sh -c "+remote.ShellQuote(strings.Join(append(parts, "exit $rc"), "; "))); err != nil {
		return fmt.Errorf("putting the image tags back on %s: %w", r.Label(), err)
	}
	return nil
}
