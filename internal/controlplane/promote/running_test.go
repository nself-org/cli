package promote

import "testing"

func TestMatchRunningIdentities(t *testing.T) {
	row := DigestRow{Service: "web", Source: "sha256:a", Target: "sha256:a"}
	image := SourceImage{Service: "web", Digest: "sha256:a"}
	for _, tc := range []struct {
		name    string
		rows    []DigestRow
		images  []SourceImage
		wantErr bool
	}{
		{"equal", []DigestRow{row}, []SourceImage{image}, false},
		{"no comparison", nil, []SourceImage{image}, true},
		{"missing running service", []DigestRow{row}, nil, true},
		{"changed running digest", []DigestRow{row}, []SourceImage{{Service: "web", Digest: "sha256:b"}}, true},
		{"uncompared service", []DigestRow{row}, []SourceImage{image, {Service: "other", Digest: "sha256:a"}}, true},
		{"unverifiable service", []DigestRow{{Service: "web", Status: "unverifiable"}}, []SourceImage{image}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := MatchRunning(tc.rows, tc.images); (err != nil) != tc.wantErr {
				t.Fatalf("MatchRunning = %v, wantErr %t", err, tc.wantErr)
			}
		})
	}
}
