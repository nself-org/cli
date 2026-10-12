package commands

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	"github.com/nself-org/cli/internal/config"
)

// Purpose: list the config key names the project's own env cascade defines.
// Inputs: project directory. Outputs: sorted distinct names, never values.
// Constraints: the host shell is not consulted, so the list is the same from
// any shell; an unreadable or unparsable cascade file contributes nothing
// (config.Load already rejected malformed files before this runs).
func dbExportProjectKeys(dir string) []string {
	env, _ := config.ResolveEnv(dir)
	names := map[string]bool{}
	for _, f := range config.EnvCascade(dir, env, config.LegacyOrderActive()) {
		if !f.Exists {
			continue
		}
		data, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}
		m, err := godotenv.UnmarshalBytes(data)
		if err != nil {
			continue
		}
		for k := range m {
			names[k] = true
		}
	}
	keys := []string{}
	for k := range names {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Purpose: read the env the project's containers run with.
// Inputs: project directory. Outputs: KEY=value pairs from .nself/compose.env,
// or nil when it is absent or unparsable. Constraints: read-only; the values
// only widen the scrubber.
func dbExportComposeEnv(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, ".nself", "compose.env"))
	if err != nil {
		return nil
	}
	m, err := godotenv.UnmarshalBytes(data)
	if err != nil {
		return nil
	}
	pairs := make([]string, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, k+"="+v)
	}
	return pairs
}

// dbExportMinSecret is the shortest value the scrubber matches; shorter ones
// would flag ordinary words.
const dbExportMinSecret = 6

// dbExportTokenUserMin is the shortest username-only URL userinfo treated as
// a token (https://TOKEN@host). Real usernames such as postgres or admin are
// shorter; a token this long is a credential.
const dbExportTokenUserMin = 16

var (
	// A query parameter name that carries a credential word, mirroring the
	// rule config.MaskURLCredentials applies (it exports no value finder).
	secretParamName = regexp.MustCompile(`(?i)pass|pwd|token|secret|key|signature|auth|credential`)
	// scheme://user[:password]@ : group 1 is the username, which is a bare
	// token when no password follows (https://TOKEN@host) and also the token
	// in GitHub's https://TOKEN:x-oauth-basic@host form.
	tokenUserinfo = regexp.MustCompile(`://([^/:@?#\s]+)(?::[^/@?#\s]*)?@`)
)

// Purpose: collect the widened secret class from the resolved environment.
// Inputs: KEY=value pairs. Outputs: sorted distinct secret values only.
// Constraints: more values only make the scrubber stricter. Covered: values
// of secret-named keys; URL userinfo passwords and long username-only tokens;
// query parameters named like credentials on any URL; and, for a secret-named
// value that is JSON (a JWT config, a service-account file), each string leaf
// under a secret-like key name, for any env key.
func dbExportSecretValues(env []string) []string {
	values := map[string]bool{}
	add := func(v string) {
		if len(v) >= dbExportMinSecret {
			values[v] = true
		}
	}
	for _, pair := range env {
		key, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		secret := config.IsSecretKey(key)
		for _, term := range []string{"SECRET", "PASSWORD", "PASS", "KEY", "TOKEN", "CREDENTIAL", "PRIVATE", "DSN"} {
			if strings.Contains(strings.ToUpper(key), term) {
				secret = true
			}
		}
		if secret {
			add(v)
		}
		// A service-account file sits under names like SERVICE_ACCOUNT_JSON,
		// so JSON values are mined whatever their key is called.
		dbExportJSONLeaves(v, add)
		if u, err := url.Parse(v); err == nil && u.User != nil {
			if p, ok := u.User.Password(); ok {
				add(p)
				if len(u.User.Username()) >= dbExportTokenUserMin {
					add(u.User.Username())
				}
			}
		}
		if strings.Contains(v, "://") {
			dbExportURLSecrets(v, add)
		}
	}
	vs := []string{}
	for v := range values {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	return vs
}

// Purpose: find credentials in a URL-shaped value. Inputs: the value and an
// adder. Outputs: userinfo tokens and credential-named query values (raw and
// percent-decoded) through add.
func dbExportURLSecrets(v string, add func(string)) {
	if m := tokenUserinfo.FindStringSubmatch(v); m != nil && len(m[1]) >= dbExportTokenUserMin {
		add(m[1])
		if dec, err := url.PathUnescape(m[1]); err == nil {
			add(dec)
		}
	}
	_, query, ok := strings.Cut(v, "?")
	if !ok {
		return
	}
	query, _, _ = strings.Cut(query, "#")
	for _, part := range strings.FieldsFunc(query, func(r rune) bool { return r == '&' || r == ';' }) {
		name, val, found := strings.Cut(part, "=")
		if !found || !secretParamName.MatchString(name) {
			continue
		}
		add(val)
		if dec, err := url.QueryUnescape(val); err == nil {
			add(dec)
		}
	}
}

// Purpose: pull string leaves out of a JSON-valued secret. Inputs: a value and
// an adder. Outputs: each string under a secret-like key name (key, secret,
// private_key, password, token ...) through add. Constraints: non-JSON values
// are ignored; arrays under a secret-like name count as leaves.
func dbExportJSONLeaves(v string, add func(string)) {
	v = strings.TrimSpace(v)
	if v == "" || (v[0] != '{' && v[0] != '[') {
		return
	}
	var doc any
	if err := json.Unmarshal([]byte(v), &doc); err != nil {
		return
	}
	var walk func(node any, secretName bool)
	walk = func(node any, secretName bool) {
		switch n := node.(type) {
		case string:
			if secretName {
				add(n)
			}
		case []any:
			for _, e := range n {
				walk(e, secretName)
			}
		case map[string]any:
			for k, e := range n {
				walk(e, config.IsSecretKey(k))
			}
		}
	}
	walk(doc, false)
}
