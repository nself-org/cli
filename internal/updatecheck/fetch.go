package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/httptimeout"
	"github.com/nself-org/cli/internal/version"
)

// VersionURL is ping's release-channel endpoint (contract:ping.release-channel).
const VersionURL = "https://ping.nself.org/version"

// Budget is the whole-request limit of one refresh.
const Budget = 250 * time.Millisecond

// maxBody bounds the response read.
const maxBody = 64 << 10

// NewClient returns the refresh client: Budget timeout through the httptimeout
// funnel, compression off so no Accept-Encoding header is sent. The request
// then carries only Host and User-Agent.
func NewClient() *http.Client {
	c := httptimeout.WithTimeout(Budget)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableCompression = true
	c.Transport = tr
	return c
}

// Refresh asks url for the latest released CLI version and returns it
// normalised (no leading "v"). An empty, invalid or non-2xx answer is an
// error. The only request header besides Host is User-Agent: nself/<version>.
func Refresh(ctx context.Context, client *http.Client, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, Budget)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "nself/"+version.GetVersion())
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("version endpoint answered HTTP %d", resp.StatusCode)
	}
	var body struct {
		Latest string `json:"latestCliVersion"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&body); err != nil {
		return "", fmt.Errorf("version endpoint answer is not JSON: %w", err)
	}
	v := strings.TrimPrefix(strings.TrimSpace(body.Latest), "v")
	if _, ok := parse(v); !ok {
		return "", errors.New("version endpoint returned no valid latestCliVersion")
	}
	return v, nil
}

// parse reads a strict MAJOR.MINOR.PATCH release version. Pre-release and
// build suffixes are not accepted: a hint is only for a plain release.
func parse(s string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		if len(p) > 9 {
			return out, false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return out, false
			}
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// newer reports whether latest is a valid release strictly above current.
func newer(latest, current string) bool {
	l, ok := parse(latest)
	if !ok {
		return false
	}
	c, ok := parse(strings.TrimPrefix(current, "v"))
	if !ok {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}
