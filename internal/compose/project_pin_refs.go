package compose

// Reference comparison and notices for project image pins.

import (
	"fmt"
	"strings"
)

// repoOf is CanonicalRef without the tag.
func repoOf(ref string) string {
	ref = CanonicalRef(ref)
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	return ref
}

// noticePinDropped prints the drop notice once per service, variable and pin.
func noticePinDropped(service, envName, pin string) {
	key := service + "/" + envName + "/" + pin
	pinNoticedMu.Lock()
	seen := pinNoticed[key]
	pinNoticed[key] = true
	w := pinNoticeW
	pinNoticedMu.Unlock()
	if seen || w == nil {
		return
	}
	if envName == "the rendered image" {
		_, _ = fmt.Fprintf(w, "notice: the %s image is now a different repository than its pin, so the pin in %s is not used\n", service, ProjectLockFile)
		return
	}
	_, _ = fmt.Fprintf(w, "notice: %s is set, so the %s pin in %s is not used\n", envName, service, ProjectLockFile)
}

func imageBase(ref string) string {
	base, _, _ := strings.Cut(ref, "@")
	return base
}

// CanonicalRef reduces an image reference to one spelling for comparison: no
// digest, no docker.io/, index.docker.io/ or library/ prefix, host and path in
// lower case, the tag as written (tags are case-sensitive). The bootstrap pins
// a reference as docker.io/<repo>:<tag>@sha256 while the legacy image strings
// omit the host, so every comparison of two spellings goes through here
// (internal/docker cannot be imported: its tests import this package).
func CanonicalRef(ref string) string {
	ref = strings.TrimSpace(imageBase(ref))
	tag := ""
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref, tag = ref[:i], ref[i:]
	}
	ref = strings.ToLower(ref)
	for _, host := range []string{"registry-1.docker.io/", "index.docker.io/", "docker.io/"} {
		ref = strings.TrimPrefix(ref, host)
	}
	return strings.TrimPrefix(ref, "library/") + tag
}
