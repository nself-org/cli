package commands

// Purpose: the TLS section of the base `nself doctor`. For every host the
// served nginx terminates TLS for, it dials that nginx with SNI, prints the
// issuer and expiry, and warns or fails on the 21/7 day thresholds. It also
// warns when the served certificate differs from the file the host's own
// ssl_certificate line names (nginx not reloaded) and when a newer certificate
// for the host sits in the ACME store or /etc/letsencrypt (renewed but not
// installed, D-0121). Fronted projects use the fronting stack's confs, ssl
// dir, env and nginx (internal/nginxtopo served resolver).
// Inputs: the project dir and verbose flag. Outputs: doctorCheckResult values;
// a skip carries Status "skip" and a message without the word "expires", never
// counts as a pass and never changes doctor's exit code.
// Constraints: the only network destination is the served nginx address; the
// fronting stack's env is read with godotenv.Read, never loaded into this
// process; InsecureSkipVerify lives in ssl.ProbeServed and only reads the leaf.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/ui"
)

// tlsProbeTimeout bounds each host's dial plus handshake.
const tlsProbeTimeout = 5 * time.Second

// tlsDeps are the outside-world seams of the check, replaced in unit tests.
type tlsDeps struct {
	findNginx      func(context.Context, docker.ServiceMatch) (string, error)
	probe          func(ctx context.Context, addr, sni string, timeout time.Duration) (ssl.ServedCert, error)
	letsencryptDir string
}

// checkServedCertificates runs the served-certificate check for the project at
// projectDir against the real docker daemon and the real served nginx.
func checkServedCertificates(ctx context.Context, projectDir string, verbose bool) []doctorCheckResult {
	return checkServedCertificatesWith(ctx, projectDir, verbose, tlsDeps{
		findNginx:      docker.FindServiceContainer,
		probe:          ssl.ProbeServed,
		letsencryptDir: "/etc/letsencrypt",
	})
}

// tlsSkip records the one skip line `TLS: skipped (<reason>)`.
func tlsSkip(verbose bool, format string, a ...any) []doctorCheckResult {
	msg := fmt.Sprintf("skipped (%s)", fmt.Sprintf(format, a...))
	printTLS("skip", "TLS", msg, verbose)
	return []doctorCheckResult{{Name: "TLS", Status: "skip", Message: msg}}
}

// printTLS renders one TLS result; printCheck has no skip form.
func printTLS(status, name, msg string, verbose bool) {
	if status == "skip" {
		fmt.Fprintf(os.Stderr, "  %s %s: %s\n", ui.C(ui.Dim, "-"), name, msg)
		return
	}
	printCheck(status, name, msg, verbose)
}

// checkServedCertificatesWith is checkServedCertificates with injectable seams.
func checkServedCertificatesWith(ctx context.Context, projectDir string, verbose bool, d tlsDeps) []doctorCheckResult {
	cfg, err := loadConfigIsolated(projectDir)
	if err != nil {
		return tlsSkip(verbose, "cannot load config: %v", err)
	}
	if cfg.SSLMode == "local" || cfg.SSLMode == "none" {
		return tlsSkip(verbose, "SSL_MODE=%s", cfg.SSLMode)
	}
	root, err := nginxtopo.ServedRoot(projectDir, cfg.Nginx.FrontedBy)
	if err != nil {
		return tlsSkip(verbose, "%v", err)
	}
	container, err := d.findNginx(ctx, docker.ServiceMatch{
		Service: "nginx", WorkingDirs: []string{root, projectDir}, Project: cfg.ProjectName,
	})
	switch {
	case errors.Is(err, docker.ErrServiceContainerNotFound):
		return tlsSkip(verbose, "no running nginx container for this stack")
	case errors.Is(err, docker.ErrServiceContainerAmbiguous):
		return tlsResult(verbose, "TLS", "fail", "cannot tell which nginx serves this stack: "+err.Error())
	case err != nil:
		return tlsSkip(verbose, "cannot look for the nginx container: %v", err)
	}
	sslDir := filepath.Join(root, "ssl")
	hosts, err := ssl.ServedHosts(filepath.Join(root, "nginx"))
	if err != nil {
		return tlsSkip(verbose, "cannot read nginx confs: %v", err)
	}
	if len(hosts) == 0 {
		return tlsSkip(verbose, "no TLS server_name found in %s", filepath.Join(root, "nginx"))
	}
	env, err := ssl.ServedEnv(root, cfg.Env)
	if err != nil {
		return tlsSkip(verbose, "cannot read the served stack's env: %v", err)
	}
	if filepath.Clean(root) == filepath.Clean(projectDir) {
		// Own stack: fill what the two env files do not set from the full cascade.
		_, https := env["NGINX_HTTPS_PORT"]
		_, legacy := env["NGINX_SSL_PORT"]
		if !https && !legacy && cfg.Nginx.SSLPort > 0 {
			env["NGINX_HTTPS_PORT"] = strconv.Itoa(cfg.Nginx.SSLPort)
		}
		if _, ok := env["NGINX_BIND_IP"]; !ok {
			env["NGINX_BIND_IP"] = cfg.Nginx.BindIP
		}
	}
	addr := ssl.ServedAddr(env)

	names := make([]string, 0, len(hosts))
	for h := range hosts {
		names = append(names, h)
	}
	sort.Strings(names)
	var out []doctorCheckResult
	for _, h := range names {
		out = append(out, checkHostCert(ctx, d, verbose, container, addr, h, hosts[h], sslDir)...)
	}
	return out
}

