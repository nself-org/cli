package plugin

import (
	"context"
	"strings"
	"testing"
)

func TestSignatureBypassCallerAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name, env, skip, legacy string
		ack, allowed, denied    bool
	}{
		{"dev_acknowledged", "dev", "1", "", true, true, false},
		{"dev_unacknowledged", "dev", "1", "", false, false, true},
		{"dev_legacy", "dev", "1", "1", false, true, false},
		{"prod_denied", "prod", "1", "", true, false, true},
		{"staging_denied", "staging", "1", "", true, false, true},
		{"force_alone_dev", "dev", "", "", true, false, false},
		{"force_alone_prod", "prod", "", "", true, false, false},
		{"force_alone_staging", "staging", "", "", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NSELF_LICENSE_SKIP_VERIFY", tc.skip)
			t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", tc.legacy)
			ctx := WithSignatureBypassAcknowledged(context.Background(), tc.ack)
			allowed, err := checkSigBypassAllowedWithContext(ctx, tc.env, "fixture")
			if allowed != tc.allowed || (err != nil) != tc.denied {
				t.Fatalf("got (%v, %v), want allowed=%v denied=%v", allowed, err, tc.allowed, tc.denied)
			}
			if tc.denied && tc.env != "dev" && !strings.HasPrefix(err.Error(), "SECURITY:") {
				t.Fatalf("bypass denial must retain SECURITY prefix: %v", err)
			}
		})
	}
}

func TestSignatureBypassAcknowledgementScoped(t *testing.T) {
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "")
	parent := context.Background()
	child := WithSignatureBypassAcknowledged(parent, true)
	if allowed, err := checkSigBypassAllowedWithContext(child, "dev", "fixture"); !allowed || err != nil {
		t.Fatalf("acknowledged child: %v, %v", allowed, err)
	}
	for _, ctx := range []context.Context{parent, WithSignatureBypassAcknowledged(child, false)} {
		if allowed, err := checkSigBypassAllowedWithContext(ctx, "dev", "fixture"); allowed || err == nil {
			t.Fatalf("unacknowledged context inherited consent: %v, %v", allowed, err)
		}
	}
}
