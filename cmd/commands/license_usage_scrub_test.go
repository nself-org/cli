package commands

// Purpose: tests of usage-error masking, identity text and held-token scrubbing (P7-SURF-16).
// Inputs: codedUsageError with a replaced invokedArgs.

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

// TestUsageErrorMasksLicenceKey: a licence key typed as the value of any flag
// (here the bool --yes, which is not secret-named) is masked in the E401 text
// that reaches the error envelope, for the parser's own wording and for a
// shorthand tail.
func TestUsageErrorMasksLicenceKey(t *testing.T) {
	compattest.Set(t, true)
	prev := invokedArgs
	invokedArgs = func() []string { return []string{"license", "revoke", "--yes=" + hostileOtherKey, "--json"} }
	t.Cleanup(func() { invokedArgs = prev })
	for _, msg := range []string{
		`invalid argument "` + hostileOtherKey + `" for "--yes" flag: strconv.ParseBool: parsing "` + hostileOtherKey + `": invalid syntax`,
		`unknown flag: --` + hostileOtherKey,
	} {
		err := codedUsageError(errors.New(msg))
		if err == nil {
			t.Fatalf("no error for %q", msg)
		}
		noSecrets(t, "usage error", err.Error(), hostileOtherKey)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E401" {
			t.Errorf("want E401, got %#v", err)
		}
	}
}

// TestUsageErrorHumanModeUnchanged: without --json a key typed as a flag value
// is left as the parser wrote it (human output stays byte-identical).
func TestUsageErrorHumanModeUnchanged(t *testing.T) {
	compattest.Set(t, true)
	prev := invokedArgs
	invokedArgs = func() []string { return []string{"license", "simulate-offline", "--days=" + hostileOtherKey} }
	t.Cleanup(func() { invokedArgs = prev })
	err := codedUsageError(errors.New(`invalid argument "` + hostileOtherKey + `" for "--days" flag`))
	if err == nil || !strings.Contains(err.Error(), hostileOtherKey) {
		t.Errorf("human usage text changed: %v", err)
	}
}

// TestUsageErrorKeepsLongFlagNames: human usage text is not rewritten by the
// opaque-token rule (a long camelCase flag name is not a token).
func TestUsageErrorKeepsLongFlagNames(t *testing.T) {
	compattest.Set(t, true)
	err := codedUsageError(errors.New("unknown flag: --someVeryLongCamelCaseOption"))
	if err == nil || !strings.Contains(err.Error(), "--someVeryLongCamelCaseOption") {
		t.Errorf("flag name rewritten: %v", err)
	}
}

// TestScrubKeepsIdentityText: emails, device names and hostnames are not
// tokens; the scrubber must leave them readable (a device id is shown to the
// user so that `devices --revoke <id>` works).
func TestScrubKeepsIdentityText(t *testing.T) {
	sc := newScrubber()
	for _, keep := range []string{
		"johnathan.richardson1985@example.com",
		"Alis-MacBook-Pro-14-inch.local",
		"ping-api-staging-1.nself.org",
		"dev-0b1c2d3e-4f50-4a6b-8c7d-9e0f1a2b3c4d",
		"nself-license-refresh.timer",
	} {
		if got := sc.str(keep); got != keep {
			t.Errorf("legitimate text rewritten: %q -> %q", keep, got)
		}
	}
	for _, bad := range []string{hostileToken, "tok-surf16-access-0123456789abcdef", "opaqueSessionTokenWithoutDigitsXYZ"} {
		if got := sc.str("x " + bad); strings.Contains(got, bad) {
			t.Errorf("token survived: %q", got)
		}
	}
}

// TestScrubErrKeepsExitStatus: an explicit exit status wrapping a coded error
// is still the exit status after the text is scrubbed.
func TestScrubErrKeepsExitStatus(t *testing.T) {
	in := &errs.ExitError{Code: 7, Err: errs.New("E401", "bad "+hostileOtherKey)}
	out := scrubErr(in)
	var xe *errs.ExitError
	if !errors.As(out, &xe) || xe.Code != 7 {
		t.Fatalf("exit status lost: %#v", out)
	}
	noSecrets(t, "scrubbed exit error", out.Error(), hostileOtherKey)
	var ce *errs.CLIError
	if !errors.As(out, &ce) || ce.Code != "E401" {
		t.Errorf("code lost: %#v", out)
	}
}

// TestScrubRedactsHeldTokensAndSegmentedTokens: the tokens in the auth file are
// redacted whatever their shape, and a base64url token split into short mixed
// segments is not mistaken for a device name.
func TestScrubRedactsHeldTokensAndSegmentedTokens(t *testing.T) {
	home := isolateHome(t)
	const held = "short7"
	writeTestAuthFile(t, home, &auth.AuthFile{AccessToken: held + "-access", SessionToken: held})
	const segmented = "aB3dE5fG7h_Jk2Lm4No6P_qR8sT0uV1w_Xy2Za4Bc6D"
	got := newScrubber().str("a " + held + "-access b " + held + " c " + segmented)
	for _, secret := range []string{held, segmented} {
		if strings.Contains(got, secret) {
			t.Errorf("secret survived: %q", got)
		}
	}
	if keep := "Alis-MacBook-Pro-14-inch"; newScrubber().str(keep) != keep {
		t.Error("device name rewritten")
	}
}
