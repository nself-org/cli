package build

import (
	"strings"
	"testing"
)

// TestPluginServiceRange: the state and the exact [start,end) text picked for
// plugin "w" in each fragment shape.
func TestPluginServiceRange(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		state serviceRangeState
		text  string // the picked text, for rangeFound; rangeWhole is the whole input
	}{
		{"sidecar first", "services:\n  side:\n    image: a\n  w:\n    image: b\n  last:\n    image: c\nnetworks:\n  n: {}\n", rangeFound, "  w:\n    image: b\n"},
		{"last, nothing after", "services:\n  side:\n    image: a\n  w:\n    image: b\n", rangeFound, "  w:\n    image: b\n"},
		{"last, top-level key after", "services:\n  side:\n    image: a\n  w:\n    image: b\nnetworks:\n  n: {}\n", rangeFound, "  w:\n    image: b\n"},
		{"comment between services", "services:\n  w:\n    image: b\n  # about side\n  side:\n    image: a\n", rangeFound, "  w:\n    image: b\n  # about side\n"},
		{"crlf", "services:\r\n  side:\r\n    image: a\r\n  w:\r\n    image: b\r\n", rangeFound, "  w:\r\n    image: b\r\n"},
		{"nself- prefix", "services:\n  side:\n    image: a\n  nself-w:\n    image: b\n", rangeFound, "  nself-w:\n    image: b\n"},
		{"single other key", "services:\n  only:\n    image: a\n", rangeFound, "  only:\n    image: a\n"},
		{"flow services", "services: {side: {image: a}, w: {image: b}}\n", rangeUnsafe, ""},
		{"multi-line flow services", "services:\n  {\n    side: {image: a},\n    w: {image: b}\n  }\n", rangeUnsafe, ""},
		{"services alias", "x: &s\n  side:\n    image: a\n  w:\n    image: b\nservices: *s\n", rangeUnsafe, ""},
		{"top-level merge carries services", "x: &b\n  services:\n    side:\n      image: a\n    w:\n      image: b\n<<: *b\n", rangeUnsafe, ""},
		{"alias key", "x-n: &w side\nservices:\n  *w : {image: a}\n  w2:\n    image: b\n", rangeUnsafe, ""},
		{"picked service merges, no own environment", "x: &base\n  environment:\n    K: \"1\"\nservices:\n  side:\n    image: s\n  w:\n    <<: *base\n    image: b\n", rangeUnsafe, ""},
		{"picked service merges and has environment", "x: &base\n  image: a\nservices:\n  side:\n    image: s\n  w:\n    <<: *base\n    environment:\n      A: \"1\"\n", rangeFound, "  w:\n    <<: *base\n    environment:\n      A: \"1\"\n"},
		{"multi-document", "services:\n  w:\n    image: a\n---\nservices:\n  w:\n    image: b\n", rangeUnsafe, ""},
		{"unparseable", "services: [\n  w:\n", rangeWhole, "services: [\n  w:\n"},
		{"no services", "networks:\n  n: {}\n", rangeWhole, "networks:\n  n: {}\n"},
		{"several services, no match", "services:\n  a:\n    image: x\n  b:\n    image: y\n", rangeNoMatch, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end, state := pluginServiceRange([]byte(c.in), "w", "")
			if state != c.state {
				t.Fatalf("state %d, want %d", state, c.state)
			}
			if state != rangeFound && state != rangeWhole {
				return
			}
			if got := c.in[start:end]; got != c.text {
				t.Errorf("range text %q, want %q", got, c.text)
			}
		})
	}
}

// TestCoreEnvTarget_UnsafeShapesWarnAndStayUnchanged: the repro shapes from
// the review (sidecars behind flow style or an alias, an anchored environment).
func TestCoreEnvTarget_UnsafeShapesWarnAndStayUnchanged(t *testing.T) {
	cases := map[string]string{
		"x-side anchor and flow services": "x-side: &side\n  environment:\n    FOO: bar\nservices: {side: *side, w: {image: b}}\n",
		"anchored environment":            "services:\n  w:\n    image: x\n    environment: &e\n      A: \"1\"\n    networks:\n      - ${DOCKER_NETWORK}\n",
		"aliased environment":             "x: &e\n  A: \"1\"\nservices:\n  w:\n    image: x\n    environment: *e\n    networks:\n      - ${DOCKER_NETWORK}\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			pluginDir := t.TempDir()
			writePluginManifestJSON(t, pluginDir, "w", 3777)
			resetWarnOnce()
			var out string
			stderr := captureStderr(t, func() { out = string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "w")) })
			if out != in {
				t.Errorf("fragment must be unchanged:\n%s", out)
			}
			if !strings.Contains(stderr, `plugin "w"`) {
				t.Errorf("expected a warning naming the plugin, got %q", stderr)
			}
			if strings.Count(out, "environment:") > strings.Count(in, "environment:") {
				t.Errorf("duplicate environment key:\n%s", out)
			}
		})
	}
}

// TestCoreEnvTarget_NoMatchPortZeroIsSilent: a pure third-party stack (port 0,
// several services) gets nothing injected and no warning.
func TestCoreEnvTarget_NoMatchPortZeroIsSilent(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "w", 0)
	in := "services:\n  one:\n    image: a\n    environment:\n      A: \"1\"\n  two:\n    image: b\n"
	resetWarnOnce()
	var out string
	stderr := captureStderr(t, func() { out = string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "w")) })
	if out != in || stderr != "" {
		t.Errorf("want unchanged and silent, got stderr %q out:\n%s", stderr, out)
	}
}
