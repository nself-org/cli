// Purpose: project the committed fact sources (.github/VERSION,
// .github/command-registry.json, .github/openapi.json) into the public facts
// document (contract:cli.public-facts v1).
// Inputs: the three files' bytes. Outputs: model.Facts and its canonical JSON
// (2-space indent, trailing newline).
// Constraints: a pure function of its inputs; no clock, no environment.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/cli/tools/facts/model"
)

var (
	versionRE = regexp.MustCompile(model.VersionPattern)
	semverRE  = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)`)
)

// registryDoc is the part of the command registry the facts read.
type registryDoc struct {
	Verbs  []string `json:"verbs"`
	Counts *struct {
		Commands    int      `json:"commands"`
		Core        int      `json:"core"`
		CoreMissing []string `json:"core_missing"`
		TopLevel    int      `json:"top_level"`
	} `json:"counts"`
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Build projects the sources into a Facts document. openapi is nil when the
// file is absent.
func Build(version, registry, openapi []byte) (model.Facts, error) {
	v := strings.TrimSpace(string(version))
	if !versionRE.MatchString(v) {
		return model.Facts{}, fmt.Errorf("VERSION %q is not a semantic version", v)
	}
	var reg registryDoc
	if err := json.Unmarshal(registry, &reg); err != nil {
		return model.Facts{}, fmt.Errorf("command-registry.json: %w", err)
	}
	if len(reg.Verbs) == 0 {
		return model.Facts{}, fmt.Errorf("command-registry.json: no verbs")
	}
	if reg.Counts == nil || reg.Counts.TopLevel != len(reg.Verbs) {
		return model.Facts{}, fmt.Errorf("command-registry.json: counts.top_level must exist and equal the number of verbs (%d)", len(reg.Verbs))
	}
	f := model.Facts{
		Generated:    model.Generated,
		ContractMode: model.ContractMode,
		Counts: model.Counts{
			Commands:    reg.Counts.Commands,
			Core:        reg.Counts.Core,
			CoreMissing: append([]string{}, reg.Counts.CoreMissing...),
			TopLevel:    reg.Counts.TopLevel,
		},
		SchemaVer: model.SchemaVersion,
		Sources:   model.Sources{RegistrySHA256: sha256Hex(registry)},
		Verbs:     append([]string{}, reg.Verbs...),
		Version:   v,
	}
	if openapi != nil {
		h := sha256Hex(openapi)
		f.Sources.OpenAPISHA256 = &h
	}
	return f, nil
}

// Marshal writes the canonical document bytes.
func Marshal(f model.Facts) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// belowV15 reports whether version is older than 1.5.0, the release where the
// v1.5 surface becomes the default. An unparsable version counts as below.
func belowV15(version string) bool {
	m := semverRE.FindStringSubmatch(version)
	if m == nil {
		return true
	}
	var maj, min int
	_, _ = fmt.Sscanf(m[1]+" "+m[2], "%d %d", &maj, &min)
	return maj < 1 || (maj == 1 && min < 5)
}
