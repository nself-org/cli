package build

import (
	"fmt"
	"github.com/nself-org/cli/internal/config"
)

// validateTraefikPorts refuses a swap whose plugin would publish different ports.
// The plugin currently defaults to 80/443 and does not consume nginx settings.
func validateTraefikPorts(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if (cfg.Nginx.HTTPPort != 0 && cfg.Nginx.HTTPPort != 80) || (cfg.Nginx.SSLPort != 0 && cfg.Nginx.SSLPort != 443) {
		return fmt.Errorf("traefik swap requires NGINX_HTTP_PORT=80 and NGINX_HTTPS_PORT=443; custom nginx ports are not supported by the traefik plugin")
	}
	return nil
}
