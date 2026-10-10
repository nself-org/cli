package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/docker"
)

// writeFragment plants a docker-compose.plugin.yml in a fresh plugin dir.
func writeFragment(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.plugin.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// stubPull records the references pulled and fails the ones in failOn.
func stubPull(t *testing.T, failOn ...string) *[]string {
	t.Helper()
	var got []string
	orig := pullImage
	pullImage = func(_ context.Context, ref string) error {
		got = append(got, ref)
		for _, f := range failOn {
			if f == ref {
				return errors.New("pull denied")
			}
		}
		return nil
	}
	t.Cleanup(func() { pullImage = orig })
	return &got
}

// TestUpdateImagesDiffPulls: only references the new fragment changed or
// added are pulled; unchanged, locally built and ${VAR} references are not.
func TestUpdateImagesDiffPulls(t *testing.T) {
	oldDir := writeFragment(t, `services:
  web:
    image: nself/web:1.0.0
  worker:
    image: nself/worker:2.0.0
  built:
    build: .
    image: nself/built:1.0.0
`)
	newDir := writeFragment(t, `services:
  web:
    image: nself/web:1.1.0
  worker:
    image: nself/worker:2.0.0
  sidecar:
    image: busybox:1.36
  built:
    build: .
    image: nself/built:2.0.0
  tagged:
    image: nself/tagged:${TAG}
`)
	prev, err := fragmentImages(oldDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := prev["built"]; ok {
		t.Fatal("a service that builds locally must not count as an image reference")
	}
	pulls := stubPull(t)
	pulled, err := pullChangedImages(context.Background(), prev, newDir)
	if err != nil {
		t.Fatalf("pullChangedImages: %v", err)
	}
	want := []string{"busybox:1.36", "nself/web:1.1.0"}
	if !reflect.DeepEqual(pulled, want) || !reflect.DeepEqual(*pulls, want) {
		t.Fatalf("pulled %v (calls %v), want %v", pulled, *pulls, want)
	}

	// Same fragment on both sides: nothing to pull.
	*pulls = nil
	if pulled, err = pullChangedImages(context.Background(), prev, oldDir); err != nil || len(pulled) != 0 || len(*pulls) != 0 {
		t.Fatalf("unchanged fragment pulled %v / %v (err %v)", pulled, *pulls, err)
	}

	// A fragment-less or missing plugin dir is a no-op, not an error.
	if pulled, err = pullChangedImages(context.Background(), prev, t.TempDir()); err != nil || len(pulled) != 0 {
		t.Fatalf("missing fragment: pulled %v, err %v", pulled, err)
	}
}

// TestUpdateImagesDiffPulls_FailureNamesImage: a failed pull stops with the
// reference named, and what was already pulled is still reported.
func TestUpdateImagesDiffPulls_FailureNamesImage(t *testing.T) {
	newDir := writeFragment(t, "services:\n  a:\n    image: x/a:2\n  b:\n    image: x/b:2\n")
	stubPull(t, "x/b:2")
	pulled, err := pullChangedImages(context.Background(), nil, newDir)
	if err == nil || !reflect.DeepEqual(pulled, []string{"x/a:2"}) {
		t.Fatalf("pulled %v, err %v; want x/a:2 pulled and an error for x/b:2", pulled, err)
	}
	if got := err.Error(); !strings.Contains(got, "x/b:2") {
		t.Fatalf("error should name the image: %v", err)
	}
}

// hasIntegrationTag reports whether this test binary was built with
// -tags integration (the only file this Ticket may add tests to is this one,
// so the integration test selects itself at run time instead of by build tag).
func hasIntegrationTag() bool {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, s := range bi.Settings {
		if s.Key == "-tags" {
			for _, tag := range strings.Split(s.Value, ",") {
				if tag == "integration" {
					return true
				}
			}
		}
	}
	return false
}

// TestUpdateImagesPullsNewTag (integration, Linux): with the real docker
// funnel, an update whose fragment moves an image tag pulls the new tag.
func TestUpdateImagesPullsNewTag(t *testing.T) {
	if !hasIntegrationTag() {
		t.Skip("integration test: run with -tags integration against a Docker daemon")
	}
	ctx := context.Background()
	const oldRef, newRef = "busybox:1.36.0", "busybox:1.36.1"
	dc := &docker.Compose{}
	_ = dc.Run(ctx, "", "image", "rm", "-f", newRef) // start without the new tag
	t.Cleanup(func() { _ = dc.Run(ctx, "", "image", "rm", "-f", newRef) })

	oldDir := writeFragment(t, "services:\n  s:\n    image: "+oldRef+"\n")
	newDir := writeFragment(t, "services:\n  s:\n    image: "+newRef+"\n")
	prev, err := fragmentImages(oldDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := dc.Run(ctx, "", "image", "inspect", newRef); err == nil {
		t.Fatalf("precondition: %s must be absent before the update", newRef)
	}
	pulled, err := pullChangedImages(ctx, prev, newDir)
	if err != nil {
		t.Fatalf("pullChangedImages: %v", err)
	}
	if !reflect.DeepEqual(pulled, []string{newRef}) {
		t.Fatalf("pulled %v, want [%s]", pulled, newRef)
	}
	if err := dc.Run(ctx, "", "image", "inspect", newRef); err != nil {
		t.Fatalf("%s should be present after the update: %v", newRef, err)
	}
}
