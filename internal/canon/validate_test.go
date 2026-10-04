package canon

import (
	"fmt"
	"strings"
	"testing"
)

// verbsHeader builds a valid document head so each case isolates one rule.
func verbsHeader(verbs string) string {
	return "schema_version: 1\nverbs: [" + verbs + "]\ncommands:\n"
}

// TestValidationRules has one failing fixture per canon-side rule; each error
// must name the offending path.
func TestValidationRules(t *testing.T) {
	var twentyOne []string
	for i := 0; i < 21; i++ {
		twentyOne = append(twentyOne, fmt.Sprintf("v%d", i))
	}
	cases := []struct {
		name, doc, wantPath, wantMsg string
	}{
		{"schema version", "schema_version: 9\nverbs: [a]\ncommands: {}\n", "schema_version", "not supported"},
		{"verbs ceiling", "schema_version: 1\nverbs: [" + strings.Join(twentyOne, ",") + "]\ncommands: {}\n", "verbs", "ceiling of 20"},
		{"verbs duplicate", "schema_version: 1\nverbs: [a, a]\ncommands: {}\n", "verbs", "twice"},
		{"canon enum", verbsHeader("a") + "  a: {canon: wizard}\n", `commands["a"]`, "not one of"},
		{"top-level canon required", verbsHeader("a") + "  zed: {}\n", `commands["zed"]`, "canon is required"},
		{"depth1 subcommand", verbsHeader("a") + "  zed: {canon: subcommand}\n", `commands["zed"]`, "depth >= 2"},
		{"depth2 core", verbsHeader("a") + "  a b: {canon: core}\n", `commands["a b"]`, "not allowed at depth >= 2"},
		{"depth2 pending", verbsHeader("a") + "  a b: {canon: pending}\n", `commands["a b"]`, "not allowed at depth >= 2"},
		{"core not in verbs", verbsHeader("a") + "  zed: {canon: core}\n", `commands["zed"]`, "not in verbs"},
		{"verb not core", verbsHeader("a") + "  a: {canon: pending}\n", `commands["a"]`, "must be core"},
		{"plugin rejected", verbsHeader("a") + "  zed: {canon: plugin}\n", `commands["zed"]`, "rejected on cobra-native"},
		{"shim needs target", verbsHeader("a") + "  zed: {canon: deprecated-shim}\n", `commands["zed"]`, "requires target"},
		{"target without shim", verbsHeader("a") + "  a: {canon: core, target: x}\n", `commands["a"]`, "only allowed on a deprecated-shim"},
		{"target self", verbsHeader("a") + "  zed: {canon: deprecated-shim, target: zed}\n", `commands["zed"]`, "itself"},
		{"side_effect enum", verbsHeader("a") + "  a: {canon: core, side_effect: boom}\n", `commands["a"]`, "side_effect"},
		{"output enum", verbsHeader("a") + "  a: {canon: core, output: pipe}\n", `commands["a"]`, "output"},
		{"json envelope in yaml", verbsHeader("a") + "  a: {canon: core, json: envelope}\n", `commands["a"]`, "may not be declared in YAML"},
		{"json enum", verbsHeader("a") + "  a: {canon: core, json: maybe}\n", `commands["a"]`, "json"},
		{"exit code non-integer", verbsHeader("a") + "  a: {canon: core, exit_codes: {x: y}}\n", `commands["a"] exit_codes`, "integer 0-125"},
		{"exit code range", verbsHeader("a") + "  a: {canon: core, exit_codes: {\"126\": y}}\n", `commands["a"] exit_codes`, "integer 0-125"},
		{"exit code v15 leading zero", verbsHeader("a") + "  a: {canon: core, exit_codes_v15: {\"010\": y}}\n", `commands["a"] exit_codes_v15`, "integer 0-125"},
		{"flag side_effect enum", verbsHeader("a") + "  a: {canon: core, flags: {f: {side_effect: boom}}}\n", `commands["a"] flag "f"`, "side_effect"},
		{"flag json envelope", verbsHeader("a") + "  a: {canon: core, flags: {f: {json: envelope}}}\n", `commands["a"] flag "f"`, "may not be declared in YAML"},
		{"flag output not stream", verbsHeader("a") + "  a: {canon: core, flags: {f: {output: interactive}}}\n", `commands["a"] flag "f"`, "only flag-level value is stream"},
		{"flag name dashes", verbsHeader("a") + "  a: {canon: core, flags: {--f: {side_effect: write}}}\n", `commands["a"] flag "--f"`, "without dashes"},
		{"key prefix", verbsHeader("a") + "  \"nself  a\": {canon: core}\n", `commands["nself  a"]`, "single-space-separated"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.doc))
			if err == nil {
				t.Fatal("Parse accepted an invalid document")
			}
			msg := err.Error()
			if !strings.Contains(msg, c.wantPath) || !strings.Contains(msg, c.wantMsg) {
				t.Errorf("error does not name %q with %q:\n%s", c.wantPath, c.wantMsg, msg)
			}
		})
	}
}
