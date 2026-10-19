package acme

// served_links_test.go: ResolveServedLinks follows a served top-level symlink
// only onto certificates/<name> and refuses every escape (D-1214).

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const servedSSL = "/etc/nginx/ssl/"

// linkTree builds an ssl dir with certificates/<name> dirs and returns it plus a
// directory outside it that escape links point at.
func linkTree(t *testing.T, names ...string) (ssl, outside string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	root := t.TempDir()
	ssl, outside = filepath.Join(root, "ssl"), filepath.Join(root, "outside")
	for _, d := range append([]string{outside}, names...) {
		if !filepath.IsAbs(d) {
			d = filepath.Join(ssl, "certificates", d)
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return ssl, outside
}

func link(t *testing.T, to, at string) {
	t.Helper()
	if err := os.Symlink(to, at); err != nil {
		t.Fatal(err)
	}
}

func TestACMEResolveServedLinks(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, ssl, outside string)
		cert  string
		want  string // "" = unchanged
		note  string // substring of the single note, "" = no note
	}{
		{"relative link to certificates/<name>", func(t *testing.T, ssl, _ string) {
			link(t, "certificates/api-task", filepath.Join(ssl, "api-task"))
		}, servedSSL + "api-task/fullchain.pem", servedSSL + "certificates/api-task/fullchain.pem", "adopting that directory"},
		{"relative link with ./ and trailing slash", func(t *testing.T, ssl, _ string) {
			link(t, "./certificates/api-task/", filepath.Join(ssl, "api-task"))
		}, servedSSL + "api-task/privkey.pem", servedSSL + "certificates/api-task/privkey.pem", "adopting"},
		{"absolute link inside the ssl dir", func(t *testing.T, ssl, _ string) {
			link(t, filepath.Join(ssl, "certificates", "api-task"), filepath.Join(ssl, "api-task"))
		}, servedSSL + "api-task/fullchain.pem", servedSSL + "certificates/api-task/fullchain.pem", "adopting"},
		{"absolute link written for the container", func(t *testing.T, ssl, _ string) {
			link(t, "/etc/nginx/ssl/certificates/api-task", filepath.Join(ssl, "api-task"))
		}, servedSSL + "api-task/fullchain.pem", servedSSL + "certificates/api-task/fullchain.pem", "adopting"},
		{"certificates/<name> is a sibling generation link (state after adoption)", func(t *testing.T, ssl, _ string) {
			link(t, "certificates/api-task", filepath.Join(ssl, "api-task"))
			gens := filepath.Join(ssl, "certificates")
			if err := os.Remove(filepath.Join(gens, "api-task")); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(gens, "api-task.gen-3"), 0o755); err != nil {
				t.Fatal(err)
			}
			link(t, "api-task.gen-3", filepath.Join(gens, "api-task"))
		}, servedSSL + "api-task/fullchain.pem", servedSSL + "certificates/api-task/fullchain.pem", "adopting"},
		{"plain directory is left alone", func(t *testing.T, ssl, _ string) {
			if err := os.Mkdir(filepath.Join(ssl, "api-task"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, servedSSL + "api-task/fullchain.pem", "", ""},
		{"path already under certificates", func(t *testing.T, _, _ string) {}, servedSSL + "certificates/x/fullchain.pem", "", ""},
		{"path outside the ssl mount", func(t *testing.T, _, _ string) {}, "/etc/other/x/fullchain.pem", "", ""},
		{"missing entry", func(t *testing.T, _, _ string) {}, servedSSL + "ghost/fullchain.pem", "", ""},
		{"escape: absolute link outside the ssl dir", func(t *testing.T, ssl, outside string) {
			link(t, outside, filepath.Join(ssl, "api-task"))
		}, servedSSL + "api-task/fullchain.pem", "", "outside the ssl directory"},
		{"escape: relative link out with ..", func(t *testing.T, ssl, _ string) {
			link(t, "../outside", filepath.Join(ssl, "api-task"))
		}, servedSSL + "api-task/fullchain.pem", "", "not certificates/<name>"},
		{"escape: dotdot hidden after certificates", func(t *testing.T, ssl, _ string) {
			link(t, "certificates/../../outside", filepath.Join(ssl, "api-task"))
		}, servedSSL + "api-task/fullchain.pem", "", "not certificates/<name>"},
		{"escape: certificates/<name> is itself a link out", func(t *testing.T, ssl, outside string) {
			link(t, "certificates/api-task", filepath.Join(ssl, "api-task"))
			if err := os.Remove(filepath.Join(ssl, "certificates", "api-task")); err != nil {
				t.Fatal(err)
			}
			link(t, outside, filepath.Join(ssl, "certificates", "api-task"))
		}, servedSSL + "api-task/fullchain.pem", "", "resolves outside certificates/"},
		{"escape: certificates dir is a symlink", func(t *testing.T, ssl, outside string) {
			link(t, "certificates/api-task", filepath.Join(ssl, "api-task"))
			if err := os.RemoveAll(filepath.Join(ssl, "certificates")); err != nil {
				t.Fatal(err)
			}
			link(t, outside, filepath.Join(ssl, "certificates"))
		}, servedSSL + "api-task/fullchain.pem", "", "certificates/ is itself a symlink"},
		{"chain: link to another top-level link", func(t *testing.T, ssl, _ string) {
			link(t, "other", filepath.Join(ssl, "api-task"))
			link(t, "certificates/api-task", filepath.Join(ssl, "other"))
		}, servedSSL + "api-task/fullchain.pem", "", "not certificates/<name>"},
		{"link to the ssl dir root", func(t *testing.T, ssl, _ string) {
			link(t, ".", filepath.Join(ssl, "api-task"))
		}, servedSSL + "api-task/fullchain.pem", "", "not certificates/<name>"},
		{"link to certificates itself", func(t *testing.T, ssl, _ string) {
			link(t, "certificates", filepath.Join(ssl, "api-task"))
		}, servedSSL + "api-task/fullchain.pem", "", "not certificates/<name>"},
		{"nginx variable paths are never touched", func(t *testing.T, _, _ string) {}, servedSSL + "$host/fullchain.pem", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ssl, outside := linkTree(t, "api-task")
			c.setup(t, ssl, outside)
			hosts := map[string]string{"h.example.org": c.cert}
			got, notes := ResolveServedLinks(ssl, hosts)
			want := c.want
			if want == "" {
				want = c.cert
			}
			if got["h.example.org"] != want {
				t.Errorf("resolved %q, want %q", got["h.example.org"], want)
			}
			if hosts["h.example.org"] != c.cert {
				t.Errorf("input map was modified: %v", hosts)
			}
			if (c.note == "") != (len(notes) == 0) || c.note != "" && !strings.Contains(notes[0], c.note) {
				t.Errorf("notes %v, want one containing %q", notes, c.note)
			}
		})
	}
}

// Two hosts behind one link produce one note, and the planner then adopts a
// lineage whose conf named the link; an escaping link still refuses it.
func TestACMEPlanThroughServedLink(t *testing.T) {
	ssl, outside := linkTree(t, "api-task", "auth-task")
	link(t, "certificates/api-task", filepath.Join(ssl, "api-task"))
	link(t, outside, filepath.Join(ssl, "auth-task"))
	hosts := map[string]string{
		"api.task.example.org":  servedSSL + "api-task/fullchain.pem",
		"api2.task.example.org": servedSSL + "api-task/fullchain.pem",
		"auth.task.example.org": servedSSL + "auth-task/fullchain.pem",
	}
	cbs := []Certbot{
		{Name: "api", Authenticator: "dns-cloudflare", Domains: []string{"api.task.example.org", "api2.task.example.org"}},
		{Name: "auth", Authenticator: "dns-cloudflare", Domains: []string{"auth.task.example.org"}},
	}
	// before: both refused
	if ad, refused := Plan(cbs, hosts, "cloudflare", ""); len(ad) != 0 || len(refused) != 2 {
		t.Fatalf("unresolved links must refuse both: %+v %v", ad, refused)
	}
	resolved, notes := ResolveServedLinks(ssl, hosts)
	if len(notes) != 2 {
		t.Errorf("want one note per link, got %v", notes)
	}
	ad, refused := Plan(cbs, resolved, "cloudflare", "")
	if len(ad) != 1 || ad[0].Name != "api" || len(ad[0].Targets) != 1 || ad[0].Targets[0] != "certificates/api-task" {
		t.Fatalf("the lineage behind the safe link must adopt certificates/api-task: %+v", ad)
	}
	if len(refused) != 1 || !strings.HasPrefix(refused[0], "auth: ") || !strings.Contains(refused[0], "auth-task") {
		t.Fatalf("the escaping link must stay refused: %v", refused)
	}
}
