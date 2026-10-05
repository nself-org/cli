package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const (
	tableBegin = "<!-- GENERATED:fields BEGIN (tools/manifestv2migrate -wiki; DO NOT HAND EDIT) -->"
	tableEnd   = "<!-- GENERATED:fields END -->"
)

// fieldTable renders the top-level keys of the schema as a markdown table.
func fieldTable(schemaJSON []byte) (string, error) {
	var doc struct {
		Properties map[string]map[string]any `json:"properties"`
		Required   []string                  `json:"required"`
	}
	if err := json.Unmarshal(schemaJSON, &doc); err != nil {
		return "", err
	}
	req := map[string]bool{}
	for _, r := range doc.Required {
		req[r] = true
	}
	keys := make([]string, 0, len(doc.Properties))
	for k := range doc.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("| Key | Type | Required | Rule |\n|---|---|---|---|\n")
	for _, k := range keys {
		p := doc.Properties[k]
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", k, typeOf(p), yesNo(req[k]), rule(p))
	}
	return b.String(), nil
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func typeOf(p map[string]any) string {
	switch t := p["type"].(type) {
	case string:
		return t
	case []any:
		var parts []string
		for _, v := range t {
			if s, _ := v.(string); s != "null" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "") + " or null"
	}
	if _, ok := p["oneOf"]; ok {
		return "array or object"
	}
	return "any"
}

func rule(p map[string]any) string {
	var parts []string
	if d, _ := p["description"].(string); d != "" {
		parts = append(parts, d)
	}
	if e, ok := p["enum"].([]any); ok {
		var vals []string
		for _, v := range e {
			vals = append(vals, fmt.Sprint(v))
		}
		parts = append(parts, "one of "+strings.Join(vals, ", "))
	}
	if s, _ := p["pattern"].(string); s != "" {
		parts = append(parts, "pattern `"+s+"`")
	}
	if c, ok := p["const"]; ok {
		parts = append(parts, fmt.Sprintf("always %v", c))
	}
	return strings.ReplaceAll(strings.Join(parts, "; "), "|", "\\|")
}

// spliceTable replaces the region between the GENERATED markers of page.
func spliceTable(page, table string) (string, error) {
	i, j := strings.Index(page, tableBegin), strings.Index(page, tableEnd)
	if i < 0 || j < i {
		return "", fmt.Errorf("page has no %q ... %q region", tableBegin, tableEnd)
	}
	return page[:i+len(tableBegin)] + "\n\n" + table + "\n" + page[j:], nil
}

// runWiki regenerates the field table of the wiki page at path.
func runWiki(path, schema string, stderr io.Writer) int {
	sch, err := os.ReadFile(schema)
	if err != nil {
		say(stderr, "manifestv2migrate: %v\n", err)
		return 2
	}
	page, err := os.ReadFile(path)
	if err != nil {
		say(stderr, "manifestv2migrate: %v\n", err)
		return 2
	}
	table, err := fieldTable(sch)
	if err == nil {
		var out string
		if out, err = spliceTable(string(page), table); err == nil {
			err = os.WriteFile(path, []byte(out), 0o644)
		}
	}
	if err != nil {
		say(stderr, "manifestv2migrate: %v\n", err)
		return 2
	}
	return 0
}
