package ssl

// served.go — inspect the certificate a running TLS endpoint actually serves.
//
// Purpose: files on disk say what nginx would serve after a reload; only a
// handshake says what it serves now (D-0121: a renewed certificate sat on
// disk while the served copy was about to expire). ProbeServed generalises the
// one-off handshake in cmd/commands/ssl.go (checkDomainTLS) so doctor, the
// ACME installer and `trust ssl status` share one prober and one set of
// verdict thresholds.
// Inputs: addr ("host:port" to dial), sni (the server name to present and to
// verify the certificate against) and a timeout covering dial plus handshake.
// Outputs: a ServedCert describing the leaf. A certificate that does not
// verify (untrusted chain, wrong hostname, expired) is reported in
// ServedCert.ChainErr and is not an error: the caller decides the verdict.
// Constraints: the only network destination is addr as passed by the caller;
// nothing is resolved, fetched or dialled beyond it.

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"time"
)

// defaultProbeTimeout bounds a probe when the caller passes a non-positive
// timeout, so a silent endpoint can never hang a command.
const defaultProbeTimeout = 10 * time.Second

// Days-remaining thresholds for Classify (D14: warn < 21, fail < 7 or expired).
const (
	certWarnDays = 21
	certFailDays = 7
)

// CertStatus is the verdict for a certificate's remaining lifetime.
type CertStatus string

// CertStatus values returned by Classify.
const (
	// CertOK means the certificate has at least 21 days left.
	CertOK CertStatus = "ok"
	// CertWarn means fewer than 21 and at least 7 days are left.
	CertWarn CertStatus = "warn"
	// CertFail means fewer than 7 days are left, or the certificate expired.
	CertFail CertStatus = "fail"
)

// Classify maps whole days until expiry (negative once expired) to a status:
// CertFail when days < 7, CertWarn when days < 21, otherwise CertOK.
func Classify(days int) CertStatus {
	switch {
	case days < certFailDays:
		return CertFail
	case days < certWarnDays:
		return CertWarn
	default:
		return CertOK
	}
}

// ServedCert describes the leaf certificate an endpoint served for one SNI.
type ServedCert struct {
	// Host is the name the probe asked for: sni, or the host of addr when sni is empty.
	Host string
	// IssuerCN is the leaf's issuer common name.
	IssuerCN string
	// NotAfter is the leaf's expiry instant.
	NotAfter time.Time
	// SHA256 is the lowercase hex SHA-256 of the leaf's DER bytes.
	SHA256 string
	// DNSNames are the leaf's subject alternative DNS names.
	DNSNames []string
	// ChainErr is the chain/hostname verification error, "" when the leaf
	// verified against the system roots for sni.
	ChainErr string
}

// ProbeServed dials addr, presents sni, and returns the leaf certificate the
// endpoint serves. The handshake uses InsecureSkipVerify so the leaf can be
// inspected even when it is untrusted or expired; the chain is then verified
// separately against the system roots (x509.VerifyOptions{DNSName: sni,
// Intermediates: the rest of the served chain}) and any failure, hostname
// mismatch included, is returned in ServedCert.ChainErr. A dial, handshake
// or empty-chain failure returns an error.
func ProbeServed(ctx context.Context, addr, sni string, timeout time.Duration) (ServedCert, error) {
	return probeServed(ctx, addr, sni, timeout, nil)
}

// probeServed is ProbeServed with injectable trust roots (nil = system roots),
// so tests can exercise a verifying chain without touching the host store.
func probeServed(ctx context.Context, addr, sni string, timeout time.Duration, roots *x509.CertPool) (ServedCert, error) {
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	d := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: timeout},
		// Inspection only: verification happens below and is reported, not enforced.
		Config: &tls.Config{ServerName: sni, InsecureSkipVerify: true}, //nolint:gosec // see ProbeServed
	}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return ServedCert{}, fmt.Errorf("probe %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	tc, ok := conn.(*tls.Conn)
	if !ok {
		return ServedCert{}, errors.New("probe " + addr + ": not a TLS connection")
	}
	chain := tc.ConnectionState().PeerCertificates
	if len(chain) == 0 {
		return ServedCert{}, fmt.Errorf("probe %s: no certificates returned", addr)
	}
	leaf := chain[0]

	host := sni
	if host == "" {
		if h, _, splitErr := net.SplitHostPort(addr); splitErr == nil {
			host = h
		} else {
			host = addr
		}
	}
	sum := sha256.Sum256(leaf.Raw)
	out := ServedCert{
		Host:     host,
		IssuerCN: leaf.Issuer.CommonName,
		NotAfter: leaf.NotAfter,
		SHA256:   hex.EncodeToString(sum[:]),
		DNSNames: append([]string(nil), leaf.DNSNames...),
	}

	inter := x509.NewCertPool()
	for _, c := range chain[1:] {
		inter.AddCert(c)
	}
	if _, verr := leaf.Verify(x509.VerifyOptions{DNSName: sni, Roots: roots, Intermediates: inter}); verr != nil {
		out.ChainErr = verr.Error()
	}
	return out, nil
}
