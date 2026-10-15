package main

import (
	"io"
	"os"
	"testing"
)

// TestRepositoryPolicyCoverage exercises the scheduled planner against the
// checked-in inputs. Only registry tags are stubbed, so this needs no network.
func TestRepositoryPolicyCoverage(t *testing.T) {
	raw, err := os.ReadFile("../../internal/compose/images.yaml")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := parseLockFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("../../.github/image-bump-policy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := loadPolicy(raw)
	if err != nil {
		t.Fatal(err)
	}
	tags := stubLister{}
	for _, image := range lock.Images {
		tags[image.Repository] = append(tags[image.Repository], image.Version)
	}
	_, problems := planBumps(lock.Images, policy, tags, io.Discard)
	for _, problem := range problems {
		t.Error(problem)
	}
}
