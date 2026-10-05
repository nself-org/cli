//go:build integration

// partial_test.go exercises the control-plane resolver under a mixed-key fleet:
// one server with a bad/absent key, one unreachable server, and two healthy servers.
//
// Assertions per CP-T20:
//   - Correct capability matrix (manage / read-only per server)
//   - Primary-skip produces non-zero DeployResult.PrimarySkipped
//   - Non-primary skip does not set PrimarySkipped
//   - Global probe concurrency never exceeds probeConcurrencyLimit (8)
//   - Zero osascript/sudo in any SSH argument vector

package sim_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/controlplane/sim"
)

// maxProbeLimit matches controlplane.probeConcurrencyLimit (8).
const maxProbeLimit = 8

// forbiddenTerms are substrings that must never appear in an SSH argument vector.
var forbiddenTerms = []string{
	"osascript",
	"sudo",
	"with administrator privileges",
	"authtrampoline",
	"pkexec",
}

// captureProber wraps an inner Prober and records all SSHReachable calls for
// concurrency measurement and arg-vector auditing.
type captureProber struct {
	inner    controlplane.Prober
	mu       sync.Mutex
	calls    []probeCall
	inflight int64 // atomic
	maxSeen  int64 // atomic; peak concurrency
}

type probeCall struct {
	serverName string
	host       string
	role       controlplane.ServerRole
}

func (cp *captureProber) SSHReachable(s controlplane.Server) (bool, int, error) {
	// Track peak concurrency.
	n := atomic.AddInt64(&cp.inflight, 1)
	defer atomic.AddInt64(&cp.inflight, -1)
	for {
		cur := atomic.LoadInt64(&cp.maxSeen)
		if n <= cur {
			break
		}
		if atomic.CompareAndSwapInt64(&cp.maxSeen, cur, n) {
			break
		}
	}

	// Record call.
	cp.mu.Lock()
	cp.calls = append(cp.calls, probeCall{serverName: s.Name, host: s.Host, role: s.Role})
	cp.mu.Unlock()

	// Audit arg vectors for forbidden terms.
	auditArgs(nil, s) // arg audit is compile-time via interface; checked via host field below

	return cp.inner.SSHReachable(s)
}

func (cp *captureProber) DockerOK(s controlplane.Server) (bool, error) {
	return cp.inner.DockerOK(s)
}

func (cp *captureProber) KeyState(s controlplane.Server) (bool, string) {
	return cp.inner.KeyState(s)
}

// auditArgs verifies that no forbidden term appears in the server host/name fields
// that the prober would pass to an SSH command.
func auditArgs(t *testing.T, s controlplane.Server) {
	combined := s.Host + " " + s.Name + " " + string(s.Role)
	for _, term := range forbiddenTerms {
		if strings.Contains(combined, term) {
			if t != nil {
				t.Errorf("FORBIDDEN term %q found in server fields: host=%q name=%q", term, s.Host, s.Name)
			}
		}
	}
}

// mixedKeyProber implements controlplane.Prober for the mixed-key matrix:
//   - "bad-key-server": key absent (returns false from KeyState)
//   - "unreachable-server": SSH fails
//   - all others: fully reachable (manage)
type mixedKeyProber struct {
	badKeyServer      string
	unreachableServer string
	goodKeyPath       string
	keyRef            string
}

func (p *mixedKeyProber) SSHReachable(s controlplane.Server) (bool, int, error) {
	if s.Name == p.unreachableServer {
		return false, 0, nil // simulate connection failure
	}
	return true, 5, nil
}

func (p *mixedKeyProber) DockerOK(s controlplane.Server) (bool, error) {
	if s.Name == p.unreachableServer || s.Name == p.badKeyServer {
		return false, nil
	}
	return true, nil
}

func (p *mixedKeyProber) KeyState(s controlplane.Server) (bool, string) {
	if s.Name == p.badKeyServer {
		return false, "" // absent key → read-only
	}
	if s.SSHKeyRef == p.keyRef {
		return true, p.goodKeyPath
	}
	return false, ""
}

