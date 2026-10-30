// Package trustplan holds the exact text of the certificates and trust-store
// plan steps, and the local-development scope a plan id may bind (P7-SHIP-02).
//
// Layer: L2 (internal/build/**); stdlib only, imported by internal/build and
// internal/reconcile.
package trustplan

// trustplan.go — the exact text of the certificates and trust-store plan
// steps, and the local-development scope a plan id may bind (P7-SHIP-02).
//
// Purpose: a plan names what a write build will do outside the project tree.
// For certificates it lists every domain the certificate will cover; for the
// mkcert CA it names the CA by the sha256 of its root certificate, or says a
// new CA will be created. Those words are part of the plan, so the plan id
// binds them: a CA replaced or a domain added after the plan changes the id
// (E450). LocalDevStep says whether a step is one a plan id may bind at all.
// Inputs: the plan-mode SSL step (internal/build effects.go); reconcile's apply gate.
// Outputs: details, and the local-development scope test.
// Constraints: plan mode only reads the CA root (never runs mkcert). Every
// detail this file does not write, every domain that is not a local
// development name and every other target is outside the scope, so the apply
// keeps refusing it (E454).

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Target is the target of the trust-store step.
const Target = "mkcert CA"

// Effect kinds (build.EffectCertificates, build.EffectTrustStore).
const (
	certificates = "certificates"
	trustStore   = "trust-store"
)

// Details of the certificates and trust-store steps.
const (
	// CertGenerate and CertCopy start a certificates detail; the domains follow, ", "-joined.
	CertGenerate       = "generate a certificate for "
	CertCopy           = "copy the certificate from `nself trust` for "
	trustDetailInstall = "install the mkcert CA sha256:"
	trustDetailSuffix  = " into the system trust store"
	trustDetailNew     = "create a new mkcert CA and install it into the system trust store"
)

// CARoot is the mkcert CA root directory, found the way mkcert finds it:
// CAROOT, else the platform data dir.
func CARoot() string {
	if root := os.Getenv("CAROOT"); root != "" {
		return root
	}
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "mkcert")
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "mkcert")
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "mkcert")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "mkcert")
}

// TrustDetail names the CA the write build will install: the sha256 of the
// existing root certificate, or a new CA when there is none yet.
func TrustDetail(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "rootCA.pem"))
	if err != nil {
		return trustDetailNew
	}
	sum := sha256.Sum256(b)
	return trustDetailInstall + hex.EncodeToString(sum[:]) + trustDetailSuffix
}

// LocalDevStep reports whether a certificates or trust-store step is a local
// development step a plan id may bind: a certificate under projectDir's
// ssl/certificates for local development names only, or the mkcert CA
// install named by fingerprint (or as a new CA). The env class is the
// caller's to check.
func LocalDevStep(kind, target, detail, projectDir string) bool {
	switch kind {
	case certificates:
		rel, err := filepath.Rel(filepath.Join(projectDir, "ssl", "certificates"), target)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") || strings.ContainsRune(rel, filepath.Separator) {
			return false
		}
		list := ""
		for _, p := range []string{CertGenerate, CertCopy} {
			if strings.HasPrefix(detail, p) {
				list = strings.TrimPrefix(detail, p)
			}
		}
		if list == "" {
			return false
		}
		for _, d := range strings.Split(list, ", ") {
			if !localDevName(d) {
				return false
			}
		}
		return true
	case trustStore:
		if target != Target {
			return false
		}
		if detail == trustDetailNew {
			return true
		}
		fp, ok := strings.CutPrefix(detail, trustDetailInstall)
		fp, ok2 := strings.CutSuffix(fp, trustDetailSuffix)
		_, err := hex.DecodeString(fp)
		return ok && ok2 && len(fp) == 64 && err == nil && fp == strings.ToLower(fp)
	}
	return false
}

// localDevName reports whether a certificate name is local development only:
// a loopback address, localhost, local.nself.org, or a name under
// .local.nself.org, .localhost or .local (wildcards included).
func localDevName(d string) bool {
	d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "*."))
	if ip := net.ParseIP(d); ip != nil {
		return ip.IsLoopback()
	}
	return d == "localhost" || d == "local.nself.org" || strings.HasSuffix(d, ".local.nself.org") ||
		strings.HasSuffix(d, ".localhost") || strings.HasSuffix(d, ".local")
}
