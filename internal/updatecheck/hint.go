package updatecheck

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// EnvOptIn is the opt-in switch: the value "1" turns the feature on.
const EnvOptIn = "NSELF_UPDATE_CHECK"

// Interval is the minimum time between refreshes and between hints.
const Interval = 24 * time.Hour

// ciMarkers are the environment variables that mean "running in CI".
var ciMarkers = []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "JENKINS_URL"}

// Guards carries everything the feature reads from the outside world, so tests
// inject a clock, a TTY answer, an environment and a server.
//
// JSON is the recorded JSON mode of the invocation. TTY reports whether stderr
// is a terminal. Path is the cache file, Current the running version, URL the
// endpoint and Client the HTTP client (nil means NewClient).
type Guards struct {
	JSON    bool
	TTY     func() bool
	Env     func(string) string
	Now     func() time.Time
	Path    string
	Current string
	URL     string
	Client  *http.Client
}

// Allowed reports whether the feature may run at all: opted in, not JSON mode,
// no CI marker, stderr is a terminal. Without the opt-in nothing else is read.
func Allowed(g Guards) bool {
	if g.Env == nil || g.Env(EnvOptIn) != "1" {
		return false
	}
	if g.JSON || g.TTY == nil || g.Now == nil || g.Path == "" {
		return false
	}
	for _, k := range ciMarkers {
		switch g.Env(k) {
		case "", "0", "false":
		default:
			return false
		}
	}
	return g.TTY()
}

// MaybeStartRefresh starts the background refresh when the feature is allowed
// and the cache is older than Interval. It never blocks: the returned channel
// closes when the refresh finishes (nil when none started), and production
// callers ignore it, so the goroutine is simply abandoned at process exit.
// A failed refresh keeps the old Latest and records CheckedAt and LastError.
func MaybeStartRefresh(g Guards) <-chan struct{} {
	if !Allowed(g) {
		return nil
	}
	if g.Now().Sub(Load(g.Path).CheckedAt) < Interval {
		return nil
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		client := g.Client
		if client == nil {
			client = NewClient()
		}
		url := g.URL
		if url == "" {
			url = VersionURL
		}
		latest, err := Refresh(context.Background(), client, url)
		c := Load(g.Path) // re-read: the hint may have recorded HintedAt meanwhile
		c.CheckedAt = g.Now()
		if err != nil {
			c.LastError = err.Error()
		} else {
			c.Latest, c.LastError = latest, ""
		}
		_ = Save(g.Path, c)
	}()
	return done
}

// Hint returns the one-line hint when it is due: allowed, the cached latest is
// a valid release newer than Current, and no hint was printed within Interval.
// It reads only the cache and records HintedAt before returning the line; if
// that record cannot be saved no line is returned, so it never repeats.
func Hint(g Guards) (string, bool) {
	if !Allowed(g) {
		return "", false
	}
	c := Load(g.Path)
	if !newer(c.Latest, g.Current) {
		return "", false
	}
	now := g.Now()
	if d := now.Sub(c.HintedAt); !c.HintedAt.IsZero() && d >= 0 && d < Interval {
		return "", false
	}
	c.HintedAt = now
	if Save(g.Path, c) != nil {
		return "", false
	}
	return fmt.Sprintf("A newer nself is available (current %s, latest %s). Run `nself update` to upgrade.", g.Current, c.Latest), true
}
