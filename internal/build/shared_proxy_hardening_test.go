package build

// shared_proxy_hardening_test.go — P7-ADOPT-03 adversarial-review cases: names a
// plugin declares at http level, host-name folding, docs.<base>, confs left by a
// failed build, and registrations that name files the project does not own.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/sharedproxy"
)

func (x *sharedFx) geoPlugin(t *testing.T) {
	t.Helper()
	x.plugin(t, "geo", 3720)
	writeFixtureFile(t, filepath.Join(x.plugins, "geo", "nginx", "route.conf"), `limit_req_zone $binary_remote_addr zone=geo_ip:10m rate=5r/s;
upstream geo_backend {
    server geo:3720;
}
server {
    listen 80;
    server_name geo.${BASE_DOMAIN};
    location / {
        limit_req zone=geo_ip burst=5;
        proxy_pass http://geo_backend;
    }
}
`, 0o644)
}

// Two projects' copies of one plugin snippet must not declare one name twice in
// the owner's http{} context.
func TestSharedProxyDeclaredNamesPrefixed(t *testing.T) {
	x := newSharedFx(t)
	x.geoPlugin(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test"))
	var upstreams, zones int
	entries, _ := os.ReadDir(x.sites())
	for _, e := range entries {
		text := readText(t, filepath.Join(x.sites(), e.Name()))
		upstreams += strings.Count(text, "\nupstream geo_backend {") + strings.Count(text, "\nupstream bproj_geo_backend {")
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "limit_req_zone") {
				zones++
			}
		}
	}
	conf := readText(t, filepath.Join(x.sites(), "bproj.geo-route.conf"))
	for _, want := range []string{"upstream bproj_geo_backend {", "zone=bproj_geo_ip:10m", "limit_req zone=bproj_geo_ip", "proxy_pass http://bproj_geo_backend;", "server bproj_geo:3720;"} {
		if !strings.Contains(conf, want) {
			t.Errorf("fronted geo conf lacks %q:\n%s", want, conf)
		}
	}
	owner := readText(t, filepath.Join(x.sites(), "geo-route.conf"))
	if !strings.Contains(owner, "upstream geo_backend {") || strings.Contains(owner, "bproj_") {
		t.Errorf("owner geo conf must be untouched:\n%s", owner)
	}
	if upstreams != 2 || zones != 2 {
		t.Errorf("want two distinct upstream blocks and two zones, got %d and %d", upstreams, zones)
	}
}

// DNS ignores case and a trailing dot, so E504 must too.
func TestSharedProxyHostFolding(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	for _, target := range []string{"http://HASURA:8080", "http://Hasura", "http://hasura.:8080"} {
		err := x.build(t, x.backend(), x.frontedEnv("bproj", "b.test", "INTERNAL_ROUTE_1_NAME=legacy", "INTERNAL_ROUTE_1_SUBDOMAIN=legacy", "INTERNAL_ROUTE_1_TARGET="+target))
		wantCode(t, err, "E504", "hasura")
	}
}

// api-docs.conf serves docs.<base>: it is a registered name and a collision is refused.
func TestSharedProxyDocsName(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test"))
	set, _ := sharedproxy.Load(x.shared(), x.owner)
	if reg := set.Find("bproj"); reg == nil || !inSorted(reg.ServerNames, "docs.b.test") {
		t.Fatalf("docs.b.test is not registered: %+v", reg)
	}
	if o := readText(t, filepath.Join(x.shared(), sharedproxy.OwnerFile)); !strings.Contains(o, "docs.a.test") {
		t.Errorf("owner does not register docs.a.test:\n%s", o)
	}
	wantCode(t, x.build(t, x.other(), x.frontedEnv("cproj", "b.test")), "E503", "b.test")
}

// A failed build leaves live confs without a registration; they still count.
func TestSharedProxyOrphanedConfsStillClaim(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test"))
	if err := os.Remove(filepath.Join(x.shared(), "bproj.json")); err != nil { // as if the registration write had failed
		t.Fatal(err)
	}
	wantCode(t, x.build(t, x.other(), x.frontedEnv("cproj", "b.test")), "E503", "bproj", "does not list")
	// The project itself may rebuild over its own orphans.
	x.mustBuild(t, x.backend(), x.frontedEnv("bproj", "b.test"))
	if _, err := os.Stat(filepath.Join(x.shared(), "bproj.json")); err != nil {
		t.Fatalf("bproj did not re-register: %v", err)
	}
}

// A registration that lists a certificate pair the project did not create (the
// owner's own key) is not removable.
func TestSharedProxyForgedCertRegistration(t *testing.T) {
	x := newSharedFx(t)
	x.mustBuild(t, x.owner, x.ownerEnv())
	key := filepath.Join(x.owner, "ssl", "certificates", "a.test", "privkey.pem")
	writeFixtureFile(t, key, "OWNER KEY", 0o600)
	reg := sharedproxy.New("bproj", filepath.Join(x.owner, "backend"))
	reg.Files = []string{"ssl/certificates/a.test/privkey.pem"}
	if err := sharedproxy.Write(x.shared(), "bproj.json", reg); err != nil {
		t.Fatal(err)
	}
	if removed, err := sharedproxy.Unregister(x.owner, "bproj"); err != nil || len(removed) != 0 {
		t.Fatalf("Unregister removed %v (%v)", removed, err)
	}
	if _, err := os.Stat(key); err != nil {
		t.Fatalf("the owner's key was deleted: %v", err)
	}
}
