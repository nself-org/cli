package main

// Purpose: render the generated operation table inside the package README.
// Inputs: operation views, current README bytes. Outputs: README bytes.
// Constraints: only the marked region changes; missing markers fail the run.

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	readmeBegin = "<!-- BEGIN GENERATED: operations (tools/sdkgen, DO NOT HAND EDIT) -->"
	readmeEnd   = "<!-- END GENERATED: operations -->"
)

// ReplaceReadmeRegion swaps the generated block of readme for the operation table.
func ReplaceReadmeRegion(readme []byte, ops []opView) ([]byte, error) {
	s := string(readme)
	i, j := strings.Index(s, readmeBegin), strings.Index(s, readmeEnd)
	if i < 0 || j < i {
		return nil, fmt.Errorf("sdkgen: README.md lacks the %q ... %q markers", readmeBegin, readmeEnd)
	}
	var b bytes.Buffer
	b.WriteString(s[:i] + readmeBegin + "\n\n")
	fmt.Fprintf(&b, "%d operations. Every method takes `{ args, flags }` (plus `confirm`, or the free-form `argv` of a plugin command that declares no args or flags, where the operation declares it) and resolves to the typed `data` member.\n\n", len(ops))
	b.WriteString("| Method | Route | Summary |\n| --- | --- | --- |\n")
	for _, o := range ops {
		fmt.Fprintf(&b, "| `%s` | `POST %s` | %s |\n", o.Method, o.Path, strings.ReplaceAll(o.Summary, "|", "\\|"))
	}
	b.WriteString("\n" + s[j:])
	return b.Bytes(), nil
}
