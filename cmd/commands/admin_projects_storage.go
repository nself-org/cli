package commands

// Purpose: on-disk persistence for admin connections/projects: the config
// dir resolver and the load/save helpers. Inputs are an AdminConnection or
// []AdminProject; outputs are a saved/loaded file or an error.
// Constraints: split out of admin_connect.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func adminConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "nself", "admin")
}

func saveAdminConnection(conn AdminConnection) error {
	dir := adminConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(dir, "connections.json")
	var conns []AdminConnection

	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &conns)
	}

	// Replace or append
	found := false
	for i, c := range conns {
		if c.Host == conn.Host {
			conns[i] = conn
			found = true
			break
		}
	}
	if !found {
		conns = append(conns, conn)
	}

	out, err := json.MarshalIndent(conns, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func loadAdminProjects() ([]AdminProject, error) {
	path := filepath.Join(adminConfigDir(), "projects.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. No projects.json means no projects registered yet — the
			// state of every machine before the first `nself admin` run, and
			// the same empty list an existing-but-empty file would give.
			// ENOENT only: saveAdminProjects rewrites this file wholesale, so
			// an unreadable file read as "no projects" would erase the
			// existing registrations on the next write.
			return nil, nil
		}
		return nil, err
	}
	var projects []AdminProject
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func saveAdminProjects(projects []AdminProject) error {
	dir := adminConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "projects.json"), data, 0o644)
}
