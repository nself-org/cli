package acme

// errors.go: maps an ACME failure to its v1.5 registry code (P7-LIVE-22).
//
// Purpose: the engine's errors are plain values; the commands raise E151 for
// all of them in v1.4. In v1.5 the failure class picks E470, E471 or E472.
// Inputs: the error an ACME run returned (any wrapping depth).
// Outputs: the code, or "" for an error that is none of the three classes (a
// preflight refusal, a state-file error), which keeps its E151.
// Constraints: the classes are told apart by the engine's own message text
// (lego.go, http01.go, install.go); TestSSLV15ErrorCodes (cmd/commands) drives the real
// functions so a reworded message fails there, not in the field.

import (
	"errors"
	"strings"
)

// Registry codes for the three failure classes (internal/errs/codes_acme.go).
const (
	CodeIssue      = "E470" // lego could not obtain a certificate
	CodeInstall    = "E471" // issued, but install or the served-certificate check failed
	CodeCredential = "E472" // the DNS credential is not in the secret store
)

// installMarkers are fragments of the errors Install returns.
var installMarkers = []string{
	"previous certificates restored", "ROLLBACK FAILED", "nothing installed",
	"switching ", "converting ", "repairing ", "refusing target", "nself generation link",
}

// issueMarkers are fragments of the errors the lego runs return.
var issueMarkers = []string{"lego run for ", "lego run (http-01) for "}

// CodeFor returns the registry code for err, or "" when err is none of the
// three classes.
func CodeFor(err error) string {
	if err == nil {
		return ""
	}
	var ae *Error
	if errors.As(err, &ae) {
		if strings.Contains(ae.What, "is not in the secret store") {
			return CodeCredential
		}
		return "" // a refusal: the run never reached the CA
	}
	msg := err.Error()
	for _, m := range installMarkers {
		if strings.Contains(msg, m) {
			return CodeInstall
		}
	}
	for _, m := range issueMarkers {
		if strings.Contains(msg, m) {
			return CodeIssue
		}
	}
	return ""
}
