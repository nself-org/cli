package compose

import "strings"

// buildVolumes returns the docker-compose volume definitions.
// Core volumes (postgres_data) are always included.
// Optional service volumes are added conditionally based on config.
func (g *Generator) buildVolumes() map[string]VolumeConfig {
	vols := map[string]VolumeConfig{
		// POSTGRES_DATA_VOLUME names the volume `nself db image switch` moved the
		// data to; unset renders exactly the volume compose names itself.
		"postgres_data": {Name: strings.TrimSpace(g.cfg.Postgres.DataVolume)},
	}

	if g.cfg.Redis.Enabled {
		vols[g.cacheVolume()] = VolumeConfig{}
	}

	if g.cfg.Minio.Enabled {
		vols["minio_data"] = VolumeConfig{}
	}

	if g.cfg.Search.Enabled {
		switch g.cfg.Search.Engine {
		case "meilisearch":
			vols["meilisearch_data"] = VolumeConfig{}
		case "typesense":
			vols["typesense_data"] = VolumeConfig{}
		}
	}

	// MLflow: volumes handled by nself-mlflow plugin
	// Monitoring: volumes handled by nself-monitoring plugin

	if g.cfg.Admin.Enabled {
		vols["nself_admin_data"] = VolumeConfig{}
	}

	return vols
}
