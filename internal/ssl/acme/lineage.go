package acme

// lineage.go: contract:cli.tls-lineages v1, <served ssl>/.acme/lineages.json.
// Per lineage: names, challenge, DNS provider, credential secret NAMES and
// targets, never a secret value. File 0600; .acme 0700 with a `*` .gitignore.

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Contract constants; RenewWindow is how close to expiry a lineage is due.
const (
	SchemaVersion = "1"
	ChallengeDNS  = "dns-01"
	RenewWindow   = 30 * 24 * time.Hour
)

// Lineage is one managed certificate.
type Lineage struct {
	Name              string   `json:"name"`
	Domains           []string `json:"domains"`
	Challenge         string   `json:"challenge"`
	DNSProvider       string   `json:"dns_provider"`
	CredentialSecrets []string `json:"credential_secrets"`
	Targets           []string `json:"targets"`
	KeyType           string   `json:"key_type"`
	AdoptedFrom       *string  `json:"adopted_from"`
	LastIssued        *string  `json:"last_issued"`
}

// File is the lineages.json document.
type File struct {
	SchemaVersion string    `json:"schema_version"`
	Contact       string    `json:"contact"`
	Lineages      []Lineage `json:"lineages"`
}

var targetRE = regexp.MustCompile(`^certificates/[A-Za-z0-9_-][A-Za-z0-9._-]*$`)

// ValidTarget reports whether t is a safe lineage target: certificates/<one
// plain name>, never `.`, empty, absolute, hidden or containing `..`.
func ValidTarget(t string) bool { return targetRE.MatchString(t) && !strings.Contains(t, "..") }

// StateDir returns <sslDir>/.acme.
func StateDir(sslDir string) string { return filepath.Join(sslDir, ".acme") }

// EnsureState creates the 0700 state dir and its `*` .gitignore if absent.
func EnsureState(sslDir string) error {
	if err := os.MkdirAll(StateDir(sslDir), 0o700); err != nil {
		return err
	}
	gi := filepath.Join(StateDir(sslDir), ".gitignore")
	if _, err := os.Stat(gi); errors.Is(err, fs.ErrNotExist) {
		return os.WriteFile(gi, []byte("*\n"), 0o600)
	}
	return nil
}

// Load reads lineages.json; a missing file is an empty File.
func Load(sslDir string) (f File, err error) {
	data, err := os.ReadFile(filepath.Join(StateDir(sslDir), "lineages.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return File{SchemaVersion: SchemaVersion}, nil
	}
	if err == nil {
		err = json.Unmarshal(data, &f)
	}
	for _, l := range f.Lineages {
		for _, t := range l.Targets {
			if err == nil && !ValidTarget(t) {
				err = Refuse("fix or remove the lineage in lineages.json", "lineage %s has an invalid target %q (want certificates/<name>)", l.Name, t)
			}
		}
	}
	return f, err
}

// Save writes f atomically (temp + rename), lineages sorted, nil lists as [].
func Save(sslDir string, f File) error {
	if err := EnsureState(sslDir); err != nil {
		return err
	}
	f.SchemaVersion = SchemaVersion
	sort.Slice(f.Lineages, func(i, j int) bool { return f.Lineages[i].Name < f.Lineages[j].Name })
	for i := range f.Lineages {
		for _, s := range []*[]string{&f.Lineages[i].Domains, &f.Lineages[i].CredentialSecrets, &f.Lineages[i].Targets} {
			if *s == nil {
				*s = []string{}
			}
		}
	}
	data, _ := json.MarshalIndent(f, "", "  ")
	dst := filepath.Join(StateDir(sslDir), "lineages.json")
	if err := os.WriteFile(dst+".tmp", append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(dst+".tmp", dst)
}

// Due reports whether a certificate expiring at notAfter is due: force, or
// 30 days or fewer left.
func Due(notAfter, now time.Time, force bool) bool { return force || notAfter.Sub(now) <= RenewWindow }