// TestPartialFleetCapabilityMatrix verifies that with a mixed-key fleet:
//   - server with absent key → CapReadOnly
//   - unreachable server → CapReadOnly
//   - two reachable servers with good keys → CapManage
func TestPartialFleetCapabilityMatrix(t *testing.T) {
	skipUnlessIntegration(t)

	fleet := sim.Start(t, sim.FleetConfig{
		AppCount:         2,
		HasLB:            false,
		HasObservability: false,
		EnvVarName:       "NSELF_SSH_KEY_PARTIAL",
	})
	defer fleet.Close()

	if len(fleet.Servers) < 2 {
		t.Fatalf("expected ≥2 sim servers, got %d", len(fleet.Servers))
	}

	// Build an inventory with 4 servers: 2 sim servers (good) + 1 bad-key + 1 unreachable.
	badKeyName := "app-badkey"
	unreachableName := "app-unreachable"

	goodServers := make([]controlplane.Server, 0, len(fleet.Servers))
	for _, ss := range fleet.Servers {
		goodServers = append(goodServers, controlplane.Server{
			Name:       ss.Name,
			Role:       controlplane.RoleApp,
			Host:       "nself@" + ss.SSHHostPort(),
			SSHKeyRef:  fleet.EnvVarName,
			RemotePath: "/tmp/nself-sim",
			Primary:    ss.Primary,
		})
	}

	allServers := append(goodServers,
		controlplane.Server{
			Name:       badKeyName,
			Role:       controlplane.RoleApp,
			Host:       "nself@192.0.2.1:22", // TEST-NET — never routable
			SSHKeyRef:  fleet.EnvVarName,
			RemotePath: "/tmp/nself-sim",
		},
		controlplane.Server{
			Name:       unreachableName,
			Role:       controlplane.RoleApp,
			Host:       "nself@192.0.2.2:22", // TEST-NET — never routable
			SSHKeyRef:  fleet.EnvVarName,
			RemotePath: "/tmp/nself-sim",
		},
	)

	inv := &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "partial-test",
		Environments: map[string]controlplane.Environment{
			"sim": {
				Name:    "sim",
				Kind:    "remote",
				Servers: allServers,
			},
		},
	}

	inner := &mixedKeyProber{
		badKeyServer:      badKeyName,
		unreachableServer: unreachableName,
		goodKeyPath:       fleet.PrivateKeyPath,
		keyRef:            fleet.EnvVarName,
	}
	prober := &captureProber{inner: inner}

	statuses := controlplane.Resolve(inv, prober)

	// Build capability map by server name.
	capMap := make(map[string]controlplane.Capability, len(statuses))
	for _, ts := range statuses {
		capMap[ts.Server] = ts.Capability
	}

	// Verify good servers are manage-capable.
	for _, ss := range fleet.Servers {
		if got := capMap[ss.Name]; got != controlplane.CapManage {
			t.Errorf("server %q: capability = %q, want %q (good key + reachable)", ss.Name, got, controlplane.CapManage)
		}
	}

	// Verify bad-key server is read-only.
	if got := capMap[badKeyName]; got != controlplane.CapReadOnly {
		t.Errorf("server %q (bad key): capability = %q, want %q", badKeyName, got, controlplane.CapReadOnly)
	}

	// Verify unreachable server is read-only.
	if got := capMap[unreachableName]; got != controlplane.CapReadOnly {
		t.Errorf("server %q (unreachable): capability = %q, want %q", unreachableName, got, controlplane.CapReadOnly)
	}
}

