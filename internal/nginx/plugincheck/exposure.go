package plugincheck

// exposure.go — the decision `nself build` makes for each plugin nginx conf:
// inject it, or leave it out of nginx/sites and say why and how to fix it.
//
// Purpose: one call (Leave) for the injection step and for routes.json, so
// both see the same set. A conf is left out when nginx would refuse it (Skip)
// or when it would publish the plugin without an access gate (D-0777):
// nginx/sites is served on the public listeners and the caller picks Host and
// SNI, so DNS keeps nothing private. A conf is public only when it is
// loopback-gated or the plugin's plugin.json declares
// `"nginx": {"public": true}`, its statement that it authenticates every
// request the conf routes or verifies a webhook signature on it.
// Inputs: the plugin directory, the plugin name, the rendered conf, the
// template vars and whether the build serves TLS.
// Outputs: Leave's reason and fix ("" = inject); OwnerOf and DropOwn for the
// owner line the build stamps on each injected file.
// Constraints: the default is closed. An unreadable or malformed plugin.json
// is not public.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HoldReason is the reason given for an ungated conf without the declaration.
const HoldReason = "it would publish the plugin on the public nginx without an access gate, and the plugin's manifest does not declare the route public"

// Public reports whether pluginDir/<name>/plugin.json declares nginx.public
// true. It decodes permissively, so v1 and v2 manifests read the same way.
func Public(pluginDir, name string) bool {
	data, err := os.ReadFile(filepath.Join(pluginDir, name, "plugin.json"))
	if err != nil {
		return false
	}
	var m struct {
		Nginx *struct {
			Public bool `json:"public"`
		} `json:"nginx"`
	}
	return json.Unmarshal(data, &m) == nil && m.Nginx != nil && m.Nginx.Public
}

// Leave returns why a rendered plugin conf stays out of nginx/sites and how to
// fix that, or two empty strings to inject it.
func Leave(pluginDir, name, rendered string, vars map[string]string, hasSSL bool) (why, fix string) {
	if why := Skip(rendered, vars, hasSSL); why != "" {
		return why, "update the plugin (nself plugin update " + name + ") or report it to the plugin, then run nself build again"
	}
	if LoopbackGated(rendered) || Public(pluginDir, name) {
		return "", ""
	}
	return HoldReason, fmt.Sprintf("reach %s on the project network, or put your own server block with authentication in nginx/conf.d; "+
		"if %s authenticates every request or verifies a webhook signature, its plugin.json sets \"nginx\": {\"public\": true} "+
		"(update it with nself plugin update %s), then run nself build again", name, name, name)
}

// OwnerPrefix starts the owner line the build stamps after the generated
// marker of each injected file ("# nself-owner: <project>/<plugin>").
const OwnerPrefix = "# nself-owner: "

// OwnerOf returns the owner named in content's first lines, or "".
func OwnerOf(content string) string {
	for i, line := range strings.SplitN(content, "\n", 6) {
		if i == 5 {
			break
		}
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), OwnerPrefix); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// Sites is the part of the build's file sink DropOwn uses.
type Sites interface {
	ReadFile(path string) ([]byte, error)
	Remove(path string) error
}

// DropOwn removes path when its owner line names owner: a conf left out of
// this build must not keep serving from an earlier one (a fronting stack's
// sites dir is not swept by this project's prune). Another owner's file, or a
// missing one, is left alone.
func DropOwn(s Sites, path, owner string) error {
	data, err := s.ReadFile(path)
	if err != nil || OwnerOf(string(data)) != owner {
		return nil
	}
	if err := s.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s, left out of nginx/sites: %w", path, err)
	}
	return nil
}
