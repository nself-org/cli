package verify

// Purpose: prove every per-platform image digest promote will deploy is listed
//          in the signed release manifest.
// Inputs:  the verified Manifest and the comparison rows of the promote
//          engine (promote.DigestRow).
// Outputs: nil, or E495 naming the first unlisted service.
// Constraints: pure. Rows without a target digest (build-context services,
//              already refused on prod by the engine) carry no identity and
//              are skipped; the engine owns that decision. A service with an
//              image artifact of its own name must use that artifact's digests.

import (
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/errs"
)

// Listed returns every digest the manifest lists: artifact sha256 values (as
// sha256:<hex>), image index digests and per-platform image manifest digests.
func (m *Manifest) Listed() map[string]bool {
	set := map[string]bool{}
	for _, a := range m.Artifacts {
		if sha256RE.MatchString(a.SHA256) {
			set["sha256:"+a.SHA256] = true
		}
		if a.Image == nil {
			continue
		}
		if digestRE.MatchString(a.Image.IndexDigest) {
			set[a.Image.IndexDigest] = true
		}
		for _, im := range a.Image.Manifests {
			if digestRE.MatchString(im.Digest) {
				set[im.Digest] = true
			}
		}
	}
	return set
}

// serviceDigests returns the digests of the image artifacts named for service
// (the artifact is "<service>" or "image/<service>"), and whether any exists.
func (m *Manifest) serviceDigests(service string) (map[string]bool, bool) {
	set := map[string]bool{}
	found := false
	for _, a := range m.Artifacts {
		if a.Image == nil || (a.Name != service && a.Name != "image/"+service) {
			continue
		}
		found = true
		if digestRE.MatchString(a.Image.IndexDigest) {
			set[a.Image.IndexDigest] = true
		}
		for _, im := range a.Image.Manifests {
			if digestRE.MatchString(im.Digest) {
				set[im.Digest] = true
			}
		}
	}
	return set, found
}

// CheckDigests fails with E495 when a compared digest is not in the manifest,
// or, for a service the manifest has an image artifact for, not one of that
// artifact's digests.
func CheckDigests(m *Manifest, rows []promote.DigestRow) error {
	if m == nil {
		return errs.New("E499", "no verified release manifest to check digests against")
	}
	listed := m.Listed()
	var missing []string
	for _, row := range rows {
		if row.Target == "" {
			continue
		}
		// A service the manifest names must run that service's own signed
		// image; a service it does not name falls back to the flat set.
		own, named := m.serviceDigests(row.Service)
		if (named && !own[row.Target]) || !listed[row.Target] {
			missing = append(missing, row.Service+" "+row.Target)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return errs.Newf("E495", "image digests not listed in the release manifest: %s", strings.Join(missing, ", "))
}
