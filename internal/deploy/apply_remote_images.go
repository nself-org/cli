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
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/deploy/imagetags"
)

// imageTag is one image ref and the image id it pointed at before the pull.
type imageTag = imagetags.Tag

// imageIDRe is the only id shape a saved pair may carry.
var imageIDRe = imagetags.IDRe

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

func saveImageTags(ctx context.Context, r Remote, refs []string) ([]imageTag, error) {
	return imagetags.Save(ctx, r, refs)
}
func restoreImageTags(ctx context.Context, r Remote, saved []imageTag) error {
	return imagetags.Restore(ctx, r, saved)
}
