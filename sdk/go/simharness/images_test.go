package simharness

import (
	"regexp"
	"strings"
	"testing"
)

var fromRe = regexp.MustCompile(`(?m)^FROM\s+(\S+)`)

// Every image the table can run is pinned by digest, directly or through the
// FROM line of its embedded Dockerfile.
func TestTableImagesArePinned(t *testing.T) {
	for key, img := range table {
		switch {
		case img.ref != "":
			if !Pinned(img.ref) {
				t.Errorf("%s: ref %q has no sha256 digest", key, img.ref)
			}
		case img.dockerfile != "":
			df, err := assets.ReadFile("testdata/" + img.dockerfile)
			if err != nil {
				t.Fatal(err)
			}
			m := fromRe.FindAllStringSubmatch(string(df), -1)
			if len(m) == 0 {
				t.Fatalf("%s: no FROM line", key)
			}
			for _, from := range m {
				if !Pinned(from[1]) {
					t.Errorf("%s: FROM %s is not pinned by digest", key, from[1])
				}
			}
		default:
			t.Errorf("%s: neither ref nor dockerfile", key)
		}
	}
	for _, k := range []string{ImageOpenSSH, ImageDebian, ImageFedora, ImageAlpine} {
		if _, ok := table[k]; !ok {
			t.Errorf("table lacks %s", k)
		}
	}
}

// The embedded sshd setup is key auth only, no root login, non-root user.
func TestDockerfilesKeyAuthOnlyNonRoot(t *testing.T) {
	ep, err := assets.ReadFile("testdata/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PasswordAuthentication=no", "PermitRootLogin=no", "KbdInteractiveAuthentication=no", "sshd -D"} {
		if !strings.Contains(string(ep), want) {
			t.Errorf("entrypoint.sh lacks %q", want)
		}
	}
	for _, name := range []string{"debian", "fedora", "alpine"} {
		df, err := assets.ReadFile("testdata/Dockerfile." + name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(df), "useradd") && !strings.Contains(string(df), "adduser") {
			t.Errorf("%s: no non-root user", name)
		}
		for _, want := range []string{"openssh-server", "entrypoint.sh"} {
			if !strings.Contains(string(df), want) {
				t.Errorf("%s: Dockerfile lacks %q", name, want)
			}
		}
	}
}

func TestLookup(t *testing.T) {
	if img, err := lookup(NodeSpec{}); err != nil || img.ref != OpenSSHRef || img.port != 2222 {
		t.Fatalf("default = %+v, %v", img, err)
	}
	pinned := "example.com/x/y@sha256:" + strings.Repeat("a", 64)
	if img, err := lookup(NodeSpec{Image: pinned, Port: 2022, User: "bob"}); err != nil || img.ref != pinned || img.port != 2022 || img.user != "bob" {
		t.Fatalf("pinned = %+v, %v", img, err)
	}
	for _, bad := range []string{"alpine:3.20", "x/y:1.2.3", "x@sha256:abc", "x@sha256:" + strings.Repeat("A", 64), "debian "} {
		if _, err := lookup(NodeSpec{Image: bad}); err == nil {
			t.Errorf("lookup accepted %q", bad)
		}
	}
}

func TestBuildImageTagsByContentAndReuses(t *testing.T) {
	d := installFakeDocker(t)
	tag1, err := buildImage(t.Context(), "Dockerfile.alpine", "")
	if err != nil {
		t.Fatal(err)
	}
	tag2, _ := buildImage(t.Context(), "Dockerfile.alpine", "linux/arm64")
	tagD, _ := buildImage(t.Context(), "Dockerfile.debian", "")
	if !regexp.MustCompile(`^nself-simharness-alpine:[0-9a-f]{12}$`).MatchString(tag1) {
		t.Fatalf("tag = %q", tag1)
	}
	if tag1 == tag2 || tag1 == tagD || !strings.HasSuffix(tag2, "-linux-arm64") {
		t.Fatalf("tags not distinct: %q %q %q", tag1, tag2, tagD)
	}
	// image inspect succeeded in the fake, so nothing was built.
	if n := len(d.find("build")); n != 0 {
		t.Fatalf("%d builds although the image exists", n)
	}
	d.missing[tag1] = true
	if _, err := buildImage(t.Context(), "Dockerfile.alpine", ""); err != nil {
		t.Fatal(err)
	}
	b := d.find("build")
	if len(b) != 1 || flagValues(b[0], "--tag")[0] != tag1 {
		t.Fatalf("build calls = %v", b)
	}
	d.missing[tag2] = true
	_, _ = buildImage(t.Context(), "Dockerfile.alpine", "linux/arm64")
	b = d.find("build")
	if len(b) != 2 || flagValues(b[1], "--platform")[0] != "linux/arm64" {
		t.Fatalf("platform build = %v", b)
	}
}

func TestBuildImageUnknownDockerfile(t *testing.T) {
	installFakeDocker(t)
	if _, err := buildImage(t.Context(), "Dockerfile.nope", ""); err == nil {
		t.Fatal("expected an error")
	}
}
