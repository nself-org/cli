// Package agentsetup renders what an AI agent needs to work with an nSelf
// project: the stdio MCP client config (Claude Code, Cursor, Codex, generic)
// and the project AGENTS.md (P7-SURF-05, cap:cli.agent-setup).
//
// Purpose:     one place that knows the client config shapes and the AGENTS.md
//
//	template, shared by `nself mcp setup` and `nself init`.
//
// Inputs:      a client name, the existing file bytes, the registry verbs.
// Outputs:     config bytes, merged config bytes, AGENTS.md text.
// Constraints: render and merge are pure over bytes; files.go does the writes. No
//
//	token or secret is ever written: stdio needs none.
package agentsetup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Client names accepted by `nself mcp setup --client`.
const (
	ClaudeCode = "claude-code"
	Cursor     = "cursor"
	Codex      = "codex"
	Generic    = "generic"
)

// Clients lists every client name in help order.
var Clients = []string{ClaudeCode, Cursor, Codex, Generic}

// serverName is the key of the nSelf entry inside mcpServers.
const serverName = "nself"

// entryJSON is the stdio server entry every client gets.
const entryJSON = `{"command":"nself","args":["mcp"]}`

// Valid reports whether name is a known client.
func Valid(name string) bool {
	for _, c := range Clients {
		if c == name {
			return true
		}
	}
	return false
}

// Path returns the project-relative file a client reads its config from, or ""
// when the client is print-only (codex and generic).
func Path(client string) string {
	switch client {
	case ClaudeCode:
		return ".mcp.json"
	case Cursor:
		return ".cursor/mcp.json"
	}
	return ""
}

// Config returns the compact config document a client needs.
func Config(client string) string {
	if client == Codex {
		return "[mcp_servers.nself]\ncommand = \"nself\"\nargs = [\"mcp\"]\n"
	}
	return `{"mcpServers":{"` + serverName + `":` + entryJSON + `}}`
}

// field is one member of a JSON object, in source order.
type field struct {
	key string
	val json.RawMessage
}

// readObject decodes a JSON object keeping member order. A repeated key keeps
// both members, so a merge never silently drops one.
func readObject(raw []byte) ([]field, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("not a JSON object")
	}
	var out []field
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		out = append(out, field{kt.(string), v})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("trailing data after the object")
	}
	return out, nil
}

func writeObject(fs []field) []byte {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range fs {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(f.key)
		b.Write(k)
		b.WriteByte(':')
		b.Write(f.val)
	}
	b.WriteByte('}')
	return b.Bytes()
}

func find(fs []field, key string) int {
	for i, f := range fs {
		if f.key == key {
			return i
		}
	}
	return -1
}

// Merge adds the nself server entry to an existing client config file and
// returns the new bytes and whether they changed. Empty existing content means
// a new file. Every other key and server entry keeps its place; an nself entry
// that is already present is left alone (the user may have tuned it), so a
// second run changes nothing and returns the input unchanged, byte for byte.
func Merge(existing []byte) ([]byte, bool, error) {
	if len(bytes.TrimSpace(existing)) == 0 {
		return indent([]byte(Config(ClaudeCode))), true, nil
	}
	top, err := readObject(existing)
	if err != nil {
		return nil, false, fmt.Errorf("existing config is not a JSON object: %w", err)
	}
	i := find(top, "mcpServers")
	if i < 0 {
		top = append(top, field{"mcpServers", json.RawMessage(`{"` + serverName + `":` + entryJSON + `}`)})
		return indent(writeObject(top)), true, nil
	}
	servers, err := readObject(top[i].val)
	if err != nil {
		return nil, false, fmt.Errorf("mcpServers is not a JSON object: %w", err)
	}
	if find(servers, serverName) >= 0 {
		return existing, false, nil
	}
	servers = append(servers, field{serverName, json.RawMessage(entryJSON)})
	top[i].val = writeObject(servers)
	return indent(writeObject(top)), true, nil
}

// indent pretty-prints compact JSON with two spaces and a trailing newline.
func indent(compact []byte) []byte {
	var b bytes.Buffer
	if err := json.Indent(&b, compact, "", "  "); err != nil {
		return append(compact, '\n')
	}
	b.WriteByte('\n')
	return b.Bytes()
}

// Note is the stderr hint printed with a client config: the server reads the
// project from its working directory.
func Note(cwd string) string {
	return strings.TrimSpace(fmt.Sprintf("note: start the client in the project directory (%s); `nself mcp` finds the project from its working directory", cwd))
}
