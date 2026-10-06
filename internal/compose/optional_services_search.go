package compose

// optional_services_search.go — MeiliSearch and Typesense optional compose services.
//
// Purpose: build the compose service definitions for MeiliSearch (including its init service) and Typesense, used by the compose builder alongside optional_services.go, split out for file size.
// Inputs: the loaded Config identifying whether these services are enabled and their settings.
// Outputs: compose service definitions for MeiliSearch and Typesense.
// Constraints: pure move from optional_services.go (CLI-R12 Batch E); no behaviour change.

import (
	"fmt"
)

// buildMeiliInitService returns the MeiliSearch initialization service configuration.
// Uses the same init-container pattern as MinIO.
func (g *Generator) buildMeiliInitService() ServiceConfig {
	return ServiceConfig{
		Image:         ImageRef("meilisearch-init", ""),
		ContainerName: fmt.Sprintf("%s_meilisearch_init", g.cfg.ProjectName),
		Restart:       "no",
		User:          "root",
		Networks:      []string{g.cfg.DockerNetwork},
		Volumes:       []string{"meilisearch_data:/data"},
		Command:       `sh -c "chown -R 1000:1000 /data; chmod -R 755 /data"`,
		Labels: map[string]string{
			"nself.type":        "init-container",
			"nself.service":     "meilisearch",
			"nself.auto-remove": "true",
		},
		// No Profiles — init containers must be visible to depends_on
	}
}

// buildMeiliService returns the MeiliSearch service configuration.
func (g *Generator) buildMeiliService() ServiceConfig {
	sc := g.cfg.Search
	mc := sc.MeiliSearch

	version := mc.Version
	if version == "" {
		version = "latest"
	}
	port := sc.Port
	if port == 0 {
		port = 7700
	}
	env := mc.Env
	if env == "" {
		env = "development"
	}

	return ServiceConfig{
		Image:         ResolveImage("meilisearch", ImageRef("meilisearch", version)),
		ContainerName: fmt.Sprintf("%s_meilisearch", g.cfg.ProjectName),
		Restart:       "unless-stopped",
		User:          "1000:1000",
		Networks:      []string{g.cfg.DockerNetwork},
		DependsOn: map[string]DepOn{
			"meilisearch-init": {Condition: "service_completed_successfully"},
		},
		Environment: map[string]string{
			"MEILI_ENV":        env,
			"MEILI_MASTER_KEY": mc.MasterKey,
		},
		Command: "meilisearch --db-path=/meili_data",
		Volumes: []string{"meilisearch_data:/meili_data"},
		Ports:   []string{fmt.Sprintf("127.0.0.1:%d:7700", port)},
		Healthcheck: &Healthcheck{
			Test:     []string{"CMD", "curl", "-f", "http://localhost:7700/health"},
			Interval: "30s",
			Timeout:  "10s",
			Retries:  3,
		},
	}
}

// MLflow: moved to nself-mlflow free plugin (not in core)

// buildTypesenseService returns the Typesense search service configuration.
func (g *Generator) buildTypesenseService() ServiceConfig {
	sc := g.cfg.Search
	tc := sc.Typesense

	version := tc.Version
	if version == "" {
		version = "27.1"
	}
	port := sc.Port
	if port == 0 {
		port = 8108
	}
	enableCORS := "true"
	if !tc.EnableCORS {
		enableCORS = "false"
	}
	logLevel := tc.LogLevel
	if logLevel == "" {
		logLevel = "info"
	}

	return ServiceConfig{
		Image:         ResolveImage("typesense", ImageRef("typesense", version)),
		ContainerName: fmt.Sprintf("%s_typesense", g.cfg.ProjectName),
		Restart:       "unless-stopped",
		Networks:      []string{g.cfg.DockerNetwork},
		Environment: map[string]string{
			"TYPESENSE_API_KEY":     tc.APIKey,
			"TYPESENSE_ENABLE_CORS": enableCORS,
			"TYPESENSE_LOG_LEVEL":   logLevel,
		},
		Command: fmt.Sprintf("--data-dir /data --api-key=%s --enable-cors", tc.APIKey),
		Volumes: []string{"typesense_data:/data"},
		Ports:   []string{fmt.Sprintf("127.0.0.1:%d:8108", port)},
		Healthcheck: &Healthcheck{
			Test:     []string{"CMD", "curl", "-f", "-H", fmt.Sprintf("X-TYPESENSE-API-KEY: %s", tc.APIKey), "http://localhost:8108/health"},
			Interval: "30s",
			Timeout:  "10s",
			Retries:  3,
		},
	}
}
