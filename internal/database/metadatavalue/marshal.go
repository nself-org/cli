// Package metadatavalue encodes values for the textual Hasura include resolver.
package metadatavalue

import (
	"encoding/json"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var metadataKeepChomp = regexp.MustCompile(`(?m)[|>](?:[1-9]?\+|\+[1-9])\n`)

// Marshal encodes one top-level value for its <key>.yaml file:
// block YAML, except single-line bodies, keep-chomp scalars, and "!include", which
// become one line of JSON with "!include" escaped.
func Marshal(v interface{}) ([]byte, error) {
	body, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	oneLine := strings.Count(strings.TrimRight(string(body), "\n"), "\n") == 0
	// The textual include resolver trims terminal newlines, so keep-chomp
	// scalars must use JSON escapes to preserve their exact string values.
	if !oneLine && !strings.Contains(string(body), "!include") && !metadataKeepChomp.Match(body) {
		return body, nil
	}
	js, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	esc := strings.ReplaceAll(string(js), "!include", `\u0021include`)
	return []byte(esc + "\n"), nil
}
