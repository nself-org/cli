package build

import (
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestD0609PluginPlaceholderScope(t *testing.T) {
	vars := buildTemplateVars(t.TempDir(), &config.Config{BaseDomain: "example.com"})
	for name, text := range map[string]string{
		"comment":            "# ${BASE_DOMAIN} ${UNKNOWN}\nserver { listen 80; }",
		"inline comment":     "server { listen 80; # ${SSL_DIR}\n }",
		"nginx map variable": "map $host $UPSTREAM { default http://api:8080; } server { location / { proxy_pass ${UPSTREAM}; } }",
	} {
		t.Run(name, func(t *testing.T) {
			if why := pluginSnippetSkip(text, vars, true); why != "" {
				t.Errorf("valid snippet rejected: %s", why)
			}
		})
	}
	if why := pluginSnippetSkip("server { server_name ${BASE_DOMAIN}; }", vars, true); why == "" {
		t.Error("unresolved known template name must still be rejected")
	}
}
