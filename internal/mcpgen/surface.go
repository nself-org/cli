package mcpgen

// surface.go — the live tool set of one `nself mcp` process (P7-SHIP-02).
//
// Purpose: the server starts with or without a project. Without one it lists
// `nself_init` and the read-only tools, so an agent that only has MCP can
// create the project. After every successful call the Surface checks whether
// the inputs of the tool set changed (a project now exists, or the installed
// plugin set changed, as after `add ci`); if so it rebuilds the registry and
// replaces the tools, which sends notifications/tools/list_changed to every
// client (ServerOptions declares tools.listChanged).
// Inputs: SurfaceOptions (server, hand tools, registry source, runner, plugin dir).
// Outputs: the server's tools; the project dir each call runs in.
// Constraints: the runner owns the confirmation Guard, so a confirmation
// issued before a refresh is still valid after it. Every call runs as a child
// process and reads only its own registry value, so a rebuild while calls are
// in flight is safe. A failed rebuild keeps the previous tools and is retried
// after the next successful call.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/server"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
)

// noProjectTools are listed, besides the read-only tools, before a project exists.
var noProjectTools = map[string]bool{"nself_init": true}

// SurfaceOptions configures a Surface.
type SurfaceOptions struct {
	Server *server.MCPServer
	// Hand registers the v1.4 hand tools on a server; nil in v1.5 mode.
	Hand func(*server.MCPServer)
	V15  bool
	// Schema returns a committed schema by its path relative to schemas/.
	Schema func(name string) ([]byte, bool)
	// Registry returns the machine registry; rebuild drops the memoised one first.
	Registry func(rebuild bool) (*cmdregistry.Registry, error)
	// Run invokes one command of reg in dir ("" is the working directory).
	Run func(ctx context.Context, reg *cmdregistry.Registry, path string, req invoke.Request, dir string) (invoke.Result, error)
	// PluginDir returns the installed plugin directory.
	PluginDir func() string
	// Project returns the project root above the working directory, or "".
	Project func() string
	// Warn receives one line per problem that does not stop the server.
	Warn func(string)
}

// Surface owns the tool set of one server.
type Surface struct {
	o    SurfaceOptions
	hand []server.ServerTool
	mu   sync.Mutex
	dir  string // the project root; "" before a project exists
	seen string // the inputs the current tools were built from
}

// NewSurface returns a Surface; Start lists the first tools.
func NewSurface(o SurfaceOptions) *Surface {
	m := &Surface{o: o}
	if o.Hand != nil {
		scratch := server.NewMCPServer(ServerName, ServerVersion)
		o.Hand(scratch)
		for _, t := range scratch.ListTools() {
			m.hand = append(m.hand, *t)
		}
		sort.Slice(m.hand, func(i, j int) bool { return m.hand[i].Tool.Name < m.hand[j].Tool.Name })
	}
	return m
}

// inputs names what the tool set is built from: the project root and every
// installed plugin entry with its manifest state.
func (m *Surface) inputs(dir string) string {
	var b strings.Builder
	b.WriteString(dir)
	plugins := m.o.PluginDir()
	entries, _ := os.ReadDir(plugins) // no plugin dir yet: no plugins
	for _, e := range entries {
		fmt.Fprintf(&b, "\x00%s", e.Name())
		for _, name := range []string{"", "plugin.json", "plugin.yaml"} {
			if info, err := os.Stat(filepath.Join(plugins, e.Name(), name)); err == nil {
				fmt.Fprintf(&b, "/%s:%d:%d", name, info.Size(), info.ModTime().UnixNano())
			}
		}
	}
	return b.String()
}

// Start lists the hand tools (v1.4), then the generated ones, and returns the
// registry they came from. On an error the hand tools stay listed.
func (m *Surface) Start() (*cmdregistry.Registry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dir = m.o.Project()
	if m.dir == "" { // an MCP-only agent must still reach init
		m.warn("no nself project here; serving init and the read-only tools until one exists")
	}
	m.seen = m.inputs(m.dir)
	m.o.Server.SetTools(visible(m.hand, m.dir)...)
	reg, err := m.o.Registry(false)
	if err != nil {
		return nil, err
	}
	return reg, m.install(reg)
}

// Invoker runs one command of reg through the runner, then refreshes the
// tools when the call succeeded.
func (m *Surface) Invoker(reg *cmdregistry.Registry) Invoker {
	return func(ctx context.Context, path string, req invoke.Request) (invoke.Result, error) {
		m.mu.Lock()
		dir := m.dir
		m.mu.Unlock()
		res, err := m.o.Run(ctx, reg, path, req, dir)
		if err == nil && res.ExitCode == 0 {
			m.refresh()
		}
		return res, err
	}
}

// refresh rebuilds the tools when their inputs changed since the last build.
func (m *Surface) refresh() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dir = m.o.Project()
	now := m.inputs(m.dir)
	if now == m.seen {
		return
	}
	reg, err := m.o.Registry(true)
	if err == nil {
		err = m.install(reg)
	}
	if err != nil {
		m.warn("the tool list was not refreshed, keeping the previous one: " + err.Error())
		return
	}
	m.seen = now
}

// install generates the tools from reg and replaces the server's set.
func (m *Surface) install(reg *cmdregistry.Registry) error {
	tools, err := Tools(Options{Registry: reg, Invoke: m.Invoker(reg), Schema: m.o.Schema, V15: m.o.V15, Warn: m.o.Warn})
	if err != nil {
		return err
	}
	m.o.Server.SetTools(visible(append(append([]server.ServerTool{}, m.hand...), tools...), m.dir)...)
	return nil
}

func (m *Surface) warn(msg string) {
	if m.o.Warn != nil {
		m.o.Warn(msg)
	}
}

// visible is every tool with a project; without one, init and the tools
// annotated read-only.
func visible(all []server.ServerTool, dir string) []server.ServerTool {
	if dir != "" {
		return all
	}
	var out []server.ServerTool
	for _, t := range all {
		ro := t.Tool.Annotations.ReadOnlyHint
		if noProjectTools[t.Tool.Name] || (ro != nil && *ro) {
			out = append(out, t)
		}
	}
	return out
}
