package deploy

// Purpose: put back the image tags the pull of a v1.5 remote apply moved when
//          the apply stops before any container is touched (D-0426): an E403
//          refusal, a failed preview, a failed pull. Without it the next manual
//          `docker compose up` on the host would run the new images and skip
//          the stateful-recreate confirmation.
// Inputs:  the image refs of the resolved ComposeModel.
// Outputs: the ref -> image id pairs saved before the pull; an error when a
//          ref could not be pointed back.
// Constraints: a ref pinned by digest cannot move and is skipped. A ref the
//              host did not have before the pull has no old image and is left
//              alone. A pull leaves the old image on the host (untagged), so a
//              tag by id restores the ref exactly.

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// imageTag is one image ref and the image id it pointed at before the pull.
type imageTag struct{ ID, Ref string }

// imageIDRe is the only id shape a saved pair may carry.
var imageIDRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// imageRefs are the model's distinct image refs, sorted, without digest pins.
func imageRefs(model ComposeModel) []string {
	seen := map[string]bool{}
	var refs []string
	for _, s := range model.Services {
		if s.Image == "" || strings.Contains(s.Image, "@") || seen[s.Image] {
			continue
		}
		seen[s.Image] = true
		refs = append(refs, s.Image)
	}
	sort.Strings(refs)
	return refs
}

// saveImageTags records the image id each ref points at on the host now. A ref
// the host does not have is absent from the result.
func saveImageTags(ctx context.Context, r Remote, refs []string) ([]imageTag, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	script := fmt.Sprintf(`for img in %s; do
if id=$(LC_ALL=C docker image inspect --format '{{.Id}}' "$img" 2>&1); then
printf '%%s %%s\n' "$id" "$img"
else
case "$id" in
"Error response from daemon: No such image: $img"|"Error: No such image: $img") ;;
*) printf '%%s\n' "$id" >&2; exit 1 ;;
esac
fi
done`, strings.Join(quoteAll(refs), " "))
	out, err := r.Run(ctx, shScript(script))
	if err != nil {
		return nil, fmt.Errorf("reading the image ids on %s before the pull: %w", r.Label(), err)
	}
	want := map[string]bool{}
	for _, ref := range refs {
		want[ref] = true
	}
	var saved []imageTag
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 || !imageIDRe.MatchString(f[0]) || !want[f[1]] {
			return nil, fmt.Errorf("invalid image snapshot record from %s: %q", r.Label(), line)
		}
		saved = append(saved, imageTag{ID: f[0], Ref: f[1]})
	}
	return saved, nil
}

// restoreImageTags points every saved ref back at its old image id. It runs on
// a context that survives the caller's cancellation, bounded by a minute.
func restoreImageTags(ctx context.Context, r Remote, saved []imageTag) error {
	if len(saved) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancel()
	parts := []string{"rc=0"}
	for _, t := range saved {
		parts = append(parts, fmt.Sprintf("docker tag %s %s || rc=1", q(t.ID), q(t.Ref)))
	}
	if _, err := r.Run(ctx, shScript(strings.Join(append(parts, "exit $rc"), "; "))); err != nil {
		return fmt.Errorf("putting the image tags back on %s: %w", r.Label(), err)
	}
	return nil
}
