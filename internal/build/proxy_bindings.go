package build

// Purpose: refuse host bindings the Traefik fragment cannot preserve (D-0794).
// Inputs: the resolved nginx configuration, including environment defaults.
// Outputs: binding losses for the existing E481 preflight diagnostic.
// Constraints: read-only; Traefik publishes 80/443 on every host address.
import (
	"fmt"

	"github.com/nself-org/cli/internal/config"
)

func proxyBindingLimitations(cfg *config.Config) []string {
	var items []string
	if port := cfg.Nginx.HTTPPort; port != 0 && port != 80 {
		items = append(items, fmt.Sprintf("NGINX_HTTP_PORT=%d: traefik publishes host port 80", port))
	}
	if port := cfg.Nginx.SSLPort; port != 0 && port != 443 {
		items = append(items, fmt.Sprintf("NGINX_HTTPS_PORT/NGINX_SSL_PORT=%d: traefik publishes host port 443", port))
	}
	if ip := cfg.Nginx.BindIP; ip != "" && ip != "0.0.0.0" {
		items = append(items, fmt.Sprintf("NGINX_BIND_IP=%s: traefik publishes on every host address", ip))
	}
	return items
}
