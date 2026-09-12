// Package server implements `nself server` (G-011): provisioning, listing,
// resizing, and destroying cloud servers, backed by the Hetzner Cloud API.
//
// Purpose: shared data types for the server lifecycle. Kept separate from
// client.go so the wire-format structs (json tags) are easy to audit
// independent of transport/HTTP concerns.
// Inputs: none (pure type definitions).
// Outputs: none.
// Constraints: field sets are intentionally narrow — only what `nself server`
// itself needs, not a full Hetzner API mirror.
package server

import "time"

// Server is the subset of a Hetzner Cloud server this package cares about.
type Server struct {
	ID         int64             `json:"id"`
	Name       string            `json:"name"`
	Status     string            `json:"status"`
	ServerType string            `json:"server_type"`
	Location   string            `json:"location"`
	Created    time.Time         `json:"created"`
	IPv4       string            `json:"ipv4"`
	IPv6       string            `json:"ipv6"`
	IPv4ID     int64             `json:"ipv4_id,omitempty"`
	IPv6ID     int64             `json:"ipv6_id,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// ServerType describes a Hetzner server type's capacity, used by Resize to
// detect a disk-shrink request before ever calling the provider.
type ServerType struct {
	Name   string  `json:"name"`
	Cores  int     `json:"cores"`
	Memory float64 `json:"memory"`
	Disk   int     `json:"disk"` // GB
}

// PrimaryIP is a Hetzner primary IP resource. AutoDelete controls whether
// the IP is destroyed along with its assigned server — the exact footgun
// design requirement 2 exists to close.
type PrimaryIP struct {
	ID         int64  `json:"id"`
	IP         string `json:"ip"`
	Type       string `json:"type"` // "ipv4" | "ipv6"
	AssigneeID int64  `json:"assignee_id"`
	AutoDelete bool   `json:"auto_delete"`
}

// Image is a Hetzner image/snapshot resource.
type Image struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`   // "snapshot", "backup", "system", ...
	Status      string `json:"status"` // "creating" | "available"
	Description string `json:"description"`
}

// Action is a Hetzner async action (used to poll snapshot creation).
type Action struct {
	ID       int64        `json:"id"`
	Status   string       `json:"status"` // "running" | "success" | "error"
	Command  string       `json:"command"`
	Error    *ActionError `json:"error,omitempty"`
	Progress int          `json:"progress"`
}

// ActionError is the error payload embedded in a failed Action.
type ActionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ProvisionRequest describes a server to create.
type ProvisionRequest struct {
	Name       string
	ServerType string
	Location   string
	Image      string
	SSHKeys    []string
	Labels     map[string]string
	UserData   string
}

// ListOptions filters `nself server list`.
type ListOptions struct {
	LabelSelector string
}

// ResizeRequest describes a resize (change_type) request.
type ResizeRequest struct {
	ServerID    int64
	TargetType  string
	UpgradeDisk bool
}

// DestroyRequest describes a destroy request and the safety choices made by
// the operator invoking it.
type DestroyRequest struct {
	ServerID      int64
	TakeSnapshot  bool
	ForceNoBackup bool
	ReleaseIP     bool
	SnapshotWait  time.Duration
}

// DestroyResult reports what a Destroy call actually did, so the command
// layer can print an accurate summary (never assume from the request alone —
// e.g. a server with no primary IPs retains none).
type DestroyResult struct {
	SnapshotID  int64
	RetainedIPs []PrimaryIP
	ReleasedIPs []PrimaryIP
}
