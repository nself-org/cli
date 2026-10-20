package main

import (
	"os"
	"testing"
)

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
	for _, image := range lock.Images {
		if image.Role == "plugin" {
			continue
		}
		t.Run(image.Name, func(t *testing.T) {
			rule, ok := policy.rules[image.Name]
			if !ok {
				t.Fatal("missing bump policy")
			}
			if !rule.re.MatchString(image.Version) {
				t.Fatal("current version does not match policy")
			}
		})
	}
}
