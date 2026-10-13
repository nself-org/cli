package sharedproxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sample(project, dir string) Registration {
	r := New(project, dir)
	r.Files = []string{"nginx/sites/" + project + ".b.conf", "nginx/sites/" + project + ".a.conf", "nginx/sites/" + project + ".a.conf"}
	r.ServerNames = []string{"z." + project + ".test", "a." + project + ".test"}
	return r
}

func TestMarshalDeterministic(t *testing.T) {
	r := sample("b", "/own/b")
	one, err := Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Files[0], r.Files[1] = r.Files[1], r.Files[0]
	two, _ := Marshal(r)
	if string(one) != string(two) {
		t.Fatalf("not deterministic:\n%s\n%s", one, two)
	}
	text := string(one)
	if !strings.HasPrefix(text, "{\n  \"_generated\": ") || strings.Count(text, "nginx/sites/b.a.conf") != 1 {
		t.Fatalf("marker not first or list not de-duplicated:\n%s", text)
	}
	if !strings.Contains(text, `"service_names": []`) || !strings.Contains(text, `"network": "b_network"`) {
		t.Fatalf("empty list or network wrong:\n%s", text)
	}
	if strings.Index(text, `"files"`) > strings.Index(text, `"network"`) || strings.Index(text, `"project_dir"`) > strings.Index(text, `"schema_version"`) {
		t.Fatalf("keys not sorted:\n%s", text)
	}
}

func TestParseRefusals(t *testing.T) {
	owner := "/own"
	good, _ := Marshal(sample("b", "/own/b"))
	if _, err := Parse("b.json", good, owner); err != nil {
		t.Fatalf("valid registration refused: %v", err)
	}
	for name, c := range map[string]struct{ file, from, to, want string }{
		"file name":     {"c.json", "", "", "file name must equal b.json"},
		"outside dir":   {"b.json", `"/own/b"`, `"/elsewhere/b"`, "not under the owner dir"},
		"traversal":     {"b.json", "nginx/sites/b.a.conf", "nginx/sites/../../etc/x", "is not nginx/sites"},
		"absolute":      {"b.json", "nginx/sites/b.a.conf", "/etc/passwd", "is not nginx/sites"},
		"wrong version": {"b.json", `"schema_version": "1"`, `"schema_version": "2"`, "schema_version"},
		"unknown key":   {"b.json", `"project": "b"`, `"project": "b", "extra": 1`, "unknown field"},
	} {
		data := good
		if c.from != "" {
			data = []byte(strings.Replace(string(good), c.from, c.to, 1))
		}
		if _, err := Parse(c.file, data, owner); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, c.want)
		}
	}
}

func TestLoadSet(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "nginx", "shared")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if set, err := Load(filepath.Join(root, "missing"), root); err != nil || set.Owner != nil || len(set.Projects) != 0 {
		t.Fatalf("missing dir should be empty: %+v %v", set, err)
	}
	write := func(name string, r Registration) {
		data, _ := Marshal(r)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(OwnerFile, New("own", root))
	write("b.json", sample("b", filepath.Join(root, "b")))
	write("c.json", sample("c", filepath.Join(root, "c")))
	set, err := Load(dir, root)
	if err != nil {
		t.Fatal(err)
	}
	if set.Owner == nil || len(set.Projects) != 2 || set.Find("c") == nil || len(set.Others("b")) != 1 {
		t.Fatalf("unexpected set %+v", set)
	}
	if !set.RegisteredFiles()["nginx/sites/c.a.conf"] || set.RegisteredFiles()["nginx/sites/own.conf"] {
		t.Fatalf("registered files wrong: %v", set.RegisteredFiles())
	}
	// A broken file fails the whole read (fail closed).
	if err := os.WriteFile(filepath.Join(dir, "d.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, root); err == nil {
		t.Fatal("a malformed registration must fail Load")
	}
}

func TestUnregister(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "nginx", "shared")
	put := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b := sample("b", filepath.Join(root, "b"))
	b.Files = []string{"nginx/sites/b.a.conf", "ssl/certificates/b-test/fullchain.pem", "ssl/certificates/acme/fullchain.pem"}
	c := sample("c", filepath.Join(root, "c"))
	c.Files = []string{"nginx/sites/c.x.conf"}
	if err := Write(dir, FileName("b"), b); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, FileName("c"), c); err != nil {
		t.Fatal(err)
	}
	put("nginx/sites/b.a.conf", "x")
	put("nginx/sites/c.x.conf", "x")
	put("ssl/certificates/b-test/fullchain.pem", "x")
	gen := filepath.Join(root, "ssl", "certificates", ".acme.gen-1")
	put("ssl/certificates/.acme.gen-1/fullchain.pem", "keep")
	if err := os.Symlink(gen, filepath.Join(root, "ssl", "certificates", "acme")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	removed, err := Unregister(root, "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed %v, want the conf and the cert (not the symlinked pair)", removed)
	}
	for rel, want := range map[string]bool{"nginx/sites/b.a.conf": false, "nginx/sites/c.x.conf": true, "ssl/certificates/b-test": false, "ssl/certificates/.acme.gen-1/fullchain.pem": true, "nginx/shared/b.json": false, "nginx/shared/c.json": true} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); (err == nil) != want {
			t.Errorf("%s exists=%v, want %v", rel, err == nil, want)
		}
	}
	if removed, err := Unregister(root, "b"); err != nil || removed != nil {
		t.Fatalf("a second Unregister must be a no-op: %v %v", removed, err)
	}
}

