package trust

import (
	"fmt"
	"runtime"
)

// TrustConfig holds runtime options for the trust setup.
type TrustConfig struct {
	WorkDir           string   // project root directory
	BaseDomain        string   // e.g. "ummat.local"
	NginxSSLPort      int      // e.g. 8443
	NginxHTTPPort     int      // e.g. 8080
	ExtraSSLDomains   []string // from EXTRA_SSL_DOMAINS
	NamespacePrefixes []string // subdomain namespaces extracted from ROUTES (e.g. "pro", "app", "dev")
	SkipDNS           bool
	SkipSSL           bool
	SkipPorts         bool
}

// TrustResult holds the outcome of each setup step.
type TrustResult struct {
	DNSConfigured       bool
	DNSAlreadyDone      bool
	ResolverConfigured  bool
	ResolverAlreadyDone bool
	CertsGenerated      bool
	CertsAlreadyDone    bool
	PortsConfigured     bool
	PortsAlreadyDone    bool
	// Skipped lists the steps ("dns", "ssl", "ports") that found their state
	// missing but made no system change because the process is headless
	// (NSELF_HEADLESS=1 or CI=1). They are not errors; the operator runs them
	// by hand on a machine with admin rights.
	Skipped []string
	// Headless is true when the run happened in headless mode.
	Headless bool
	Errors   []error
}

// skip records a step as skipped (headless).
func (r *TrustResult) skip(step string) {
	r.Skipped = append(r.Skipped, step)
}

// currentOS returns the OS name to dispatch on. Production always returns
// runtime.GOOS; tests may override this var to drive the cross-platform
// switch without rebuilding for a different GOOS.
var currentOS = func() string { return runtime.GOOS }

// Setup orchestrates the full local dev trust setup for the current OS.
// Steps run in order: DNS → SSL → Ports. Each step is skipped if the
// corresponding SkipXxx flag is set in cfg. Steps are idempotent —
// already-configured state is recorded in TrustResult but not treated as
// an error.
func Setup(cfg TrustConfig) (*TrustResult, error) {
	result := &TrustResult{Headless: isHeadless()}

	switch currentOS() {
	case "darwin":
		return setupDarwin(cfg, result)
	case "linux":
		return setupLinux(cfg, result)
	default:
		return nil, fmt.Errorf("unsupported OS: %s — nself trust supports macOS and Linux", currentOS())
	}
}

// Step-function seams for testability. Production wires to the real OS-specific
// implementations; tests may override to drive the success and error branches
// of setupDarwin / setupLinux without needing real admin privileges or brew.
var (
	setupDNSDarwinFunc   = SetupDNSDarwin
	setupMkcertFunc      = SetupMkcert
	setupPortsDarwinFunc = SetupPortsDarwin
	setupDNSLinuxFunc    = SetupDNSLinux
	setupPortsLinuxFunc  = SetupPortsLinux
)

// setupDarwin runs the macOS-specific trust setup pipeline.
func setupDarwin(cfg TrustConfig, result *TrustResult) (*TrustResult, error) {
	// Step 1: DNS
	if !cfg.SkipDNS {
		dnsAlready, resolverAlready, err := setupDNSDarwinFunc(cfg)
		if isSkipped(err) {
			result.skip("dns")
			result.DNSAlreadyDone = dnsAlready
			result.ResolverAlreadyDone = resolverAlready
		} else if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("dns setup: %w", err))
			// Continue — DNS failure is non-fatal for SSL + port steps.
		} else {
			result.DNSConfigured = true
			result.DNSAlreadyDone = dnsAlready
			result.ResolverConfigured = true
			result.ResolverAlreadyDone = resolverAlready
		}
	}

	// Step 2: SSL / mkcert
	if !cfg.SkipSSL {
		certsAlready, err := setupMkcertFunc(cfg)
		if isSkipped(err) {
			result.skip("ssl")
		} else if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("ssl setup: %w", err))
		} else {
			result.CertsGenerated = true
			result.CertsAlreadyDone = certsAlready
		}
	}

	// Step 3: Port forwarding
	if !cfg.SkipPorts {
		portsAlready, err := setupPortsDarwinFunc(cfg)
		if isSkipped(err) {
			result.skip("ports")
		} else if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("port forwarding setup: %w", err))
		} else {
			result.PortsConfigured = true
			result.PortsAlreadyDone = portsAlready
		}
	}

	return result, nil
}

// setupLinux runs the Linux trust setup pipeline.
func setupLinux(cfg TrustConfig, result *TrustResult) (*TrustResult, error) {
	// Step 1: DNS
	if !cfg.SkipDNS {
		dnsAlready, err := setupDNSLinuxFunc(cfg)
		if isSkipped(err) {
			result.skip("dns")
		} else if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("dns setup: %w", err))
		} else {
			result.DNSConfigured = true
			result.DNSAlreadyDone = dnsAlready
			result.ResolverConfigured = true
		}
	}

	// Step 2: SSL / mkcert
	if !cfg.SkipSSL {
		certsAlready, err := setupMkcertFunc(cfg)
		if isSkipped(err) {
			result.skip("ssl")
		} else if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("ssl setup: %w", err))
		} else {
			result.CertsGenerated = true
			result.CertsAlreadyDone = certsAlready
		}
	}

	// Step 3: Port forwarding
	if !cfg.SkipPorts {
		portsAlready, err := setupPortsLinuxFunc(cfg)
		if isSkipped(err) {
			result.skip("ports")
		} else if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("port forwarding setup: %w", err))
		} else {
			result.PortsConfigured = true
			result.PortsAlreadyDone = portsAlready
		}
	}

	return result, nil
}