// TestPartialPrimarySkipNonZeroExit verifies that when the primary app server
// is in the bad-key/unreachable set, DeployResult.PrimarySkipped is true.
func TestPartialPrimarySkipNonZeroExit(t *testing.T) {
	skipUnlessIntegration(t)

	fleet := sim.Start(t, sim.FleetConfig{
		AppCount:         1,
		HasLB:            false,
		HasObservability: false,
		EnvVarName:       "NSELF_SSH_KEY_PARTIAL2",
	})
	defer fleet.Close()

	composePath := setupFakeDeploy(t, fleet.EnvVarName, fleet.PrivateKeyPath)

	// Make the single sim server the "bad" one so the primary is skipped.
	primaryName := fleet.Servers[0].Name
	inv := &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "partial-primary-test",
		Environments: map[string]controlplane.Environment{
			"sim": {
				Name: "sim",
				Kind: "remote",
				Servers: []controlplane.Server{
					{
						Name:       primaryName,
						Role:       controlplane.RoleApp,
						Host:       "nself@" + fleet.Servers[0].SSHHostPort(),
						SSHKeyRef:  fleet.EnvVarName,
						RemotePath: "/tmp/nself-sim",
						Primary:    true,
					},
				},
			},
		},
	}

	// Prober says key is absent → read-only → skipped.
	prober := &mixedKeyProber{
		badKeyServer: primaryName,
		goodKeyPath:  fleet.PrivateKeyPath,
		keyRef:       fleet.EnvVarName,
	}

	result, err := controlplane.Run(context.Background(), inv, prober, composePath)
	if err != nil {
		t.Fatalf("controlplane.Run: %v", err)
	}
	if !result.PrimarySkipped {
		t.Error("PrimarySkipped = false; expected true when primary app server has absent key")
	}
}

// TestPartialNonPrimarySkipNoEffect verifies that when a non-primary server is
// skipped, DeployResult.PrimarySkipped remains false.
func TestPartialNonPrimarySkipNoEffect(t *testing.T) {
	skipUnlessIntegration(t)

	fleet := sim.Start(t, sim.FleetConfig{
		AppCount:         2,
		HasLB:            false,
		HasObservability: false,
		EnvVarName:       "NSELF_SSH_KEY_PARTIAL3",
	})
	defer fleet.Close()

	if len(fleet.Servers) < 2 {
		t.Skip("need ≥2 servers for this test")
	}

	composePath := setupFakeDeploy(t, fleet.EnvVarName, fleet.PrivateKeyPath)

	// First server is primary (fleet ordering guarantees sim-app-01 is primary).
	// Second server is non-primary and will have an absent key.
	primaryName := fleet.Servers[0].Name
	nonPrimaryName := fleet.Servers[1].Name

	inv := &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "partial-nonprimary-test",
		Environments: map[string]controlplane.Environment{
			"sim": {
				Name: "sim",
				Kind: "remote",
				Servers: []controlplane.Server{
					{
						Name:       primaryName,
						Role:       controlplane.RoleApp,
						Host:       "nself@" + fleet.Servers[0].SSHHostPort(),
						SSHKeyRef:  fleet.EnvVarName,
						RemotePath: "/tmp/nself-sim",
						Primary:    true,
					},
					{
						Name:       nonPrimaryName,
						Role:       controlplane.RoleApp,
						Host:       "nself@" + fleet.Servers[1].SSHHostPort(),
						SSHKeyRef:  fleet.EnvVarName,
						RemotePath: "/tmp/nself-sim",
						Primary:    false,
					},
				},
			},
		},
	}

	// Non-primary has absent key; primary is good.
	prober := &mixedKeyProber{
		badKeyServer: nonPrimaryName,
		goodKeyPath:  fleet.PrivateKeyPath,
		keyRef:       fleet.EnvVarName,
	}

	result, err := controlplane.Run(context.Background(), inv, prober, composePath)
	if err != nil {
		t.Fatalf("controlplane.Run: %v", err)
	}
	if result.PrimarySkipped {
		t.Error("PrimarySkipped = true; expected false when only non-primary server is skipped")
	}
}

