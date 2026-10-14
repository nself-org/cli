package entitlement_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
)

func TestRefreshRefusesHardlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix link count")
	}
	compattest.Set(t, true)
	seedVerifiedCache(t)
	dir := t.TempDir()
	path := entitlement.Path(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(target, []byte("untouched"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, path); err != nil {
		t.Fatal(err)
	}
	if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
		t.Fatalf("hardlink accepted: %v", err)
	}
	data, _ := os.ReadFile(target)
	info, _ := os.Stat(target)
	if string(data) != "untouched" || info.Mode().Perm() != 0644 {
		t.Fatal("linked target changed")
	}
}

func TestRefreshRefusesSymlinkedDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges")
	}
	compattest.Set(t, true)
	seedVerifiedCache(t)
	for _, name := range []string{".nself", ".nself/state"} {
		t.Run(name, func(t *testing.T) {
			dir, target := t.TempDir(), t.TempDir()
			link := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(link), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			if err := entitlement.Refresh(context.Background(), dir); errs.ExitCodeFor(err) != 3 {
				t.Fatalf("directory symlink accepted: %v", err)
			}
			entries, _ := os.ReadDir(target)
			if len(entries) > 0 {
				t.Fatal("symlink directory target changed")
			}
		})
	}
}

func TestRefreshOfflineRetainsOverDifferentCacheBundle(t *testing.T) {
	compattest.Set(t, true)
	entry, priv := signedFixture(t, "synthetic-license-key", []string{"chat"}, time.Now().Add(time.Hour))
	installLicensed(t, "chat")
	dir := t.TempDir()
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	// A valid older token for another bundle cannot displace the installed token.
	parts := strings.Split(entry.JWT, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	claims["plugins"] = []string{"claw"}
	claims["exp"] = time.Now().Add(-31 * 24 * time.Hour).Unix()
	payload, _ = json.Marshal(claims)
	unsigned := parts[0] + "." + base64.RawURLEncoding.EncodeToString(payload)
	entry.JWT = unsigned + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
	entry.PluginsAllowed = []string{"claw"}
	body, _ := json.Marshal(map[string]any{"valid": true, "tier": "plus", "plugins_allowed": entry.PluginsAllowed, "jwt": entry.JWT})
	entry.RawBody = string(body)
	entry.BodySig = hex.EncodeToString(ed25519.Sign(priv, body))
	if err := license.WriteCache(entry); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(entitlement.Path(dir))
	if err := entitlement.Refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(entitlement.Path(dir))
	if string(after) != string(before) {
		t.Fatal("cache displaced runtime token")
	}
}
