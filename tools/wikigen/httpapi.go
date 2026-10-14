package main

// HTTP-API-Reference.md: the contract and every operation of `nself admin serve`.
// Purpose: the page is the committed contract text plus one row per OpenAPI
// operation, so it cannot describe a route the server does not have.
// Inputs: internal/httpapi/contract.md (exact bytes), .github/openapi.json and
// the hermetic registry (side effect and streaming of each operation).
// Outputs: the page text. Constraints: the contract bytes follow the generated
// header unchanged; rows are sorted by path then method.

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
)

// httpOp is one operation row.
type httpOp struct {
	Method, Path, ID, Summary string
}

func renderHTTPAPI(reg *cmdregistry.Registry, src machineSources) (string, error) {
	contract, err := os.ReadFile(src.contract)
	if err != nil {
		return "", err
	}
	spec, err := os.ReadFile(src.openapi)
	if err != nil {
		return "", err
	}
	byName := map[string]*cmdregistry.Command{}
	for i := range reg.Commands {
		byName[cmdregistry.ToolName(reg.Commands[i].Path)] = &reg.Commands[i]
	}
	return renderHTTPPage(string(contract), spec, func(id string) *cmdregistry.Command { return byName[id] })
}

// openAPIOperations lists the operations of an OpenAPI document.
func openAPIOperations(spec []byte) ([]httpOp, error) {
	var doc struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
			Summary     string `json:"summary"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		return nil, fmt.Errorf("openapi: %w", err)
	}
	var ops []httpOp
	for path, methods := range doc.Paths {
		for method, op := range methods {
			if method != "get" && method != "post" {
				continue
			}
			if op.OperationID == "" {
				return nil, fmt.Errorf("openapi: %s %s has no operationId", strings.ToUpper(method), path)
			}
			ops = append(ops, httpOp{strings.ToUpper(method), path, op.OperationID, op.Summary})
		}
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Path != ops[j].Path {
			return ops[i].Path < ops[j].Path
		}
		return ops[i].Method < ops[j].Method
	})
	return ops, nil
}

func renderHTTPPage(contract string, spec []byte, resolve func(string) *cmdregistry.Command) (string, error) {
	ops, err := openAPIOperations(spec)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(generatedNotice + "\n")
	b.WriteString(contract)
	if !strings.HasSuffix(contract, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n## Operations\n\nOne row per operation of the committed OpenAPI document. Side effect is the highest class the operation reaches through its exposed flags; streaming says when `Accept: application/x-ndjson` selects NDJSON.\n\n")
	b.WriteString("| Method | Path | Summary | Side effect | Streaming |\n|---|---|---|---|---|\n")
	for _, op := range ops {
		side, stream := canon.SideEffectRead, "no" // /openapi.json has no command behind it
		if cmd := resolve(op.ID); cmd != nil {
			side, stream = httpSide(cmd), httpStreaming(cmd)
		} else if op.ID != "openapi" {
			return "", fmt.Errorf("operation %s has no registry command", op.ID)
		}
		fmt.Fprintf(&b, "| %s | `%s` | %s | `%s` | %s |\n", op.Method, op.Path, cell(op.Summary), side, stream)
	}
	b.WriteString("\n" + pluginCommandsParagraph("routes", "RoutePath"))
	return b.String(), nil
}

// httpSide is the highest side-effect class the command reaches through its
// flags exposed on HTTP.
func httpSide(cmd *cmdregistry.Command) string {
	best := cmd.SideEffect
	for _, f := range invoke.ExposedFlags(cmd, invoke.TransportHTTP) {
		if f.SideEffect != nil && canon.Rank(*f.SideEffect) > canon.Rank(best) {
			best = *f.SideEffect
		}
	}
	return best
}

// httpStreaming says whether the command streams always, only with a flag, or never.
func httpStreaming(cmd *cmdregistry.Command) string {
	if cmd.Output == canon.OutputStream {
		return "yes"
	}
	var flags []string
	for _, f := range cmd.Flags {
		if f.Output != nil && *f.Output == canon.OutputStream {
			flags = append(flags, "`"+f.Name+"`")
		}
	}
	if len(flags) > 0 {
		return "with " + strings.Join(flags, ", ")
	}
	return "no"
}
