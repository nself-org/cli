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
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/doctor"
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
	cfg, err := doctor.LoadConfigIsolated(projectDir)
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
	hosts, herr := ssl.ServedHosts(filepath.Join(root, "nginx"))
	var out []doctorCheckResult
	if herr != nil {
		out = tlsResult(verbose, "TLS conf", "warn", "some nginx confs were not read, their hosts are not checked: "+herr.Error())
	}
	if len(hosts) == 0 {
		if herr != nil {
			return out
		}
		return tlsSkip(verbose, "no TLS server_name found in %s", filepath.Join(root, "nginx"))
	}
	var env map[string]string
	if filepath.Clean(root) == filepath.Clean(projectDir) {
		// Own stack: compose publishes the full cascade's port and bind IP.
		env = map[string]string{"NGINX_BIND_IP": cfg.Nginx.BindIP}
		if cfg.Nginx.SSLPort > 0 {
			env["NGINX_HTTPS_PORT"] = strconv.Itoa(cfg.Nginx.SSLPort)
		}
	} else if env, err = ssl.ServedEnv(root, cfg.Env); err != nil {
		return tlsSkip(verbose, "cannot read the served stack's env: %v", err)
	}
	pr := &tlsProber{d: d, verbose: verbose, container: container, addr: ssl.ServedAddr(env),
		sslDir: filepath.Join(root, "ssl"), letsencrypt: cfg.SSLMode == "letsencrypt"}

	names := make([]string, 0, len(hosts))
	for h := range hosts {
		names = append(names, h)
	}
	sort.Strings(names)
	for _, h := range names {
		out = append(out, pr.check(ctx, h, hosts[h])...)
	}
	return out
}

// tlsResult prints and returns a single result.
func tlsResult(verbose bool, name, status, msg string) []doctorCheckResult {
	printTLS(status, name, msg, verbose)
	return []doctorCheckResult{{Name: name, Status: status, Message: msg}}
}

// tlsProber probes the hosts of one served stack on one address.
type tlsProber struct {
	d           tlsDeps
	verbose     bool
	container   string
	addr        string
	sslDir      string
	letsencrypt bool  // SSL_MODE=letsencrypt: a chain that does not verify is a warning
	dead        error // first dial failure; later hosts fail without dialling again
}

// check probes one host and compares the served certificate with the host
// name, the file its conf names and any newer certificate known on the host.
func (p *tlsProber) check(ctx context.Context, host, confCert string) []doctorCheckResult {
	name, v := "TLS "+host, p.verbose
	var served ssl.ServedCert
	err := p.dead
	if err == nil {
		served, err = p.d.probe(ctx, p.addr, host, tlsProbeTimeout)
		if doctor.DeadAddr(err) {
			p.dead = err
		}
	}
	if err != nil {
		return tlsResult(v, name, "fail",
			fmt.Sprintf("nginx container %s is running but %s did not answer a TLS handshake: %v", p.container, p.addr, err))
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
	servedDay := served.NotAfter.UTC().Format("2006-01-02")
	msg := fmt.Sprintf("%s expires %s (%d days)", issuer, servedDay, days)
	if days < 0 {
		msg += " EXPIRED"
	}
	res := tlsResult(v, name, status, msg)

	// The leaf must cover the host (RFC 6125: a wildcard spans one label). The
	// verdict does not match on ssl.ServedCert.ChainErr text, which differs per platform.
	if (&x509.Certificate{DNSNames: served.DNSNames}).VerifyHostname(host) != nil {
		res = append(res, tlsResult(v, name+" name", "fail", fmt.Sprintf(
			"served certificate covers [%s], not %s: clients will reject it", strings.Join(served.DNSNames, " "), host))...)
	} else if served.ChainErr != "" {
		// Not judged for custom certificates: a private CA is not a broken chain.
		detail := "chain not verified: " + served.ChainErr
		if p.letsencrypt {
			res = append(res, tlsResult(v, name+" chain", "warn", detail+" (serve fullchain.pem, not cert.pem)")...)
		} else {
			res[0].Detail = detail
			fmt.Fprintf(os.Stderr, "      %s\n", detail)
		}
	}

	switch disk, mapped := ssl.DiskCertFor(p.sslDir, confCert); {
	case confCert == "":
		res = append(res, tlsResult(v, name+" disk", "warn",
			"no ssl_certificate found for this host in the nginx confs (include?): served vs disk not checked")...)
	case !mapped:
	default:
		dc, derr := ssl.ReadDiskCert(disk)
		switch {
		case derr != nil:
			res = append(res, tlsResult(v, name+" disk", "warn", fmt.Sprintf(
				"cannot read %s (ssl_certificate %s in %s): %v; nginx -t fails on the next reload", disk, confCert, p.sslDir, derr))...)
		case dc.SHA256 != served.SHA256:
			res = append(res, tlsResult(v, name+" disk", "warn", fmt.Sprintf(
				"served certificate differs from %s in %s (served expires %s, file expires %s); reload nginx",
				disk, p.sslDir, servedDay, dc.NotAfter.UTC().Format("2006-01-02")))...)
		}
	}
	if nk, ok := ssl.NewestKnownCert(p.sslDir, host, p.d.letsencryptDir); ok && nk.NotAfter.After(served.NotAfter) && nk.SHA256 != served.SHA256 {
		res = append(res, tlsResult(v, name+" renewal", "warn", fmt.Sprintf(
			"renewed but not installed: %s expires %s, served certificate expires %s; install/reload it into %s and reload nginx",
			nk.Path, nk.NotAfter.UTC().Format("2006-01-02"), servedDay, p.sslDir))...)
	}
	return res
}
