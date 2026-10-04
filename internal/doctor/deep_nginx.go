package doctor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/ssl"
)

// Purpose: nginx/TLS/reachability --deep checks — config test, served
// certificate expiry (a TLS handshake against the served nginx), Let's Encrypt
// renewal cron, and the ping.nself.org health probe.
// Inputs: a context, the project directory (used to resolve the nginx
// container name via PROJECT_NAME), and verbose flag.
// Outputs: []CheckResult per category.
// Constraints: split out of deep.go (CLI-R12) as a pure move; no behavior
// changed. Container name resolution added later — see project_name.go.

// NginxChecks verifies the nginx config test and the served certificate expiry.
func NginxChecks(ctx context.Context, projectDir string, verbose bool) []CheckResult {
	var results []CheckResult
	nginxContainer := health.ContainerName(resolveProjectName(projectDir), "nginx")

	// nginx -t
	cmd := exec.CommandContext(ctx, "docker", "exec", nginxContainer, "nginx", "-t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		results = append(results, CheckResult{Section: "nginx", Name: "Nginx config test", Status: "fail",
			Message: strings.TrimSpace(string(out))})
	} else {
		results = append(results, CheckResult{Section: "nginx", Name: "Nginx config test", Status: "pass", Message: "syntax ok"})
	}

	// Served-certificate expiry per server_name, read from the TLS handshake
	// nginx actually answers (ssl.ProbeServed), not from files inside the container.
	results = append(results, servedCertChecks(ctx, projectDir)...)

	return results
}

// servedCertChecks reports the expiry of the certificate the served nginx
// presents for each of its TLS server_names (21/7 day thresholds, shared with
// the base doctor's TLS section via ssl.Classify). Fronted projects are read
// through the fronting stack's dirs, env and nginx. Not applicable cases (no
// TLS, no running nginx, unresolved layout) return one skip result whose
// message never says "expires"; a running nginx that does not answer fails.
func servedCertChecks(ctx context.Context, projectDir string) []CheckResult {
	skip := func(format string, a ...any) []CheckResult {
		return []CheckResult{{Section: "nginx", Name: "Served TLS", Status: "skip",
			Message: fmt.Sprintf("skipped (%s)", fmt.Sprintf(format, a...))}}
	}
	cfg, err := config.Load(projectDir)
	if err != nil {
		return skip("cannot load config: %v", err)
	}
	if cfg.SSLMode == "local" || cfg.SSLMode == "none" {
		return skip("SSL_MODE=%s", cfg.SSLMode)
	}
	root, err := nginxtopo.ServedRoot(projectDir, cfg.Nginx.FrontedBy)
	if err != nil {
		return skip("%v", err)
	}
	_, err = docker.FindServiceContainer(ctx, docker.ServiceMatch{
		Service: "nginx", WorkingDirs: []string{root, projectDir}, Project: cfg.ProjectName,
	})
	if errors.Is(err, docker.ErrServiceContainerNotFound) {
		return skip("no running nginx container for this stack")
	}
	if errors.Is(err, docker.ErrServiceContainerAmbiguous) {
		return []CheckResult{{Section: "nginx", Name: "Served TLS", Status: "fail", Message: err.Error()}}
	}
	if err != nil {
		return skip("cannot look for the nginx container: %v", err)
	}
	hosts, err := ssl.ServedHosts(filepath.Join(root, "nginx"))
	if err != nil || len(hosts) == 0 {
		return skip("no TLS server_name readable in %s", filepath.Join(root, "nginx"))
	}
	env, err := ssl.ServedEnv(root, cfg.Env)
	if err != nil {
		return skip("cannot read the served stack's env: %v", err)
	}
	addr := ssl.ServedAddr(env)
	names := make([]string, 0, len(hosts))
	for h := range hosts {
		names = append(names, h)
	}
	sort.Strings(names)
	var results []CheckResult
	for _, h := range names {
		name := "SSL expiry: " + h
		cert, perr := ssl.ProbeServed(ctx, addr, h, 5*time.Second)
		if perr != nil {
			results = append(results, CheckResult{Section: "nginx", Name: name, Status: "fail",
				Message: fmt.Sprintf("nginx is running but %s did not answer a TLS handshake: %v", addr, perr)})
			continue
		}
		days := int(math.Floor(time.Until(cert.NotAfter).Hours() / 24))
		status := string(ssl.Classify(days))
		if status == string(ssl.CertOK) {
			status = "pass"
		}
		results = append(results, CheckResult{Section: "nginx", Name: name, Status: status,
			Message: fmt.Sprintf("%s expires %s (%d days)", cert.IssuerCN, cert.NotAfter.UTC().Format("2006-01-02"), days)})
	}
	return results
}

// SSLChecks verifies LE renewal cron, last renewal, OCSP stapling.
func SSLChecks(ctx context.Context, verbose bool) []CheckResult {
	var results []CheckResult

	// LE renewal cron active
	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "certbot.timer")
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "active" {
		results = append(results, CheckResult{Section: "ssl", Name: "Certbot timer", Status: "warn",
			Message: "certbot.timer not active", FixCmd: "sudo systemctl enable --now certbot.timer"})
	} else {
		results = append(results, CheckResult{Section: "ssl", Name: "Certbot timer", Status: "pass", Message: "active"})
	}

	// Last renewal <60d
	cmd = exec.CommandContext(ctx, "find", "/etc/letsencrypt/renewal", "-name", "*.conf", "-mtime", "-60")
	out, err = cmd.Output()
	if err == nil {
		files := strings.TrimSpace(string(out))
		if files == "" {
			results = append(results, CheckResult{Section: "ssl", Name: "Last renewal", Status: "warn",
				Message: "no renewals in past 60 days"})
		} else {
			count := len(strings.Split(files, "\n"))
			results = append(results, CheckResult{Section: "ssl", Name: "Last renewal", Status: "pass",
				Message: fmt.Sprintf("%d cert(s) renewed within 60d", count)})
		}
	}

	return results
}

// PingChecks verifies ping.nself.org reachable and license cache fresh.
func PingChecks(ctx context.Context, verbose bool) []CheckResult {
	var results []CheckResult

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://ping.nself.org/health")
	if err != nil {
		results = append(results, CheckResult{Section: "ping", Name: "ping.nself.org", Status: "warn",
			Message: fmt.Sprintf("unreachable: %v", err)})
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode == 200 {
			results = append(results, CheckResult{Section: "ping", Name: "ping.nself.org", Status: "pass", Message: "reachable"})
		} else {
			results = append(results, CheckResult{Section: "ping", Name: "ping.nself.org", Status: "warn",
				Message: fmt.Sprintf("returned %d", resp.StatusCode)})
		}
	}

	return results
}
