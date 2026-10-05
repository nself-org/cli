package manifestv2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// Load reads a plugin.json and returns the validated v2 Manifest. A v2 file is
// decoded strictly and checked (Validate, CheckCompat); a v1 file (no
// manifest_version, or 1) is converted by the one normalizer (Normalize). Under
// compat.V15() a v1 file also prints the once-per-process deprecation notice.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading plugin manifest: %w", err)
	}
	m, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// Parse is Load for bytes already read.
func Parse(data []byte) (*Manifest, error) { return parse(data, true) }

// ParseQuiet is Parse without the v1 deprecation notice, for tools that
// convert v1 files on purpose.
func ParseQuiet(data []byte) (*Manifest, error) { return parse(data, false) }

func parse(data []byte, notice bool) (*Manifest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, invalid("plugin.json", "not a JSON object: "+err.Error())
	}
	ver, err := versionOf(raw)
	if err != nil {
		return nil, err
	}
	if ver == 1 {
		m, err := Normalize(data)
		if err != nil {
			return nil, err
		}
		if notice {
			noteV1(m.Name)
		}
		return m, nil
	}
	m, err := decodeV2(data, raw)
	if err != nil {
		return nil, err
	}
	if err := CheckCompat(m); err != nil {
		return nil, err
	}
	return m, nil
}

// IsV1Version reports whether a raw manifest_version value marks a v1 file.
// Released CLIs ignore the key, so every spelling of "one or nothing" loads as
// v1: null, 0, "1", 1 and 1.0 (an absent key is v1 too).
func IsV1Version(v json.RawMessage) bool {
	t := strings.TrimSpace(string(v))
	switch t {
	case "", "null", `"1"`:
		return true
	}
	if strings.HasPrefix(t, `"`) {
		return false
	}
	f, err := strconv.ParseFloat(t, 64)
	return err == nil && (f == 0 || f == 1)
}

// versionOf returns 1 for a v1 manifest_version (IsV1Version), 2 for 2, else E114.
func versionOf(raw map[string]json.RawMessage) (int, error) {
	v, ok := raw["manifest_version"]
	if !ok || IsV1Version(v) {
		return 1, nil
	}
	if strings.TrimSpace(string(v)) == "2" {
		return 2, nil
	}
	return 0, errs.Newf("E114", "manifest_version %s is not supported (supported: 1 or absent, and 2)", strings.TrimSpace(string(v)))
}

// decodeV2 runs the E111 scan, the strict decode and Validate; it skips
// CheckCompat so the migrate tool can read a file whose compat keys are stale.
func decodeV2(data []byte, raw map[string]json.RawMessage) (*Manifest, error) {
	if bad := forbiddenIn(raw); len(bad) > 0 {
		return nil, errs.Newf("E111", "forbidden key%s in a v2 plugin.json: %s", plural(len(bad)), strings.Join(bad, ", "))
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, invalid("plugin.json", err.Error())
	}
	if dec.More() {
		return nil, invalid("plugin.json", "unexpected data after the JSON object")
	}
	Canonicalize(&m)
	if err := Validate(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeV2 decodes and validates a v2 file without the compatibility check.
func DecodeV2(data []byte) (*Manifest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, invalid("plugin.json", "not a JSON object: "+err.Error())
	}
	ver, err := versionOf(raw)
	if err != nil {
		return nil, err
	}
	if ver != 2 {
		return nil, invalid("manifest_version", "expected 2")
	}
	return decodeV2(data, raw)
}

// forbiddenIn lists the E111 keys present in raw, sorted.
func forbiddenIn(raw map[string]json.RawMessage) []string {
	var bad []string
	for _, k := range ForbiddenKeys {
		if _, ok := raw[k]; ok {
			bad = append(bad, k)
		}
	}
	sort.Strings(bad)
	return bad
}

// invalid builds an E106 naming the offending field.
func invalid(field, msg string) error {
	return errs.Wrap("E106", field+": "+msg, errs.ErrPluginManifest)
}

func invalidf(field, format string, args ...any) error {
	return invalid(field, fmt.Sprintf(format, args...))
}

var itoa = strconv.Itoa

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
