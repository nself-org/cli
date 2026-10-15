package remote

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// GlobalKnownHostsFileOption returns one quoted, absolute pin-file option.
// Inputs: a local path. Outputs: an OpenSSH -o value, or an error.
// Constraints: quotes, literal backslashes and controls cannot enter ssh's
// whitespace-separated file list. Windows separators become forward slashes.
func GlobalKnownHostsFileOption(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("SSH pin file path is empty")
	}
	if strings.ContainsAny(filepath.ToSlash(path), "\"\\") || strings.ContainsFunc(path, unicode.IsControl) {
		return "", fmt.Errorf("SSH pin file path %q has a quote, backslash or control character", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("SSH pin file path: %w", err)
	}
	sshPath := filepath.ToSlash(abs)
	if strings.ContainsAny(sshPath, "\"\\") || strings.ContainsFunc(sshPath, unicode.IsControl) {
		return "", fmt.Errorf("SSH pin file path %q has a quote, backslash or control character", abs)
	}
	return `GlobalKnownHostsFile="` + sshPath + `"`, nil
}
