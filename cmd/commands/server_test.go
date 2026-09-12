package commands

// Tests exercise the `nself server` cobra wiring (flag parsing, error
// propagation, output) against a fakeServerClient injected via the
// newServerClient indirection in server_client.go — never against the real
// Hetzner Cloud API. Deeper safety-logic tests (backup gate, IP protection,
// disk-shrink detection) live in internal/server/*_test.go.

import (
	"context"
	"testing"

	"github.com/nself-org/cli/internal/server"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// fakeServerClient is a minimal in-memory server.Client for command-layer
// tests. It never performs network I/O.
type fakeServerClient struct {
	servers    map[int64]*server.Server
	nextID     int64
	primaryIPs map[int64]*server.PrimaryIP
}

func newFakeServerClient() *fakeServerClient {
	return &fakeServerClient{
		servers:    map[int64]*server.Server{},
		primaryIPs: map[int64]*server.PrimaryIP{},
		nextID:     1,
	}
}

func (f *fakeServerClient) CreateServer(_ context.Context, req server.ProvisionRequest) (*server.Server, error) {
	id := f.nextID
	f.nextID++
	s := &server.Server{ID: id, Name: req.Name, ServerType: req.ServerType, Location: req.Location, Status: "running", IPv4: "203.0.113.20", Labels: req.Labels}
	f.servers[id] = s
	return s, nil
}

func (f *fakeServerClient) ListServers(_ context.Context, _ server.ListOptions) ([]server.Server, error) {
	out := make([]server.Server, 0, len(f.servers))
	for _, s := range f.servers {
		out = append(out, *s)
	}
	return out, nil
}

func (f *fakeServerClient) GetServer(_ context.Context, id int64) (*server.Server, error) {
	s, ok := f.servers[id]
	if !ok {
		return nil, errNotFound(id)
	}
	cp := *s
	return &cp, nil
}

func (f *fakeServerClient) DeleteServer(_ context.Context, id int64) error {
	if _, ok := f.servers[id]; !ok {
		return errNotFound(id)
	}
	delete(f.servers, id)
	return nil
}

func (f *fakeServerClient) ListServerTypes(_ context.Context) ([]server.ServerType, error) {
	return []server.ServerType{{Name: "cx22", Disk: 40}, {Name: "cx41", Disk: 160}}, nil
}

func (f *fakeServerClient) ChangeServerType(_ context.Context, id int64, targetType string, _ bool) (*server.Action, error) {
	s, ok := f.servers[id]
	if !ok {
		return nil, errNotFound(id)
	}
	s.ServerType = targetType
	return &server.Action{ID: 1, Status: "success"}, nil
}

func (f *fakeServerClient) CreateSnapshot(_ context.Context, id int64, description string) (*server.Image, *server.Action, error) {
	if _, ok := f.servers[id]; !ok {
		return nil, nil, errNotFound(id)
	}
	return &server.Image{ID: 500, Status: "available", Description: description},
		&server.Action{ID: 1, Status: "success"}, nil
}

func (f *fakeServerClient) GetImage(_ context.Context, id int64) (*server.Image, error) {
	return &server.Image{ID: id, Status: "available"}, nil
}

func (f *fakeServerClient) GetAction(_ context.Context, id int64) (*server.Action, error) {
	return &server.Action{ID: id, Status: "success"}, nil
}

func (f *fakeServerClient) ListPrimaryIPs(_ context.Context, id int64) ([]server.PrimaryIP, error) {
	var out []server.PrimaryIP
	for _, ip := range f.primaryIPs {
		if ip.AssigneeID == id {
			out = append(out, *ip)
		}
	}
	return out, nil
}

func (f *fakeServerClient) SetPrimaryIPAutoDelete(_ context.Context, ipID int64, autoDelete bool) error {
	ip, ok := f.primaryIPs[ipID]
	if !ok {
		return errNotFound(ipID)
	}
	ip.AutoDelete = autoDelete
	return nil
}

func errNotFound(id int64) error {
	return &notFoundErr{id}
}

type notFoundErr struct{ id int64 }

func (e *notFoundErr) Error() string { return "not found" }

// withFakeServerClient points newServerClient at fc for the duration of the
// test, restoring the real (Hetzner-backed) factory afterward.
func withFakeServerClient(t *testing.T, fc *fakeServerClient) {
	t.Helper()
	old := newServerClient
	newServerClient = func(cmd *cobra.Command) (server.Client, error) {
		return fc, nil
	}
	t.Cleanup(func() { newServerClient = old })
}

// sliceResetter is implemented by pflag's StringArray/StringSlice values.
// resetFlags (access_test.go) sets scalar flags back to their default via
// Value.Set(f.DefValue), but StringArrayValue.Set APPENDS rather than
// replaces, so Set("[]") on an already-empty flag leaves a bogus one-element
// slice ["[]"] instead of an empty one. Replace(nil) is the real reset.
type sliceResetter interface {
	Replace([]string) error
}

// resetServerFlags restores every `nself server` subcommand's flags to their
// registered defaults, including the repeatable --ssh-key/--label flags
// which resetFlags's generic Set(DefValue) approach cannot clear correctly.
func resetServerFlags() {
	for _, c := range []*cobra.Command{serverProvisionCmd, serverListCmd, serverResizeCmd, serverDestroyCmd} {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if r, ok := f.Value.(sliceResetter); ok {
				_ = r.Replace(nil)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		})
	}
}

