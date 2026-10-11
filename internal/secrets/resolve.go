package secrets

// Purpose: resolve D5 references through the existing age store, in memory.
// Inputs: project root, references, attempt id and an audit sink.
// Outputs: ordered values or a fixed, value-free error; one audit event.
// Constraints: no plaintext files; callers enforce FIFO-only delivery.
// SPORT: P7-TRUST-03, contract:cli.secret-resolve/v1.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

type Ref struct{ Raw, Class, Env, Name string }

var refNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)
var refEnvPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// validRefEnv validates syntax and rejects names that collide with store classes.
func validRefEnv(s string) bool {
	return refEnvPattern.MatchString(s) && s != "ci" && s != "release" && s != "team" && !strings.HasPrefix(s, "ci-")
}

// ParseRef validates the complete D5 grammar before any store access.
func ParseRef(s string) (Ref, error) {
	p := strings.Split(s, "/")
	r := Ref{Raw: s}
	bad := errs.New("E540", "Invalid secret reference")
	if len(p) < 2 {
		return Ref{}, bad
	}
	r.Class = p[0]
	switch r.Class {
	case "project", "team", "release":
		if len(p) != 2 {
			return Ref{}, bad
		}
	case "environment", "deploy":
		if len(p) != 3 || !validRefEnv(p[1]) {
			return Ref{}, bad
		}
		r.Env = p[1]
	default:
		return Ref{}, bad
	}
	r.Name = p[len(p)-1]
	if !refNamePattern.MatchString(r.Name) {
		return Ref{}, bad
	}
	return r, nil
}

// FileFor revalidates Ref so manually constructed refs cannot escape a root.
func FileFor(projectRoot, home string, r Ref) (path, source string, err error) {
	parsed, err := ParseRef(r.Raw)
	if err != nil || parsed != r {
		return "", "", errs.New("E540", "Invalid secret reference")
	}
	name := ""
	switch r.Class {
	case "project":
		name = "ci"
	case "environment":
		name = "ci-" + r.Env
	case "deploy":
		name = r.Env
	case "release":
		name = "release"
	case "team":
		path = filepath.Join(home, ".config", "nself", "secrets", "team.age")
	}
	if path == "" {
		path = filepath.Join(projectRoot, ".secrets", name+".age")
	}
	return path, "age:" + filepath.Base(path), nil
}

type ResolveRequest struct {
	ProjectRoot string
	Refs        []string
	AttemptID   string
}

type AuditFunc func(event string, fields map[string]string) error

type Value struct {
	Ref       string  `json:"ref"`
	EnvName   string  `json:"env_name"`
	Value     string  `json:"value"`
	Source    string  `json:"source"`
	ExpiresAt *string `json:"expires_at"`
}

// auditUser uses USER or the current OS account for audit metadata.
func auditUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "unknown"
}

// AuditResolve is shared with command refusals; never pass values to the sink.
// A failed sink is deliberately not wrapped: sink errors are not trusted text.
func AuditResolve(sink AuditFunc, req ResolveRequest, outcome string) error {
	refs := req.Refs
	if refs == nil {
		refs = []string{}
	}
	b, _ := json.Marshal(refs)
	project, err := filepath.Abs(req.ProjectRoot)
	if err != nil {
		return errs.New("E401", "Cannot determine absolute project directory")
	}
	if sink == nil || sink("secret-resolve", map[string]string{
		"refs": string(b), "attempt_id": req.AttemptID, "project": project,
		"user": auditUser(), "outcome": outcome,
	}) != nil {
		return errs.New("E401", "Secret resolution audit could not be written")
	}
	return nil
}

// readableIdentity opens the configured regular identity before decrypting.
func readableIdentity() (string, error) {
	p, err := ageKeyPath()
	if err != nil {
		return "", decryptError()
	}
	f, err := os.Open(p)
	if err != nil {
		return "", decryptError()
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return "", decryptError()
	}
	return p, nil
}

// decryptError hides age diagnostics and plaintext-derived JSON errors.
func decryptError() error {
	return errs.New("E543", "Age identity unreadable or secret store decryption failed")
}

// loadTeamStore is path-rooted because loadStore always roots at project/.secrets.
// Decrypted stdout stays in memory; age stderr and JSON errors never escape.
func loadTeamStore(ctx context.Context, path, key string) (*SecretStore, error) {
	b, err := exec.CommandContext(ctx, "age", "--decrypt", "-i", key, path).Output()
	if err != nil {
		return nil, decryptError()
	}
	defer func() { clear(b) }()
	var store SecretStore
	if json.Unmarshal(b, &store) != nil {
		return nil, decryptError()
	}
	return &store, nil
}

// Resolve audits exactly once, including validation failures and partial loads.
// Audit failure drops all values. All refs and collisions are checked first.
func Resolve(ctx context.Context, req ResolveRequest, sink AuditFunc) (values []Value, err error) {
	defer func() {
		outcome := "ok"
		if err != nil {
			outcome = "E543"
			var ce *errs.CLIError
			if errors.As(err, &ce) {
				outcome = ce.Code
			}
			values = nil
		}
		if ae := AuditResolve(sink, req, outcome); ae != nil {
			values, err = nil, ae
		}
	}()
	refs, err := parseResolveRefs(req.Refs)
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, decryptError()
	}
	stores := map[string]*SecretStore{}
	values = []Value{}
	for _, r := range refs {
		path, source, _ := FileFor(req.ProjectRoot, home, r)
		store := stores[path]
		if store == nil {
			store, err = loadRefStore(ctx, req.ProjectRoot, path, r)
			if err != nil {
				return nil, err
			}
			stores[path] = store
		}
		entry, ok := store.Secrets[r.Name]
		if !ok {
			return nil, errs.Newf("E541", "Secret reference %s not found", r.Raw)
		}
		values = append(values, Value{Ref: r.Raw, EnvName: r.Name, Value: entry.Value, Source: source})
	}
	return values, nil
}

// parseResolveRefs validates all refs and rejects duplicate environment names.
func parseResolveRefs(raw []string) ([]Ref, error) {
	if len(raw) == 0 {
		return nil, errs.New("E540", "At least one secret reference is required")
	}
	refs := make([]Ref, 0, len(raw))
	seen := map[string]bool{}
	for _, s := range raw {
		r, err := ParseRef(s)
		if err != nil {
			return nil, err
		}
		if seen[r.Name] {
			return nil, errs.New("E540", "Secret references collide on environment variable name")
		}
		seen[r.Name] = true
		refs = append(refs, r)
	}
	return refs, nil
}

// loadRefStore loads one existing mapped store with sanitized failure messages.
func loadRefStore(ctx context.Context, root, path string, r Ref) (*SecretStore, error) {
	if ctx.Err() != nil {
		return nil, decryptError()
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, errs.Newf("E541", "Secret reference %s not found", r.Raw)
		}
		return nil, decryptError()
	}
	key, err := readableIdentity()
	if err != nil {
		return nil, err
	}
	if r.Class == "team" {
		return loadTeamStore(ctx, path, key)
	}
	store, err := loadStore(root, strings.TrimSuffix(filepath.Base(path), ".age"))
	if err != nil {
		return nil, decryptError()
	}
	return store, nil
}
