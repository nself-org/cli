package licensing

import (
	"context"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/licensing/licensetest"
)

func TestGuardFailedSnapshotRetriesAfterMinute(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid", true: "missing"}[missing], func(t *testing.T) {
			pub, priv := syntheticKey(t)
			licensetest.UseKey(t, pub)
			now := time.Unix(1800000000, 0)
			clock := &fakeClock{now: now, ticker: &fakeTicker{ticks: make(chan time.Time), stopped: make(chan struct{})}}
			path := setToken(t, "invalid")
			if missing {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			g := Guard(context.Background(), "paid", handler(), WithClock(clock))
			defer g.Close()
			if serve(g, "/app").Code != 503 {
				t.Fatal("invalid snapshot accepted")
			}
			if err := os.WriteFile(path, []byte(tokenFor(t, priv, header(), claimsAt(now))), 0600); err != nil {
				t.Fatal(err)
			}
			clock.mu.Lock()
			clock.now = now.Add(59 * time.Second)
			clock.mu.Unlock()
			if serve(g, "/app").Code != 503 {
				t.Fatal("retry exceeded once per minute")
			}
			clock.mu.Lock()
			clock.now = now.Add(60 * time.Second)
			clock.mu.Unlock()
			if serve(g, "/app").Code != 200 {
				t.Fatal("valid replacement not recovered without ticker")
			}
			clock.mu.Lock()
			clock.now = now.Add(61 * time.Second)
			clock.mu.Unlock()
			if serve(g, "/app").Code != 200 {
				t.Fatal("replacement lost after 61 seconds without ticker")
			}
		})
	}
}

func TestEntitlementPolicyConstants(t *testing.T) {
	if EntitlementGracePeriod != 30*24*time.Hour || maxTokenBytes != 1024*1024 {
		t.Fatal("entitlement grace or bounded read contract changed")
	}
}

func TestGuardHealthMethodsAndPaths(t *testing.T) {
	setToken(t, "invalid")
	g := Guard(context.Background(), "paid", handler())
	defer g.Close()
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/health", 200}, {"HEAD", "/health", 200}, {"POST", "/health", 503},
		{"GET", "/HEALTH", 503}, {"GET", "/health/x", 503}, {"GET", "/health/../app", 503},
		{"GET", "//health", 503}, {"GET", "/health?x=1", 200},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			g.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("got %d, want %d", w.Code, tc.status)
			}
		})
	}
}

func TestPingKeysCommittedDefault(t *testing.T) {
	keys := PingKeys()
	if len(keys) != 1 || keys[0].ID != "1" || hex.EncodeToString(keys[0].Public) != "0ac4c2d7ec30d23bf2be55775e7170bd24a7544bad036d0d19f2a4deb516e00c" {
		t.Fatalf("unexpected committed keys: %+v", keys)
	}
}

func TestGuardCachesSignatureAndSlugUntilReload(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	setToken(t, tokenFor(t, priv, header(), claimsAt(time.Now())))
	g := Guard(context.Background(), "paid", handler())
	defer g.Close()
	replacement, _ := syntheticKey(t)
	licensetest.UseKey(t, replacement)
	if serve(g, "/app").Code != 200 {
		t.Fatal("cached snapshot reverified its signature on request")
	}
	g.reload()
	if serve(g, "/app").Code != 503 {
		t.Fatal("reload retained stale signature decision")
	}
}

func TestGuardCachedSignedExpiryBoundary(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	now := time.Unix(1800000000, 0)
	clock := &fakeClock{now: now, ticker: &fakeTicker{ticks: make(chan time.Time), stopped: make(chan struct{})}}
	setToken(t, tokenFor(t, priv, header(), claimsAt(now)))
	g := Guard(context.Background(), "paid", handler(), WithClock(clock))
	defer g.Close()
	for _, tc := range []struct {
		at     time.Time
		status int
	}{
		{now.Add(EntitlementGracePeriod), 200},
		{now.Add(EntitlementGracePeriod + time.Nanosecond), 503},
		{now.Add(EntitlementGracePeriod + time.Second), 503},
	} {
		clock.mu.Lock()
		clock.now = tc.at
		clock.mu.Unlock()
		if got := serve(g, "/app").Code; got != tc.status {
			t.Fatalf("at %s got %d, want %d", tc.at, got, tc.status)
		}
	}
}
