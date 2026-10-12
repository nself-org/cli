package config

import "testing"

func TestServiceContainerName(t *testing.T) {
	cfg := &Config{ProjectName: "acme"}
	for svc, want := range map[string]string{
		"hasura":      "acme_hasura",
		"auth":        "acme_auth",
		"minio":       "acme_minio",
		"nself-admin": "acme_admin",
		"my-api":      "acme_my-api",
	} {
		if got := cfg.ServiceContainerName(svc); got != want {
			t.Errorf("ServiceContainerName(%q) = %q, want %q", svc, got, want)
		}
	}
}
