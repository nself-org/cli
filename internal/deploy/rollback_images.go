package deploy

// Purpose: make `deploy rollback` bring back the image the previous deploy ran
//          (D-0700). The pull of an apply moves a mutable tag (app:latest); the
//          restored compose names the same tag, so a pull in the rollback would
//          fetch the NEW image again and the rollback would report success on
//          the promoted code. The apply therefore records, next to its
//          pre-deploy tar, the image id each ref pointed at before the pull;
//          the rollback points those refs back at those ids and pulls only the
//          services it has no record for.
// Inputs:  the pre-deploy tar path (the record sits beside it as
//          pre-deploy-<id>.images, one "<image id> <ref>" per line) and the
//          resolved model of the restored compose.
// Outputs: the ref -> id pairs to restore; E489 when a recorded image is gone.
// Constraints: only refs the restored model names are used, so a tampered record
//              cannot retag an unrelated image; an id must have the sha256 shape
//              of saveImageTags. A deploy without a record (made before this
//              change, or by another tool) rolls back as it did: full pull.

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// imageRecordPath is the record that belongs to a pre-deploy tar.
func imageRecordPath(tar string) string {
	return strings.TrimSuffix(tar, ".tar.gz") + ".images"
}

// writeImageRecord stores saved beside tar (mode 0600). An empty list still
// writes the file: it says "this deploy had nothing to protect", which differs
// from "no record".
func writeImageRecord(ctx context.Context, r Remote, tar string, saved []imageTag) error {
	var lines strings.Builder
	for _, t := range saved {
		fmt.Fprintf(&lines, `printf '%%s %%s\n' %s %s >> "$t"; `, q(t.ID), q(t.Ref))
	}
	script := fmt.Sprintf(`set -e; umask 077; f=%s; t="$f.tmp"; : > "$t"; %smv -f "$t" "$f"`, q(imageRecordPath(tar)), lines.String())
	if _, err := r.Run(ctx, shScript(script)); err != nil {
		return fmt.Errorf("recording the running image ids on %s: %w", r.Label(), err)
	}
	return nil
}

// readImageRecord returns the record beside tar; found is false when there is none.
func readImageRecord(ctx context.Context, r Remote, tar string) (saved []imageTag, found bool, err error) {
	f := imageRecordPath(tar)
	out, err := r.Run(ctx, shScript(fmt.Sprintf(`if [ -f %s ] && [ ! -L %s ]; then echo FOUND; cat %s; fi`, q(f), q(f), q(f))))
	if err != nil {
		return nil, false, fmt.Errorf("reading %s on %s: %w", path.Base(f), r.Label(), err)
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "FOUND" {
		return nil, false, nil
	}
	for _, line := range lines[1:] {
		if f := strings.Fields(line); len(f) == 2 && imageIDRe.MatchString(f[0]) {
			saved = append(saved, imageTag{ID: f[0], Ref: f[1]})
		}
	}
	return saved, true, nil
}

// imagesToRestore picks the recorded pairs whose ref the restored model names
// and checks that each recorded image is still on the host. A missing image
// refuses the rollback (E489): pulling the tag would return the promoted image.
func imagesToRestore(ctx context.Context, r Remote, tar string, model ComposeModel) ([]imageTag, error) {
	saved, found, err := readImageRecord(ctx, r, tar)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	named := map[string]bool{}
	for _, ref := range imageRefs(model) {
		named[ref] = true
	}
	var keep []imageTag
	for _, t := range saved {
		if named[t.Ref] {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		return nil, nil
	}
	ids := make([]string, len(keep))
	for i, t := range keep {
		ids[i] = t.ID
	}
	script := fmt.Sprintf(`for id in %s; do if docker image inspect --format '{{.Id}}' "$id" >/dev/null 2>&1; then printf 'OK %%s\n' "$id"; fi; done; exit 0`, strings.Join(quoteAll(ids), " "))
	out, err := r.Run(ctx, shScript(script))
	if err != nil {
		return nil, fmt.Errorf("checking the recorded images on %s: %w", r.Label(), err)
	}
	present := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if id, ok := strings.CutPrefix(strings.TrimSpace(line), "OK "); ok {
			present[id] = true
		}
	}
	var gone []string
	for _, t := range keep {
		if !present[t.ID] {
			gone = append(gone, t.Ref)
		}
	}
	if len(gone) > 0 {
		return nil, errs.Newf("E489", "rollback refused on %s: the image(s) the previous deploy ran are gone from the host (%s)", r.Label(), strings.Join(gone, ", ")).
			WithWhy("A pull would fetch the promoted image again under the same tag, so the rollback would keep running the new code.").
			WithFix("Load or pull the previous image on the host (for example by digest), or restore the previous release by hand; the files are restored, the database and containers were not touched.")
	}
	return keep, nil
}

// pullTargets are the services a rollback still has to pull: those with an
// image and no restored tag. Empty means no pull at all.
func pullTargets(model ComposeModel, keep []imageTag) []string {
	kept := map[string]bool{}
	for _, t := range keep {
		kept[t.Ref] = true
	}
	var svcs []string
	for name := range model.Services {
		if img := model.Services[name].Image; img != "" && !kept[img] {
			svcs = append(svcs, name)
		}
	}
	sort.Strings(svcs)
	return svcs
}