func TestParseUnderSymlinkedOwner(t *testing.T) {
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	data, _ := Marshal(sample("b", filepath.Join(real, "b")))
	if _, err := Parse("b.json", data, link); err != nil {
		t.Fatalf("owner reached through a symlink: %v", err)
	}
	if _, err := Parse("b.json", data, filepath.Join(t.TempDir(), "elsewhere")); err == nil {
		t.Fatal("a project_dir outside the owner must still be refused")
	}
}

func TestValidShapes(t *testing.T) {
	for rel, want := range map[string]bool{
		"nginx/sites/b.a.conf":                  true,
		"ssl/certificates/b-test/fullchain.pem": true,
		"ssl/certificates/b-test/privkey.pem":   true,
		"ssl/certificates/fullchain.pem":        false, // would remove the whole certificates dir
		"ssl/certificates/b-test/other.pem":     false,
		"nginx/sites/sub/x.conf":                false,
		"nginx/sites":                           false,
		"nginx/shared/_owner.json":              false,
		"ssl/.acme/lineages.json":               false,
	} {
		if ValidFile(rel) != want {
			t.Errorf("ValidFile(%q) = %v, want %v", rel, !want, want)
		}
	}
	for name, want := range map[string]bool{"bproj": true, "b.c-d_e": true, "_owner": false, "": false, "../x": false, "a/b": false, ".hid": false, "a..b": false} {
		if ValidProject(name) != want {
			t.Errorf("ValidProject(%q) = %v, want %v", name, !want, want)
		}
	}
	// A project registration may only list its own site files (an owner conf is refused at read time).
	r := sample("b", "/own/b")
	r.Files = []string{"nginx/sites/auth.conf"}
	data, _ := Marshal(r)
	if _, err := Parse("b.json", data, "/own"); err == nil || !strings.Contains(err.Error(), "not one of this project's") {
		t.Errorf("a registration listing the owner's conf must be refused: %v", err)
	}
	r = sample("_owner", "/own/b")
	data, _ = Marshal(r)
	if _, err := Parse("_owner.json", data, "/own"); err != nil {
		t.Logf("owner file parse: %v", err)
	}
}

func TestRemovableRefusesOtherProjectsFile(t *testing.T) {
	root := t.TempDir()
	set := Set{Projects: []Registration{{Project: "b", Files: []string{"nginx/sites/b.x.conf"}}, {Project: "c", Files: []string{"nginx/sites/b.x.conf"}}}}
	if set.Removable(root, "b", "nginx/sites/b.x.conf") {
		t.Fatal("a path another registration lists must not be removable")
	}
	if !set.Removable(root, "c", "nginx/sites/c.y.conf") {
		t.Fatal("a valid path no other project lists is removable")
	}
}
