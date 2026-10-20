package supabase

import (
	"context"
	"testing"

	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/portable"
)

func TestSkipStorageExemptsSourceObjects(t *testing.T) {
	for _, bucket := range []string{"avatars", "Invalid bucket!", "a"} {
		t.Run(bucket, func(t *testing.T) {
			db := detectionDB(t, true, []portable.Object{{Bucket: bucket, Key: "folder/photo.png"}})
			r := &run{skipStorage: true}
			m := portable.Manifest{}
			if err := r.catalogCounts(context.Background(), db, &m); err != nil {
				t.Fatal(err)
			}
			if len(m.Exemptions) != 1 || m.Exemptions[0].ID != bucket+"/folder/photo.png" || m.Exemptions[0].Reason != "--skip-storage" {
				t.Fatalf("exemptions = %+v", m.Exemptions)
			}
			if err := r.finish(context.Background(), nil, &m); err != nil {
				t.Fatal(err)
			}
			if len(m.Storage.Objects) != 0 {
				t.Fatal("skipped objects were exported")
			}
		})
	}
}

func TestSkipStorageNeedsNoAPICredentials(t *testing.T) {
	for _, values := range []map[string]string{{}, {"api-url": "invalid"}, {"project-ref": "invalid!"}} {
		a, err := apiFor(importer.Input{Values: values, SkipStorage: true})
		if err != nil || a != nil {
			t.Fatal("skipped storage requested API credentials")
		}
	}
}
