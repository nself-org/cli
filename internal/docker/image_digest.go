package docker

// image_digest.go — the repo digests of a local image.
//
// Purpose: the project lock (nself.lock.yaml, P7-LIVE-09) pins a first lock to
// the digest of the image a project already runs, so it needs the RepoDigests
// of a local image without pulling anything. All docker calls stay in this
// package (the funnel); this one goes through runCapture.
// Inputs: an image reference (a tag such as "redis:7-alpine").
// Outputs: ImageRepoDigests returns the raw "repo@sha256:<hex>" entries;
// LocalRepoDigest returns the single "sha256:<hex>" for the reference's own
// repository, or "" when there is not exactly one.
// Constraints: read only (`docker image inspect`); an image that is not
// present locally is ErrImageNotFound, any other docker failure is returned
// as-is so a caller can tell "absent" from "cannot tell".

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrImageNotFound means the image is not present locally. Match with errors.Is.
var ErrImageNotFound = errors.New("image not found locally")

var repoDigestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ImageRepoDigests returns the RepoDigests ("repo@sha256:<hex>") of the local
// image ref. An image without any (built locally, never pushed) returns an
// empty slice and no error.
func ImageRepoDigests(ctx context.Context, ref string) ([]string, error) {
	if strings.TrimSpace(ref) == "" {
		return nil, errors.New("image repo digests: empty reference")
	}
	out, err := runCapture(ctx, "image", "inspect", "--format", "{{json .RepoDigests}}", ref)
	if err != nil {
		if strings.Contains(err.Error(), "No such image") {
			return nil, fmt.Errorf("%s: %w", ref, ErrImageNotFound)
		}
		return nil, err
	}
	return parseRepoDigests(out)
}

// parseRepoDigests reads the JSON list `{{json .RepoDigests}}` prints. The
// template prints null for an image with none.
func parseRepoDigests(raw []byte) ([]string, error) {
	var list []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &list); err != nil {
		return nil, fmt.Errorf("image repo digests: unreadable inspect output: %w", err)
	}
	return list, nil
}

// LocalRepoDigest returns "sha256:<hex>" when the local image for ref has
// exactly one distinct digest recorded for ref's own repository, else "". A
// digest recorded for another repository (a retagged image) never counts.
func LocalRepoDigest(ctx context.Context, ref string) string {
	digests, err := ImageRepoDigests(ctx, ref)
	if err != nil {
		return ""
	}
	return pickRepoDigest(ref, digests)
}

// pickRepoDigest selects the single digest of ref's repository from entries.
func pickRepoDigest(ref string, entries []string) string {
	repo := repoOf(ref)
	found := ""
	for _, e := range entries {
		name, digest, ok := strings.Cut(e, "@")
		if !ok || !repoDigestRE.MatchString(digest) || repoOf(name) != repo {
			continue
		}
		if found != "" && found != digest {
			return ""
		}
		found = digest
	}
	return found
}

// repoOf reduces a reference to its normalised repository name: no tag, no
// digest, no docker.io/ or library/ prefix.
func repoOf(ref string) string {
	ref, _, _ = strings.Cut(ref, "@")
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	ref = strings.TrimPrefix(ref, "docker.io/")
	ref = strings.TrimPrefix(ref, "index.docker.io/")
	return strings.TrimPrefix(ref, "library/")
}
