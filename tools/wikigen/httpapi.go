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

// openAPIMethods are the keys of a path item that hold an operation; the
// others (summary, description, servers, parameters, $ref, x-*) are metadata.
var openAPIMethods = map[string]bool{"get": true, "put": true, "post": true, "delete": true, "options": true, "head": true, "patch": true, "trace": true}

// openAPIOperations lists the operations of an OpenAPI document. An operation
// with no operationId, or one that repeats an id, is an error.
func openAPIOperations(spec []byte) ([]httpOp, error) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		return nil, fmt.Errorf("openapi: %w", err)
	}
	var ops []httpOp
	seen := map[string]string{}
	for path, item := range doc.Paths {
		for method, raw := range item {
			if !openAPIMethods[method] {
				continue
			}
			var op struct {
				OperationID string `json:"operationId"`
				Summary     string `json:"summary"`
			}
			if err := json.Unmarshal(raw, &op); err != nil {
				return nil, fmt.Errorf("openapi: %s %s: %w", strings.ToUpper(method), path, err)
			}
			where := strings.ToUpper(method) + " " + path
			switch prev, dup := seen[op.OperationID]; {
			case op.OperationID == "":
				return nil, fmt.Errorf("openapi: %s has no operationId", where)
			case dup:
				return nil, fmt.Errorf("openapi: operationId %s is used by %s and %s", op.OperationID, prev, where)
			}
			seen[op.OperationID] = where
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

// httpSide is the highest side-effect class the command reaches through the
// flags a request can carry on HTTP, NDJSON streams included.
func httpSide(cmd *cmdregistry.Command) string {
	best := cmd.SideEffect
	for _, f := range invoke.ExposedFlags(cmd, invoke.TransportHTTPStream) {
		if f.SideEffect != nil && canon.Rank(*f.SideEffect) > canon.Rank(best) {
			best = *f.SideEffect
		}
	}
	return best
}

// httpStreaming says whether the command streams always, only with a flag, or
// never. A stream is advertised only where Exposure serves it on the NDJSON
// transport: a flag that is not exposed, or that raises the side effect above
// read, does not count.
func httpStreaming(cmd *cmdregistry.Command) string {
	if cmd.Output == canon.OutputStream {
		if ok, _ := invoke.Exposure(cmd, invoke.TransportHTTPStream, nil); ok {
			return "yes"
		}
		return "no"
	}
	var flags []string
	for _, f := range invoke.ExposedFlags(cmd, invoke.TransportHTTPStream) {
		if f.Output == nil || *f.Output != canon.OutputStream {
			continue
		}
		if ok, _ := invoke.Exposure(cmd, invoke.TransportHTTPStream, map[string]bool{f.Name: true}); ok {
			flags = append(flags, "`"+f.Name+"`")
		}
	}
	if len(flags) > 0 {
		return "with " + strings.Join(flags, ", ")
	}
	return "no"
}
