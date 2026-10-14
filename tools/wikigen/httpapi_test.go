package main

// Tests for HTTP-API-Reference.md: the contract bytes, one row per OpenAPI
// operation, and the side-effect and streaming columns on fixtures.

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
)

func TestHTTPAPIPageMatchesOpenAPI(t *testing.T) {
	page, err := renderHTTPAPI(hermeticRegistry(t), repoTestSources)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := os.ReadFile(contractFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(page, generatedNotice+"\n"+string(contract)) {
		t.Fatal("page does not start with the generated header and the exact bytes of contract.md")
	}
	spec, err := os.ReadFile(openapiFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatal(err)
	}
	var want []string
	for path, methods := range doc.Paths {
		for method, op := range methods {
			if op.OperationID != "" {
				want = append(want, strings.ToUpper(method)+" `"+path+"`")
			}
		}
	}
	var got []string
	for _, l := range strings.Split(page, "\n") {
		if cells := strings.Split(l, " | "); len(cells) == 5 && (strings.HasPrefix(l, "| GET | ") || strings.HasPrefix(l, "| POST | ")) {
			got = append(got, strings.TrimPrefix(cells[0], "| ")+" "+cells[1])
		}
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("page has %d operation rows, openapi has %d\npage:    %v\nopenapi: %v", len(got), len(want), got, want)
	}
	if last := lastLine(page); !strings.HasPrefix(last, "**Plugin commands.**") || !strings.Contains(last, "RoutePath") {
		t.Errorf("last line is %q", last)
	}
}

const fixtureSpec = `{"paths":{
 "/v1/commands/logs":{"post":{"operationId":"nself_logs","summary":"Show logs"}},
 "/v1/commands/destroy":{"post":{"operationId":"nself_destroy","summary":"Remove | it"}},
 "/openapi.json":{"get":{"operationId":"openapi","summary":"Read this OpenAPI document"}}}}`

func TestHTTPPageFixture(t *testing.T) {
	stream := "stream"
	cmds := map[string]*cmdregistry.Command{
		"nself_logs":    {SideEffect: "read", Output: "document", Flags: []cmdregistry.Flag{{Name: "follow", Output: &stream}}},
		"nself_destroy": {SideEffect: "destructive", Output: "document"},
	}
	resolve := func(id string) *cmdregistry.Command { return cmds[id] }
	page, err := renderHTTPPage("# Title\n\nbody\n", []byte(fixtureSpec), resolve)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(page, generatedNotice+"\n# Title\n\nbody\n\n## Operations") {
		t.Errorf("page frame: %q", page[:80])
	}
	want := []string{
		"| GET | `/openapi.json` | Read this OpenAPI document | `read` | no |",
		"| POST | `/v1/commands/destroy` | Remove \\| it | `destructive` | no |",
		"| POST | `/v1/commands/logs` | Show logs | `read` | with `follow` |",
	}
	at := -1
	for _, w := range want {
		i := strings.Index(page, w)
		if i <= at {
			t.Errorf("row %q missing or out of order", w)
		}
		at = i
	}
	// A contract without a trailing newline still gets a clean break.
	if p, err := renderHTTPPage("# Title", []byte(fixtureSpec), resolve); err != nil || !strings.HasPrefix(p, generatedNotice+"\n# Title\n\n## Operations") {
		t.Errorf("no-newline contract: %v", err)
	}
	// Drift: a changed operation summary changes the page; an unknown operation fails.
	changed := strings.Replace(fixtureSpec, "Show logs", "Tail logs", 1)
	if p, _ := renderHTTPPage("# Title\n\nbody\n", []byte(changed), resolve); p == page {
		t.Error("a changed summary did not change the page")
	}
	delete(cmds, "nself_logs")
	if _, err := renderHTTPPage("x\n", []byte(fixtureSpec), resolve); err == nil {
		t.Error("an operation without a registry command must fail")
	}
}
