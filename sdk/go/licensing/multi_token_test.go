package licensing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/licensing/licensetest"
)

func TestMultilineEntitlementUsesMatchingMaximumDeadline(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	now := time.Now().Truncate(time.Second)
	var tokens []string
	for _, item := range []struct {
		slug string
		exp  time.Time
	}{{"other", now.Add(time.Hour)}, {"paid", now.Add(-EntitlementGracePeriod)}, {"paid", now.Add(time.Minute)}} {
		claims := claimsAt(item.exp)
		claims["plugins"] = []string{item.slug}
		tokens = append(tokens, tokenFor(t, priv, header(), claims))
	}
	tokens = append(tokens, "invalid.token.signature")
	setToken(t, strings.Join(tokens, "\n"))
	if err := VerifyEntitlement("paid", now.Add(time.Second)); err != nil {
		t.Fatalf("a matching valid line must authorize: %v", err)
	}
	g := Guard(context.Background(), "paid", handler())
	defer g.Close()
	if serve(g, "/app").Code != 200 || g.deadline != now.Add(time.Minute+EntitlementGracePeriod).Unix() {
		t.Fatalf("guard must use latest matching deadline; valid=%v deadline=%d", g.valid, g.deadline)
	}
	if err := VerifyEntitlement("missing", now); err != ErrEntitlementExpired {
		t.Fatal("uncovered slug was authorized")
	}
}
