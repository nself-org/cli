package ssl

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const (
	// hostsFile is the system hosts file path.
	hostsFile = "/etc/hosts"

	// hostsMarker is appended to every line that nself manages.
	hostsMarker = "# nself-managed"

	// hostsLoopback is the loopback address used for all nself entries.
	hostsLoopback = "127.0.0.1"
)

// AddHosts adds the given hostnames to /etc/hosts pointing to 127.0.0.1.
// Existing entries (with or without the nself-managed marker) are not
// duplicated. Each added line is tagged with "# nself-managed".
//
// Returns ErrSudoRequired if the file cannot be written due to permissions.
// Wildcard entries (e.g. *.example.com) are skipped because /etc/hosts does
// not support wildcards.
//
// AddHosts is the exported form for use by the dns-setup command.
func AddHosts(hostnames []string) (int, error) {
	filtered := filterHostsEntries(hostnames)
	if len(filtered) == 0 {
		return 0, nil
	}
	existing, err := readHostsFile(hostsFile)
	if err != nil {
		return 0, err
	}
	present := buildPresentSet(existing)
	var toAdd []string
	for _, h := range filtered {
		if !present[h] {
			toAdd = append(toAdd, h)
		}
	}
	if len(toAdd) == 0 {
		return 0, nil
	}
	if err := addHosts(toAdd); err != nil {
		return 0, err
	}
	return len(toAdd), nil
}

// canWriteHostsFile reports whether path can be opened for writing without
// actually modifying it — a pre-flight permission check so the automatic
// (build-time) hosts path can skip with a warning instead of attempting the
// write, discovering it can't, and only then falling back to a manual note.
// Opening with O_WRONLY and no O_TRUNC/O_APPEND never touches the file's
// contents; the file is closed immediately after the open succeeds.
func canWriteHostsFile(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func addHosts(hostnames []string) error {
	// Filter out entries that cannot go into /etc/hosts.
	filtered := filterHostsEntries(hostnames)
	if len(filtered) == 0 {
		return nil
	}

	// Read current hosts file content.
	existing, err := readHostsFile(hostsFile)
	if err != nil {
		return err
	}

	// Build a set of already-present hostnames (any line, not just ours).
	present := buildPresentSet(existing)

	// Determine which entries we actually need to add.
	var toAdd []string
	for _, h := range filtered {
		if !present[h] {
			toAdd = append(toAdd, h)
		}
	}
	if len(toAdd) == 0 {
		// Nothing new to add.
		return nil
	}

	// Append new entries.
	var sb strings.Builder
	for _, line := range existing {
		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	for _, h := range toAdd {
		sb.WriteString(hostsLoopback)
		sb.WriteByte('\t')
		sb.WriteString(h)
		sb.WriteByte('\t')
		sb.WriteString(hostsMarker)
		sb.WriteByte('\n')
	}

	if err := writeHostsFile(hostsFile, sb.String()); err != nil {
		return err
	}

	return nil
}

// filterHostsEntries removes entries that /etc/hosts cannot handle:
//   - wildcard entries (*.example.com)
//   - IP addresses (already resolve correctly without an entry)
//   - nip.io-style entries that contain an IP segment (they resolve via DNS)
func filterHostsEntries(hostnames []string) []string {
	var out []string
	for _, h := range hostnames {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		// Skip wildcards — /etc/hosts does not support them.
		if strings.HasPrefix(h, "*.") || strings.Contains(h, "*") {
			continue
		}
		// Skip raw IP addresses.
		if isIPAddress(h) {
			continue
		}
		// Skip nip.io-style names: they embed an IP so resolve via public DNS.
		// Heuristic: any subdomain whose labels are all digits separated by dashes
		// just before a known suffix. Simple check: if it contains ".nip.io" or
		// ".sslip.io" we skip it.
		if strings.HasSuffix(h, ".nip.io") ||
			strings.HasSuffix(h, ".sslip.io") ||
			strings.HasSuffix(h, ".xip.io") {
			continue
		}
		out = append(out, h)
	}
	return out
}

// readHostsFile reads /etc/hosts and returns lines (without trailing newlines).
// Empty last line is dropped to avoid double-newlines on append.
func readHostsFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsPermission(err) {
			return nil, ErrSudoRequired
		}
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return lines, nil
}

// writeHostsFile writes content to path atomically-ish.
// Returns ErrSudoRequired on permission errors.
func writeHostsFile(path string, content string) error {
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		if os.IsPermission(err) {
			return ErrSudoRequired
		}
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// buildPresentSet returns a set of all hostnames already in the hosts lines.
// It parses each non-comment line to extract the hostname fields.
func buildPresentSet(lines []string) map[string]bool {
	present := make(map[string]bool)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip comments and blanks.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Strip inline comment.
		if idx := strings.Index(trimmed, "#"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
		fields := strings.Fields(trimmed)
		// fields[0] is the IP; fields[1:] are hostnames.
		for _, f := range fields[1:] {
			present[f] = true
		}
	}
	return present
}