func TestServerCmd_Structure(t *testing.T) {
	names := map[string]bool{}
	for _, c := range serverCmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"provision", "list", "resize", "destroy"} {
		if !names[want] {
			t.Errorf("nself server missing subcommand %q", want)
		}
	}
}

func TestServerProvision_HappyPath(t *testing.T) {
	resetServerFlags()
	fc := newFakeServerClient()
	withFakeServerClient(t, fc)

	cmd := serverProvisionCmd
	_ = cmd.Flags().Set("name", "ci-runner-3")
	_ = cmd.Flags().Set("type", "cx22")
	_ = cmd.Flags().Set("location", "fsn1")
	_ = cmd.Flags().Set("image", "ubuntu-24.04")
	cmd.SetContext(context.Background())

	if err := runServerProvision(cmd, nil); err != nil {
		t.Fatalf("runServerProvision: %v", err)
	}
	if len(fc.servers) != 1 {
		t.Fatalf("expected 1 server created, got %d", len(fc.servers))
	}
}

func TestServerProvision_MissingName(t *testing.T) {
	resetServerFlags()
	fc := newFakeServerClient()
	withFakeServerClient(t, fc)

	cmd := serverProvisionCmd
	_ = cmd.Flags().Set("type", "cx22")
	_ = cmd.Flags().Set("location", "fsn1")
	_ = cmd.Flags().Set("image", "ubuntu-24.04")
	cmd.SetContext(context.Background())

	if err := runServerProvision(cmd, nil); err == nil {
		t.Fatal("runServerProvision: want error when --name is missing")
	}
}

func TestServerProvision_BadLabel(t *testing.T) {
	resetServerFlags()
	fc := newFakeServerClient()
	withFakeServerClient(t, fc)

	cmd := serverProvisionCmd
	_ = cmd.Flags().Set("name", "x")
	_ = cmd.Flags().Set("type", "cx22")
	_ = cmd.Flags().Set("location", "fsn1")
	_ = cmd.Flags().Set("image", "ubuntu-24.04")
	_ = cmd.Flags().Set("label", "not-a-kv-pair")
	cmd.SetContext(context.Background())

	if err := runServerProvision(cmd, nil); err == nil {
		t.Fatal("runServerProvision: want error for malformed --label")
	}
}

func TestServerList_Empty(t *testing.T) {
	resetServerFlags()
	fc := newFakeServerClient()
	withFakeServerClient(t, fc)

	cmd := serverListCmd
	cmd.SetContext(context.Background())
	if err := runServerList(cmd, nil); err != nil {
		t.Fatalf("runServerList: %v", err)
	}
}

func TestServerResize_DiskShrink_Refused(t *testing.T) {
	resetServerFlags()
	fc := newFakeServerClient()
	fc.servers[1] = &server.Server{ID: 1, Name: "web-1", ServerType: "cx41"}
	withFakeServerClient(t, fc)

	cmd := serverResizeCmd
	_ = cmd.Flags().Set("id", "1")
	_ = cmd.Flags().Set("type", "cx22")
	cmd.SetContext(context.Background())

	if err := runServerResize(cmd, nil); err == nil {
		t.Fatal("runServerResize: want error refusing the disk shrink")
	}
}

func TestServerDestroy_NoBackupFlag_Refused(t *testing.T) {
	resetServerFlags()
	fc := newFakeServerClient()
	fc.servers[1] = &server.Server{ID: 1, Name: "web-1"}
	withFakeServerClient(t, fc)

	cmd := serverDestroyCmd
	_ = cmd.Flags().Set("id", "1")
	cmd.SetContext(context.Background())

	if err := runServerDestroy(cmd, nil); err == nil {
		t.Fatal("runServerDestroy: want error when neither --snapshot nor --force-no-backup is passed")
	}
	if _, ok := fc.servers[1]; !ok {
		t.Fatal("server was deleted despite missing the backup gate")
	}
}

func TestServerDestroy_ForceNoBackup_Succeeds(t *testing.T) {
	resetServerFlags()
	fc := newFakeServerClient()
	fc.servers[1] = &server.Server{ID: 1, Name: "web-1"}
	fc.primaryIPs[10] = &server.PrimaryIP{ID: 10, IP: "203.0.113.10", AssigneeID: 1, AutoDelete: true}
	withFakeServerClient(t, fc)

	cmd := serverDestroyCmd
	_ = cmd.Flags().Set("id", "1")
	_ = cmd.Flags().Set("force-no-backup", "true")
	cmd.SetContext(context.Background())

	if err := runServerDestroy(cmd, nil); err != nil {
		t.Fatalf("runServerDestroy: %v", err)
	}
	if _, ok := fc.servers[1]; ok {
		t.Fatal("server still exists after a successful destroy")
	}
	if fc.primaryIPs[10].AutoDelete {
		t.Error("primary IP auto_delete must be false by the time destroy completes without --release-ip")
	}
}
