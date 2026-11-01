package scaffold

// scaffold_render.go — template rendering and naming helpers.
//
// Purpose: render a named template to a string, and convert plugin names into PascalCase identifiers, env var prefixes and license text, used throughout scaffold.go, split out for file size.
// Inputs: a template name/body and template data, or a raw plugin name/tier.
// Outputs: a rendered string, or a derived identifier/license string.
// Constraints: pure move from scaffold.go (CLI-R12 Batch F); no behaviour change.

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/nself-org/cli/internal/compose"
)

// render executes a Go template with the given params, returning the result.
// Returns an error if template parsing or execution fails.
func render(tmpl string, p Params) (string, error) {
	t, err := template.New("").Funcs(template.FuncMap{"image": scaffoldImage}).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("scaffold: template parse error: %w", err)
	}
	var buf strings.Builder
	if err := t.Execute(&buf, p); err != nil {
		return "", fmt.Errorf("scaffold: template execute error: %w", err)
	}
	return buf.String(), nil
}

// renderAnyErr is like renderAny but returns an error instead of panicking.
// Used in paths where the caller can propagate the error (e.g. buildFiles).
func renderAnyErr(tmpl string, data any) (string, error) {
	t, err := template.New("").Funcs(template.FuncMap{"image": scaffoldImage}).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("scaffold: template parse error: %w", err)
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("scaffold: template execute error: %w", err)
	}
	return buf.String(), nil
}

// --- helpers ---

func toPascal(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

func toEnvPrefix(s string) string {
	return strings.ToUpper(strings.ReplaceAll(s, "-", "_"))
}

func licenseForTier(tier string) string {
	if tier == "pro" {
		return "Source-Available"
	}
	return "MIT"
}

// scaffoldImage resolves the supported Dockerfile bases using the active lock mode.
func scaffoldImage(name string) (string, error) {
	images := map[string]string{
		"golang":            compose.ImageRef("golang", ""),
		"distroless-static": compose.ImageRef("distroless-static", ""),
		"node":              compose.ImageRef("node", ""),
		"rust":              compose.ImageRef("rust", ""),
		"alpine":            compose.ImageRef("alpine", ""),
		"nginx":             compose.ImageRef("nginx", ""),
	}
	if ref := images[name]; ref != "" {
		return ref, nil
	}
	return "", fmt.Errorf("unknown image lock entry %q", name)
}