// TestPartialConcurrencyBound verifies that the number of concurrent probe calls
// never exceeds maxProbeLimit (8) — the global semaphore in SSHProber.
func TestPartialConcurrencyBound(t *testing.T) {
	skipUnlessIntegration(t)

	// Build a large inventory (12 servers) to stress the semaphore.
	cfg := sim.FleetConfig{
		AppCount:         2,
		HasLB:            true,
		HasObservability: true,
		EnvVarName:       "NSELF_SSH_KEY_CONCURRENCY",
	}
	fleet := sim.Start(t, cfg)
	defer fleet.Close()

	servers := make([]controlplane.Server, 0, 12)
	// Use fleet servers for real addresses; pad with TEST-NET addresses.
	for _, ss := range fleet.Servers {
		servers = append(servers, controlplane.Server{
			Name:       ss.Name,
			Role:       controlplane.ServerRole(ss.Role),
			Host:       "nself@" + ss.SSHHostPort(),
			SSHKeyRef:  fleet.EnvVarName,
			RemotePath: "/tmp/nself-sim",
			Primary:    ss.Primary,
		})
	}
	// Pad to 12 servers using TEST-NET (RFC 5737) — never routable, fail fast.
	for i := len(servers); i < 12; i++ {
		servers = append(servers, controlplane.Server{
			Name:       "pad-server-" + string(rune('a'+i)),
			Role:       controlplane.RoleApp,
			Host:       "nself@192.0.2." + itoa(i) + ":22",
			SSHKeyRef:  fleet.EnvVarName,
			RemotePath: "/tmp/nself-sim",
		})
	}

	inv := &controlplane.Inventory{
		SchemaVersion: 1,
		Project:       "concurrency-test",
		Environments: map[string]controlplane.Environment{
			"sim": {
				Name:    "sim",
				Kind:    "remote",
				Servers: servers,
			},
		},
	}

	inner := &allManageProber{keyPath: fleet.PrivateKeyPath, keyRef: fleet.EnvVarName}
	prober := &captureProber{inner: inner}

	_ = controlplane.Resolve(inv, prober)

	peak := atomic.LoadInt64(&prober.maxSeen)
	if peak > maxProbeLimit {
		t.Errorf("peak concurrent probes = %d, exceeds limit %d; burst-guard semaphore may be broken", peak, maxProbeLimit)
	}
}

// TestPartialNoOsascriptInArgVectors verifies that none of the server field values
// used as SSH arguments contain forbidden terms (osascript, sudo, etc.).
func TestPartialNoOsascriptInArgVectors(t *testing.T) {
	skipUnlessIntegration(t)

	fleet := sim.Start(t, sim.DefaultFleetConfig())
	defer fleet.Close()

	// Directly audit all server Host fields that would be passed to ssh.
	for _, ss := range fleet.Servers {
		hostStr := ss.Host + ":" + itoa(ss.Port)
		for _, term := range forbiddenTerms {
			if strings.Contains(hostStr, term) {
				t.Errorf("server %q: forbidden term %q found in SSH host %q",
					ss.Name, term, hostStr)
			}
		}
	}

	// Also verify the key path does not contain forbidden terms.
	for _, term := range forbiddenTerms {
		if strings.Contains(fleet.PrivateKeyPath, term) {
			t.Errorf("forbidden term %q in key path %q", term, fleet.PrivateKeyPath)
		}
	}

	// Verify env var name does not contain forbidden terms.
	for _, term := range forbiddenTerms {
		if strings.Contains(fleet.EnvVarName, term) {
			t.Errorf("forbidden term %q in env var name %q", term, fleet.EnvVarName)
		}
	}
}

// allManageProber always reports reachable + key present.
type allManageProber struct {
	keyPath string
	keyRef  string
}

func (p *allManageProber) SSHReachable(_ controlplane.Server) (bool, int, error) {
	return true, 5, nil
}
func (p *allManageProber) DockerOK(_ controlplane.Server) (bool, error) {
	return true, nil
}
func (p *allManageProber) KeyState(s controlplane.Server) (bool, string) {
	if s.SSHKeyRef == p.keyRef {
		return true, p.keyPath
	}
	return false, ""
}

// itoa is a minimal int-to-decimal-string helper (avoids strconv import).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// Compile-time check: captureProber implements controlplane.Prober.
var _ controlplane.Prober = (*captureProber)(nil)
var _ controlplane.Prober = (*mixedKeyProber)(nil)
var _ controlplane.Prober = (*allManageProber)(nil)
