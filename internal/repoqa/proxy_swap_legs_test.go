package repoqa

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestProxySwapRecoveryLegsRunOnPullRequests: the round-trip and kill-mid-remove
// legs of the proxy swap e2e used to run only on manual dispatch, so a pull
// request that broke the way back from a swap, or the recovery after a kill,
// stayed green (D-1165). Every leg step must run on pull_request.
func TestProxySwapRecoveryLegsRunOnPullRequests(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/proxy-swap-e2e.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
				If   string `yaml:"if"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			for _, leg := range []string{"--add", "--round-trip", "--kill-mid-remove", "--renew"} {
				if !strings.Contains(step.Run, "proxy-swap.sh "+leg) {
					continue
				}
				seen[leg] = true
				if !strings.Contains(step.If, "github.event_name == 'pull_request'") {
					t.Errorf("step %q (proxy-swap.sh %s) does not run on pull_request: if = %q", step.Name, leg, step.If)
				}
			}
		}
	}
	for _, leg := range []string{"--add", "--round-trip", "--kill-mid-remove", "--renew"} {
		if !seen[leg] {
			t.Errorf("no workflow step runs proxy-swap.sh %s", leg)
		}
	}
}

// Exercise the matrix's actual normalizer without Docker or fixture credentials.
func TestProxySwapHeaderParity(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts/e2e/proxy-swap.sh"))
	if err != nil {
		t.Fatal(err)
	}
	_, source, ok := strings.Cut(string(data), "python3 - \"$W/headers\" \"$ws\" >> \"$output\" <<'PY'\n")
	if !ok {
		t.Fatal("matrix header normalizer not found")
	}
	source, _, ok = strings.Cut(source, "\nPY\n")
	if !ok {
		t.Fatal("matrix header normalizer terminator not found")
	}
	for _, tc := range []struct{ name, ws, input, want string }{
		{"vary", "0", "Vary: Accept-Encoding\r\n", "vary: Accept-Encoding\n"},
		{"upgrade vary", "1", "Vary: Origin\r\n", "vary: Origin\n"},
		{"upgrade hsts", "1", "Strict-Transport-Security: max-age=31536000\r\n", "strict-transport-security: max-age=31536000\n"},
		{"upgrade application header", "1", "X-App: value\r\n", "x-app: value\n"},
		{"document policy", "0", "X-Frame-Options: DENY\r\n", "x-frame-options: DENY\n"},
		{"upgrade document policy", "1", "Content-Security-Policy: default-src 'self'\r\nX-Frame-Options: DENY\r\n", "\n"},
		{"framing", "0", "Connection: keep-alive\r\nContent-Length: 42\r\nTransfer-Encoding: chunked\r\nKeep-Alive: timeout=5\r\n", "\n"},
		{"handshake", "1", "Upgrade: websocket\r\nSec-WebSocket-Accept: sample\r\n", "sec-websocket-accept: sample\nupgrade: websocket\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The matrix script reads a headers file named by argv[1]. Windows has
			// no /dev/stdin, so give it a real file, written and closed before the
			// normalizer starts.
			headers := filepath.Join(t.TempDir(), "headers")
			if err := os.WriteFile(headers, []byte(tc.input), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("python3", "-c", source, headers, tc.ws)
			got, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("normalizer: %v: %s", err, got)
			}
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
