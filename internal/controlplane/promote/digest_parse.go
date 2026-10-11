package promote

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

var (
	manifestDigestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	linuxPlatformRE  = regexp.MustCompile(`^linux/(amd64|arm64|arm)(/v[0-9]+)?$`)
)

func parseComposeImages(raw []byte) ([]composeImage, error) {
	var images []composeImage
	if err := json.Unmarshal(raw, &images); err == nil {
		return images, nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var image composeImage
		if err := json.Unmarshal(scanner.Bytes(), &image); err != nil {
			return nil, fmt.Errorf("parse docker compose images: %w", err)
		}
		images = append(images, image)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse docker compose images: %w", err)
	}
	return images, nil
}

func parseInspect(raw []byte, ref string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(string(raw)), "|")
	if len(parts) != 2 || parts[1] == "" {
		return "", "", fmt.Errorf("unexpected image inspect output")
	}
	var repoDigests []string
	if err := json.Unmarshal([]byte(parts[0]), &repoDigests); err != nil {
		return "", "", fmt.Errorf("repo digests: %w", err)
	}
	if len(repoDigests) == 0 {
		return "", "", fmt.Errorf("image %s has no repo digest", ref)
	}
	platform := normalizePlatform(parts[1])
	if !linuxPlatformRE.MatchString(platform) {
		return "", "", fmt.Errorf("unsupported image platform %q", parts[1])
	}
	repository := imageRepository(ref)
	for _, candidate := range repoDigests {
		repo, digest, ok := strings.Cut(candidate, "@")
		if ok && sameRepository(repo, repository) && manifestDigestRE.MatchString(digest) {
			return digest, platform, nil
		}
	}
	return "", "", fmt.Errorf("image %s has no matching repo digest", ref)
}

// Purpose: equate arm64's default variant while preserving other platform identities.
// Inputs: a Docker or Compose platform selector.
// Outputs: the canonical selector used by capture, comparison and manifest lookup.
// Constraints: only linux/arm64/v8 aliases linux/arm64; other variants remain distinct.
func normalizePlatform(platform string) string {
	if platform == "linux/arm64/v8" {
		return "linux/arm64"
	}
	return platform
}

// Purpose: select the same platform identity from registry and image-lock maps.
// Inputs: manifest digests keyed by platform and the requested selector.
// Outputs: the matching manifest digest, or an empty string when absent.
// Constraints: canonical keys take precedence over equivalent default variants.
func platformDigest(platforms map[string]string, platform string) string {
	platform = normalizePlatform(platform)
	if digest := platforms[platform]; digest != "" {
		return digest
	}
	for candidate, digest := range platforms {
		if digest != "" && normalizePlatform(candidate) == platform {
			return digest
		}
	}
	return ""
}

func sameRepository(left, right string) bool {
	normalize := func(value string) string {
		value = strings.TrimPrefix(value, "docker.io/")
		if !strings.Contains(value, "/") {
			value = "library/" + value
		}
		return value
	}
	return normalize(left) == normalize(right)
}

func imageRepository(ref string) string {
	if at := strings.IndexByte(ref, '@'); at >= 0 {
		ref = ref[:at]
	}
	if colon := strings.LastIndexByte(ref, ':'); colon > strings.LastIndexByte(ref, '/') {
		ref = ref[:colon]
	}
	return ref
}

func releaseDriftError(drift, unverifiable []string) error {
	var details []string
	if len(drift) > 0 {
		details = append(details, "different services: "+strings.Join(drift, ", "))
	}
	if len(unverifiable) > 0 {
		details = append(details, "unverifiable services: "+strings.Join(unverifiable, ", "))
	}
	return errs.New("E490", "release drift: "+strings.Join(details, "; "))
}

func sortRows(rows []DigestRow) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].Service < rows[j].Service })
}
