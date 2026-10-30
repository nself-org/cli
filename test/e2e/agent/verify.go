//go:build e2eagent

package agent

// Purpose: verify an agent E2E transcript against SHIP Epic D7: every agent
// record is an MCP request to the nself server, every tools/call result is a
// v1 envelope, the D7 steps all happen in order, and the two legs hold.
// Inputs: transcript Records (ReadTranscript or Writer.Records).
// Outputs: nil, or an error listing every violation found.
// Constraints: pure function of the records; no retries, no network.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/nself-org/cli/schemas"
)

// call is an agent tools/call paired with the server result that follows it.
type call struct {
	seq    int
	tool   string
	args   map[string]any
	result *Record
}

func (c call) flags() map[string]any { m, _ := c.args["flags"].(map[string]any); return m }

func (c call) arg0() string {
	if list, ok := c.args["args"].([]any); ok && len(list) > 0 {
		s, _ := list[0].(string)
		return s
	}
	return ""
}

func (c call) confirmed() bool { _, ok := c.args["confirm"]; return ok }

func (c call) data() map[string]any {
	if c.result == nil {
		return nil
	}
	var doc struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(c.result.Envelope, &doc)
	return doc.Data
}

func (c call) ok() bool { return c.result != nil && !c.result.IsError }

func (c call) confirmation() bool {
	d := c.data()
	return c.ok() && d != nil && d["confirmation_required"] == true
}

// milestone is one D7 step; first is the seq of its first matching call.
type milestone struct {
	name  string
	match func(call) bool
}

func toolIs(name string) func(call) bool { return func(c call) bool { return c.tool == name } }

// Milestones are the D7 flow steps in their required order.
var Milestones = []milestone{
	{"init", func(c call) bool { return toolIs("nself_init")(c) && c.ok() }},
	{"build plan", func(c call) bool { return toolIs("nself_build")(c) && !c.confirmed() && c.confirmation() }},
	{"build confirm", func(c call) bool { return toolIs("nself_build")(c) && c.confirmed() && c.ok() }},
	{"start", func(c call) bool { return toolIs("nself_start")(c) && c.ok() }},
	{"db migrate up", func(c call) bool { return toolIs("nself_db_migrate_up")(c) && c.ok() }},
	{"add ci", func(c call) bool { return toolIs("nself_add")(c) && c.arg0() == "ci" && c.confirmed() && c.ok() }},
	{"ci detach", func(c call) bool {
		return toolIs("nself_ci")(c) && c.flags()["detach"] == true && c.confirmed() && c.ok()
	}},
	{"ci runs wait", func(c call) bool { return toolIs("nself_ci_runs_wait")(c) && c.ok() && terminal(c.data()) }},
	{"ci evidence", func(c call) bool { return toolIs("nself_ci_evidence")(c) && c.ok() && c.data()["result"] == "pass" }},
	{"deploy qa", func(c call) bool { return toolIs("nself_deploy")(c) && c.arg0() == "qa" && c.confirmed() && c.ok() }},
	{"promote dry-run", func(c call) bool { return isPromote(c) && c.flags()["dry-run"] == true && c.ok() }},
	{"promote", func(c call) bool { return isPromote(c) && c.flags()["dry-run"] != true && c.confirmed() && c.ok() }},
}

func isPromote(c call) bool { return c.tool == "nself_deploy_promote" }

func terminal(d map[string]any) bool {
	s, _ := d["state"].(string)
	return s == "passed" || s == "failed" || s == "cancelled"
}

// Verify returns every D7 violation in recs, joined; nil when the transcript holds.
func Verify(recs []Record) error {
	var errs []error
	calls := pair(recs)
	errs = append(errs, checkMCPOnly(recs)...)
	errs = append(errs, checkEnvelopes(recs)...)
	first := map[string]int{}
	for _, m := range Milestones {
		for _, c := range calls {
			if m.match(c) {
				first[m.name] = c.seq
				break
			}
		}
	}
	prev, prevName := 0, ""
	for _, m := range Milestones {
		seq, ok := first[m.name]
		if !ok {
			errs = append(errs, fmt.Errorf("missing step %q", m.name))
			continue
		}
		if seq < prev {
			errs = append(errs, fmt.Errorf("step %q (seq %d) runs before %q (seq %d)", m.name, seq, prevName, prev))
		}
		prev, prevName = seq, m.name
	}
	errs = append(errs, checkPromoteOrder(calls)...)
	errs = append(errs, checkEvidenceOrder(calls)...)
	errs = append(errs, checkCIDiscovery(recs, first)...)
	errs = append(errs, checkRecoveryLeg(recs, calls)...)
	errs = append(errs, checkGateLeg(recs, calls)...)
	return errors.Join(errs...)
}

