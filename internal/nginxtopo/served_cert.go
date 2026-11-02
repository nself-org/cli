package nginxtopo

import (
	"path/filepath"
	"strings"
)

// ServedCertDir returns <served ssl dir>/certificates/<domain-safe>.
// Dots and colons become dashes, preserving the certbot install convention.
// An unconfirmed fronted layout returns ErrFrontingUnresolved wrapped.
func ServedCertDir(projectDir, frontedBy, domain string) (string, error) {
	sslDir, err := ServedSSLDir(projectDir, frontedBy)
	if err != nil {
		return "", err
	}
	return filepath.Join(sslDir, "certificates", strings.NewReplacer(".", "-", ":", "-").Replace(domain)), nil
}
