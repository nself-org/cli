package licensing

// Tests for contract plugin.entitlement-token v1.1 (D-0851): one JWT per line,
// any line that verifies for the slug authorizes, deadline is the max over
// matching lines only, and line 1 stays a complete v1 file.

import (
	"context"
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/licensing/licensetest"
)

func lineToken(t *testing.T, signer ed25519.PrivateKey, plugins []string, exp time.Time) string {
	t.Helper()
	c := claimsAt(exp)
	c["plugins"] = plugins
	return tokenFor(t, signer, header(), c)
}

func TestVerifyEntitlementMultiLine(t *testing.T) {
	pub, priv := syntheticKey(t)
	_, other := syntheticKey(t)
	licensetest.UseKey(t, pub)
	now := time.Now()
	future := now.Add(time.Hour)
	expired := now.Add(-EntitlementGracePeriod - time.Hour)
	chat := lineToken(t, priv, []string{"chat"}, future)
	claw := lineToken(t, priv, []string{"claw"}, future)
	forged := lineToken(t, other, []string{"chat"}, future)
	stale := lineToken(t, priv, []string{"chat"}, expired)
	for _, tc := range []struct {
		name  string
		lines []string
		slug  string
		want  bool
	}{
		{"one-key-as-today", []string{chat}, "chat", true},
		{"one-key-other-slug", []string{chat}, "claw", false},
		{"two-keys-first-bundle", []string{chat, claw}, "chat", true},
		{"two-keys-second-bundle", []string{chat, claw}, "claw", true},
		{"bad-signature-line-1-valid-line-2", []string{forged, chat}, "chat", true},
		{"bad-signature-only", []string{forged}, "chat", false},
		{"malformed-line-1-valid-line-2", []string{"not.a.jwt", chat}, "chat", true},
		{"expired-line-then-valid", []string{stale, chat}, "chat", true},
		{"valid-then-expired-line", []string{chat, stale}, "chat", true},
		{"expired-only", []string{stale}, "chat", false},
		{"slug-no-line-covers", []string{chat, claw}, "paid", false},
		{"crlf-and-blank-lines", []string{chat + "\r", "", claw}, "claw", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setToken(t, strings.Join(tc.lines, "\n"))
			if got := VerifyEntitlement(tc.slug, now) == nil; got != tc.want {
				t.Fatalf("authorized=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestLinesDeadlineMaxOverMatchingLinesOnly(t *testing.T) {
	pub, priv := syntheticKey(t)
	_, other := syntheticKey(t)
	licensetest.UseKey(t, pub)
	base := time.Unix(1800000000, 0)
	grace := int64(EntitlementGracePeriod / time.Second)
	early := lineToken(t, priv, []string{"chat"}, base)
	late := lineToken(t, priv, []string{"chat"}, base.Add(48*time.Hour))
	other1 := lineToken(t, priv, []string{"claw"}, base.Add(96*time.Hour))
	forged := lineToken(t, other, []string{"chat"}, base.Add(200*time.Hour))
	for _, tc := range []struct {
		name  string
		lines []string
		want  int64
		found bool
	}{
		{"single", []string{early}, base.Unix() + grace, true},
		{"max-of-matching", []string{early, late}, base.Unix() + 48*3600 + grace, true},
		{"order-independent", []string{late, early}, base.Unix() + 48*3600 + grace, true},
		{"ignores-non-matching-slug", []string{early, other1}, base.Unix() + grace, true},
		{"ignores-forged-later-line", []string{early, forged}, base.Unix() + grace, true},
		{"none-match", []string{other1, forged}, 0, false},
		{"empty", nil, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, found := linesDeadline(tc.lines, "chat")
			if got != tc.want || found != tc.found {
				t.Fatalf("deadline=%d found=%v, want %d %v", got, found, tc.want, tc.found)
			}
		})
	}
	// VerifyEntitlement agrees: past the late line's grace, the later claw line grants chat nothing.
	setToken(t, strings.Join([]string{early, late, other1}, "\n"))
	pastLate := time.Unix(base.Unix()+48*3600+grace+1, 0)
	if VerifyEntitlement("chat", pastLate) == nil {
		t.Fatal("a later line for another slug extended chat's deadline")
	}
	if VerifyEntitlement("claw", pastLate) != nil {
		t.Fatal("claw line rejected within its own grace")
	}
	if VerifyEntitlement("chat", time.Unix(base.Unix()+grace+1, 0)) != nil {
		t.Fatal("the later chat line did not extend past the earlier one")
	}
}

// Line 1 of a v1.1 file is byte-for-byte the v1 file the CLI wrote for the
// same best token: a reader that takes line 1 gets today's decision.
func TestLineOneIsCompleteV1File(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	now := time.Now()
	both := lineToken(t, priv, []string{"chat", "claw"}, now.Add(time.Hour))
	claw := lineToken(t, priv, []string{"claw"}, now.Add(time.Hour))
	file := both + "\n" + claw
	line1, _, _ := strings.Cut(file, "\n")
	if line1 != both {
		t.Fatal("line 1 is not the first token")
	}
	setToken(t, line1)
	for _, slug := range []string{"chat", "claw"} {
		if err := VerifyEntitlement(slug, now); err != nil {
			t.Fatalf("v1 read of line 1 rejected %s: %v", slug, err)
		}
		if err := verifyToken(line1, slug, now); err != nil {
			t.Fatalf("line 1 alone does not verify for %s: %v", slug, err)
		}
	}
}

func TestGuardMultiLine(t *testing.T) {
	pub, priv := syntheticKey(t)
	_, other := syntheticKey(t)
	licensetest.UseKey(t, pub)
	now := time.Now()
	chat := lineToken(t, priv, []string{"chat"}, now.Add(time.Hour))
	claw := lineToken(t, priv, []string{"claw"}, now.Add(time.Hour))
	forged := lineToken(t, other, []string{"chat", "claw"}, now.Add(time.Hour))
	stale := lineToken(t, priv, []string{"chat"}, now.Add(-EntitlementGracePeriod-time.Hour))
	for _, tc := range []struct {
		name, slug string
		lines      []string
		want       int
	}{
		{"one-key", "chat", []string{chat}, 200},
		{"two-bundles-chat", "chat", []string{chat, claw}, 200},
		{"two-bundles-claw", "claw", []string{chat, claw}, 200},
		{"bad-signature-line-1", "chat", []string{forged, chat}, 200},
		{"torn-line-1-valid-line-2", "claw", []string{chat[:len(chat)/2], claw}, 200},
		{"expired-line-plus-valid", "chat", []string{stale, chat}, 200},
		{"forged-line-grants-nothing", "claw", []string{forged, chat}, 503},
		{"uncovered-slug", "paid", []string{chat, claw}, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setToken(t, strings.Join(tc.lines, "\n"))
			g := Guard(context.Background(), tc.slug, handler())
			defer g.Close()
			if got := serve(g, "/app").Code; got != tc.want {
				t.Fatalf("status %d, want %d", got, tc.want)
			}
		})
	}
}