// tlsResult prints and returns a single result.
func tlsResult(verbose bool, name, status, msg string) []doctorCheckResult {
	printTLS(status, name, msg, verbose)
	return []doctorCheckResult{{Name: name, Status: status, Message: msg}}
}

// checkHostCert probes one host and compares the served certificate with the
// file its conf names and with any newer certificate known on the host.
func checkHostCert(ctx context.Context, d tlsDeps, verbose bool, container, addr, host, confCert, sslDir string) []doctorCheckResult {
	name := "TLS " + host
	served, err := d.probe(ctx, addr, host, tlsProbeTimeout)
	if err != nil {
		return tlsResult(verbose, name, "fail",
			fmt.Sprintf("nginx container %s is running but %s did not answer a TLS handshake: %v", container, addr, err))
	}
	days := int(math.Floor(time.Until(served.NotAfter).Hours() / 24))
	status := string(ssl.Classify(days))
	if status == string(ssl.CertOK) {
		status = "pass"
	}
	issuer := served.IssuerCN
	if issuer == "" {
		issuer = "unknown issuer"
	}
	msg := fmt.Sprintf("%s expires %s (%d days)", issuer, served.NotAfter.UTC().Format("2006-01-02"), days)
	if days < 0 {
		msg += " EXPIRED"
	}
	res := tlsResult(verbose, name, status, msg)
	if served.ChainErr != "" {
		// Reported, not judged: a private CA or a missing system root store is not a stale certificate.
		res[0].Detail = "chain not verified: " + served.ChainErr
		if verbose {
			fmt.Fprintf(os.Stderr, "      %s\n", res[0].Detail)
		}
	}

	if disk, ok := ssl.DiskCertFor(sslDir, confCert); ok {
		dc, derr := ssl.ReadDiskCert(disk)
		switch {
		case derr != nil:
			res = append(res, tlsResult(verbose, name+" disk", "warn",
				fmt.Sprintf("cannot read %s (ssl_certificate %s in %s): %v", disk, confCert, sslDir, derr))...)
		case dc.SHA256 != served.SHA256:
			res = append(res, tlsResult(verbose, name+" disk", "warn",
				fmt.Sprintf("served certificate differs from %s in %s (served expires %s, file expires %s); reload nginx",
					disk, sslDir, served.NotAfter.UTC().Format("2006-01-02"), dc.NotAfter.UTC().Format("2006-01-02")))...)
		}
	}
	if nk, ok := ssl.NewestKnownCert(sslDir, host, d.letsencryptDir); ok && nk.NotAfter.After(served.NotAfter) && nk.SHA256 != served.SHA256 {
		res = append(res, tlsResult(verbose, name+" renewal", "warn",
			fmt.Sprintf("renewed but not installed: %s expires %s, served certificate expires %s; install/reload it into %s and reload nginx",
				nk.Path, nk.NotAfter.UTC().Format("2006-01-02"), served.NotAfter.UTC().Format("2006-01-02"), sslDir))...)
	}
	return res
}

// loadConfigIsolated is config.Load for the project with the process
// environment put back afterwards: Load overlays the .env cascade onto it, and
// a check must not change what later code sees.
func loadConfigIsolated(dir string) (*config.Config, error) {
	saved := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			saved[k] = v
		}
	}
	defer func() {
		for _, kv := range os.Environ() {
			if k, _, ok := strings.Cut(kv, "="); ok && k != "" {
				if _, keep := saved[k]; !keep {
					_ = os.Unsetenv(k)
				}
			}
		}
		for k, v := range saved {
			_ = os.Setenv(k, v)
		}
	}()
	return config.Load(dir)
}
