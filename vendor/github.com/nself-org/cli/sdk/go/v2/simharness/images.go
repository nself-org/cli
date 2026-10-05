package simharness

// Purpose: the image table (digest-pinned reference, or a Dockerfile built on a digest-pinned base) and build-on-demand.
// Inputs: an image key (openssh, debian, fedora, alpine) or a digest-pinned reference.
// Outputs: a runnable image reference; built images are tagged by content hash and reused.
// Constraints: a reference without @sha256:<64 hex> is refused; built images pin the base by digest in testdata/Dockerfile.*.

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Image keys accepted by NodeSpec.Image.
const (
	ImageOpenSSH = "openssh"
	ImageDebian  = "debian"
	ImageFedora  = "fedora"
	ImageAlpine  = "alpine"
)

// OpenSSHRef is the default image: linuxserver/openssh-server pinned to the
// index digest resolved on 2026-10-05 (the then-current tag, multi-arch).
const OpenSSHRef = "lscr.io/linuxserver/openssh-server@sha256:46f115de7c251558297e7e87566fc3fc08544b63e55502b5cf294454db5d29d1"

//go:embed testdata/Dockerfile.debian testdata/Dockerfile.fedora testdata/Dockerfile.alpine testdata/entrypoint.sh
var assets embed.FS

var pinnedRe = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)

// Pinned reports whether ref names its image by sha256 digest.
func Pinned(ref string) bool { return pinnedRe.MatchString(ref) }

// image is how simharness runs one image.
type image struct {
	ref        string // set for pulled images
	dockerfile string // set for built images (name under testdata/)
	port       int    // sshd port inside the container
	user       string // login user
	env        map[string]string
}

// table maps NodeSpec.Image keys to images.
var table = map[string]image{
	ImageOpenSSH: {ref: OpenSSHRef, port: 2222, user: "nself", env: map[string]string{
		"USER_NAME": "nself", "SUDO_ACCESS": "false", "PASSWORD_ACCESS": "false"}},
	ImageDebian: {dockerfile: "Dockerfile.debian", port: 22, user: "nself"},
	ImageFedora: {dockerfile: "Dockerfile.fedora", port: 22, user: "nself"},
	ImageAlpine: {dockerfile: "Dockerfile.alpine", port: 22, user: "nself"},
}

// lookup resolves NodeSpec.Image: a table key, or a digest-pinned reference
// that follows the openssh-server conventions (PUBLIC_KEY, USER_NAME).
func lookup(s NodeSpec) (image, error) {
	key := s.Image
	if key == "" {
		key = ImageOpenSSH
	}
	img, ok := table[key]
	if !ok {
		if !Pinned(key) {
			return image{}, fmt.Errorf("image %q is not in the table and has no @sha256 digest", key)
		}
		img = image{ref: key, port: 22, user: "nself", env: map[string]string{"USER_NAME": "nself"}}
	}
	if s.Port != 0 {
		img.port = s.Port
	}
	if s.User != "" {
		img.user = s.User
	}
	return img, nil
}

var buildMu sync.Mutex

// reference returns the image to run, building it first when needed.
func (img image) reference(ctx context.Context, platform string) (string, error) {
	if img.ref != "" {
		return img.ref, nil
	}
	return buildImage(ctx, img.dockerfile, platform)
}

// buildImage builds testdata/<dockerfile> and tags it by content hash, so an
// unchanged Dockerfile is built once per Docker host.
func buildImage(ctx context.Context, dockerfile, platform string) (string, error) {
	df, err := assets.ReadFile("testdata/" + dockerfile)
	if err != nil {
		return "", err
	}
	ep, err := assets.ReadFile("testdata/entrypoint.sh")
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append(append([]byte(platform+"\x00"), df...), ep...))
	tag := "nself-simharness-" + strings.TrimPrefix(dockerfile, "Dockerfile.") + ":" + hex.EncodeToString(sum[:6])
	if platform != "" {
		tag += "-" + strings.NewReplacer("/", "-").Replace(platform)
	}

	buildMu.Lock()
	defer buildMu.Unlock()
	if _, err := docker(ctx, "image", "inspect", "--format", "{{.Id}}", tag); err == nil {
		return tag, nil
	}
	dir, err := os.MkdirTemp("", "simharness-build-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), df, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "entrypoint.sh"), ep, 0o700); err != nil {
		return "", err
	}
	args := []string{"build", "--quiet", "--tag", tag, "--label", LabelKey + "=1"}
	if platform != "" {
		args = append(args, "--platform", platform)
	}
	bctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if _, err := run(bctx, "", append(args, dir)); err != nil {
		return "", err
	}
	return tag, nil
}
