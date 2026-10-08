// Package controlplane implements the shared control-plane core consumed by
// both the nSelf CLI and the nSelf Admin companion. It provides declarative
// multi-server inventory management, capability resolution, topology-aware
// deployment pipelines, and load-balancer lifecycle hooks.
//
// Design invariants:
//   - Secrets are never stored inline. SSHKeyRef holds an env-var NAME whose
//     value is the path to the private key. Host holds user@host only.
//   - Capability is derived from runtime probes, never from stored authority.
//   - All file-system writes (inventory, state cache) are created mode 0600.
package controlplane

import "time"

// ServerRole classifies a server's function within an environment. The
// topology-aware pipeline uses roles to order operations correctly.
type ServerRole string

const (
	// RoleApp identifies a primary application server.
	RoleApp ServerRole = "app"
	// RoleLB identifies a load-balancer server.
	RoleLB ServerRole = "lb"
	// RoleObservability identifies a monitoring / observability server.
	RoleObservability ServerRole = "observability"
	// RoleDB identifies a dedicated database server.
	RoleDB ServerRole = "db"
	// RoleWorker identifies a background-worker server.
	RoleWorker ServerRole = "worker"
)

// Server describes a single remote (or local) host within an environment.
//
// Host must be in user@host form. No key paths or passwords are stored here;
// SSHKeyRef holds the name of an environment variable whose value is the
// absolute path to the private key file.
type Server struct {
	// Name is a human-readable identifier unique within the environment.
	Name string `yaml:"name" json:"name"`

	// Role classifies the server's function (app, lb, observability, db, worker).
	Role ServerRole `yaml:"role" json:"role"`

	// Host is the SSH target in user@host form. Empty string means local-only
	// (capability is always "manage" for local environments).
	Host string `yaml:"host,omitempty" json:"host,omitempty"`

	// SSHKeyRef is the name of an environment variable whose value holds the
	// absolute path to the SSH private key. Never the key path itself.
	SSHKeyRef string `yaml:"ssh_key_ref,omitempty" json:"ssh_key_ref,omitempty"`

	// RemotePath is the absolute path on the remote host where the nSelf
	// stack is installed (e.g. /opt/nself). Used to locate nself-lb helper.
	RemotePath string `yaml:"remote_path,omitempty" json:"remote_path,omitempty"`

	// Primary marks the authoritative app server. The pipeline returns a
	// non-zero exit code if this server is skipped due to read-only capability.
	Primary bool   `yaml:"primary,omitempty" json:"primary,omitempty"`
	Arch    string `yaml:"arch,omitempty" json:"arch,omitempty"`

	// Upstreams lists the names of app-server backends this LB routes to.
	// Applicable only when Role == RoleLB.
	Upstreams []string `yaml:"upstreams,omitempty" json:"upstreams,omitempty"`
}

// Environment groups the servers that form a single deployment target.
// Kind distinguishes local environments (no SSH needed) from remote ones.
type Environment struct {
	// Name is a well-known identifier: "local", "staging", "prod", or a
	// custom label defined by the operator.
	Name string `yaml:"name" json:"name"`

	// Kind is "local" for loopback environments and "remote" for SSH-accessed
	// environments. Capability resolution uses this to short-circuit probes.
	Kind                   string `yaml:"kind" json:"kind"`
	Tier                   Tier   `yaml:"tier,omitempty" json:"tier,omitempty"`
	RequireVerifiedRelease *bool  `yaml:"require_verified_release,omitempty" json:"require_verified_release,omitempty"`

	// Servers is the ordered list of hosts in this environment.
	Servers []Server `yaml:"servers" json:"servers"`
}

// Inventory is the top-level declarative model persisted in
// .nself/control-plane.yaml (mode 0600). SchemaVersion enables forward-compatible
// migrations. Secrets are never inlined here.
type Inventory struct {
	// SchemaVersion is incremented when the YAML structure changes in a
	// backward-incompatible way. Current supported version: 2.
	SchemaVersion int `yaml:"schema_version" json:"schema_version"`

	// Project is the canonical project name, matching the nSelf project
	// identifier (e.g. "nself", "ummat", "unity").
	Project string `yaml:"project" json:"project"`

	// Environments maps environment name to its definition. Keys must be unique
	// and match the Environment.Name field of their value.
	Environments map[string]Environment `yaml:"environments" json:"environments"`
}

// Capability describes the level of control the current host can exercise
// over a remote server, as determined by runtime probes.
type Capability string

const (
	// CapManage indicates full SSH + Docker access; deploy operations proceed.
	CapManage Capability = "manage"

	// CapReadOnly indicates the server is known but not fully reachable; the
	// pipeline reports a SKIPPED line for this server.
	CapReadOnly Capability = "read-only"

	// CapHidden indicates the server should not appear in normal output
	// (e.g. Host is empty, or not relevant to the current operation).
	CapHidden Capability = "hidden"
)

// TargetStatus is the resolved state of a single server after the capability
// resolver has run. It is the unit of work for the deployment pipeline.
//
// The Reason field uses the exact strings defined in §5.3 of the architecture
// spec so that callers can pattern-match without parsing free text.
type TargetStatus struct {
	// Env is the environment name this status belongs to.
	Env string

	// Server is the server name within that environment.
	Server string

	// Capability is the resolved access level for this target.
	Capability Capability

	// SSHReachable is true when the SSH probe completed without error.
	SSHReachable bool

	// KeyPresent is true when the key file referenced by SSHKeyRef exists on
	// the local filesystem.
	KeyPresent bool

	// DockerOK is true when the docker-info probe over SSH succeeded.
	DockerOK bool

	// LatencyMS is the round-trip time of the SSH probe in milliseconds.
	// Zero if the probe was skipped (local environment, key absent, etc.).
	LatencyMS int

	// Reason provides the human-readable explanation for the assigned
	// Capability. Empty string means capability is CapManage (no caveat).
	Reason string

	// ProbedAt records when these probe results were obtained.
	ProbedAt time.Time
}