// pair links each agent tools/call with the next server result for its tool.
func pair(recs []Record) []call {
	var out []call
	for i, r := range recs {
		if r.Role != RoleAgent || r.Kind != KindToolsCall {
			continue
		}
		c := call{seq: r.Seq, tool: r.Tool, args: r.Arguments}
		for j := i + 1; j < len(recs); j++ {
			if recs[j].Role == RoleServer && recs[j].Kind == KindResult && recs[j].Tool == r.Tool {
				c.result = &recs[j]
				break
			}
		}
		out = append(out, c)
	}
	return out
}

func checkMCPOnly(recs []Record) []error {
	var errs []error
	for _, r := range recs {
		if r.Role != RoleAgent {
			continue
		}
		if r.Kind != KindToolsCall && r.Kind != KindToolsList {
			errs = append(errs, fmt.Errorf("seq %d: non-MCP tool use: agent record kind %q", r.Seq, r.Kind))
		} else if r.Kind == KindToolsCall && !strings.HasPrefix(r.Tool, "nself_") {
			errs = append(errs, fmt.Errorf("seq %d: non-MCP tool use: %q is not a tool of the nself server", r.Seq, r.Tool))
		}
	}
	return errs
}

var envelopeSchema = func() *jsonschema.Schema {
	c := jsonschema.NewCompiler()
	for _, name := range schemas.Names() {
		b, _ := schemas.Lookup(name)
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			continue
		}
		if m, ok := doc.(map[string]any); ok {
			if id, ok := m["$id"].(string); ok {
				_ = c.AddResource(id, doc)
			}
		}
	}
	return c.MustCompile("urn:nself:cli:schema:envelope:v1")
}()

func checkEnvelopes(recs []Record) []error {
	var errs []error
	for i, r := range recs {
		if r.Role != RoleServer || r.Kind != KindResult || i == 0 || !callBefore(recs, i) {
			continue
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(r.Envelope))
		if err == nil {
			err = envelopeSchema.Validate(doc)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("seq %d: %s envelope fails the v1 schema: %v", r.Seq, r.Tool, err))
		}
	}
	return errs
}

// callBefore reports whether the result at i answers a tools/call (tools/list
// results carry a tool-name list, not an envelope).
func callBefore(recs []Record, i int) bool {
	for j := i - 1; j >= 0; j-- {
		if recs[j].Role == RoleAgent {
			return recs[j].Kind == KindToolsCall
		}
	}
	return false
}

func checkPromoteOrder(calls []call) []error {
	for _, c := range calls {
		if !isPromote(c) {
			continue
		}
		if c.flags()["dry-run"] == true {
			return nil
		}
		if c.confirmed() {
			return []error{fmt.Errorf("seq %d: promote apply before its dry-run", c.seq)}
		}
	}
	return nil
}

func checkEvidenceOrder(calls []call) []error {
	for _, c := range calls {
		switch c.tool {
		case "nself_ci_runs_wait":
			return nil
		case "nself_ci_evidence":
			return []error{fmt.Errorf("seq %d: ci evidence before ci runs wait", c.seq)}
		}
	}
	return nil
}

// checkCIDiscovery wants a tools/list result after `add ci` naming the three ci tools.
func checkCIDiscovery(recs []Record, first map[string]int) []error {
	after, ok := first["add ci"]
	if !ok {
		return nil
	}
	for _, r := range recs {
		if r.Seq <= after || r.Role != RoleServer || r.Kind != KindResult || r.Tool != "" {
			continue
		}
		var list struct {
			Tools []string `json:"tools"`
		}
		if json.Unmarshal(r.Envelope, &list) == nil && hasAll(list.Tools, "nself_ci", "nself_ci_runs_wait", "nself_ci_evidence") {
			return nil
		}
	}
	return []error{errors.New("no tools/list after add ci shows nself_ci, nself_ci_runs_wait and nself_ci_evidence")}
}

func hasAll(have []string, want ...string) bool {
	set := map[string]bool{}
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}
