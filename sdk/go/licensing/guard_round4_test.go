package licensing

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/licensing/licensetest"
)

func TestVerifyEntitlementUnconfigured(t *testing.T) {
	t.Setenv(FileEnv, "")
	if err := os.Unsetenv(FileEnv); err != nil {
		t.Fatal(err)
	}
	if err := VerifyEntitlement("paid", time.Now()); !errors.Is(err, ErrEntitlementUnconfigured) {
		t.Fatalf("unset FileEnv returned %v", err)
	}
	t.Setenv(FileEnv, "")
	if err := VerifyEntitlement("paid", time.Now()); !errors.Is(err, ErrEntitlementExpired) {
		t.Fatalf("configured empty FileEnv returned %v", err)
	}
}

func TestGuardCompiledTestKeyWarning(t *testing.T) {
	setToken(t, "invalid")
	var logs bytes.Buffer
	g := Guard(context.Background(), "paid", handler(), WithLogger(log.New(&logs, "", 0)))
	defer g.Close()
	for i := 0; i < 3; i++ {
		serve(g, "/app")
	}
	want := 0
	if TestKeysCompiled() {
		want = 1
	}
	if got := strings.Count(logs.String(), "WARNING: nself_testkeys compiled in"); got != want {
		t.Fatalf("got %d compiled-key warnings, want %d: %s", got, want, logs.String())
	}
	if want == 1 && !strings.Contains(logs.String(), "UNSAFE FOR RELEASE") {
		t.Fatal("compiled-key warning did not identify unsafe release")
	}
}

func TestGuardAuthorizedRequestsUseReadLock(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	setToken(t, tokenFor(t, priv, header(), claimsAt(time.Now())))
	g := Guard(context.Background(), "paid", handler())
	defer g.Close()
	g.mu.RLock()
	done := make(chan int, 1)
	go func() { done <- serve(g, "/app").Code }()
	select {
	case status := <-done:
		g.mu.RUnlock()
		if status != 200 {
			t.Fatalf("authorized request returned %d", status)
		}
	case <-time.After(time.Second):
		g.mu.RUnlock()
		<-done
		t.Fatal("authorized request waited for an exclusive lock")
	}
}
