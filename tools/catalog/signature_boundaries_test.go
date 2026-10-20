package main

import (
	"github.com/nself-org/cli/internal/plugin/manifestv2"
	"github.com/nself-org/cli/tools/catalog/model"
	"testing"
)

func TestLicensedPlatformSignaturesRefused(t *testing.T) {
	for _, signed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsigned", true: "signed"}[signed], func(t *testing.T) {
			m := &manifestv2.Manifest{Name: "fixture", License: manifestv2.LicenseLicensed, Version: "1.0.0"}
			r := model.Release{Version: "1.0.0"}
			if signed {
				r.PlatformSignatures = map[string]*model.Signature{"linux-amd64": nil}
			}
			_, rep := buildRegistry(licensedTier, map[string]*manifestv2.Manifest{"fixture": m}, &model.Releases{Releases: map[string]model.Release{"fixture": r}}, &model.Bundles{}, nil, model.GeneratedFrom{}, false)
			if (len(rep.problems) > 0) != signed {
				t.Fatalf("problems=%v, signed=%v", rep.problems, signed)
			}
		})
	}
}

func TestCatalogPlatformSignaturesValidated(t *testing.T) {
	for _, platform := range []string{"linux-amd64", "unknown"} {
		t.Run(platform, func(t *testing.T) {
			e := model.RegistryEntry{Catalog: &model.Info{}, PlatformSignatures: map[string]*model.Signature{platform: nil}}
			if _, problem := rowOf("fixture", model.TierFree, e, &model.Bundles{}); problem == "" {
				t.Fatal("accepted incomplete platform signature")
			}
		})
	}
}
