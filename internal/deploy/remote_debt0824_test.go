package deploy

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebt0824GuardedFailure(t *testing.T) {
	f := newFakeRemote(t)
	_, err := f.Run(context.Background(), shScript(guardedScript(t.TempDir(), []string{"first", "last"}, `[ "$1" != first ]`)))
	if err == nil {
		t.Fatal("an earlier action failure was hidden by the final action")
	}
}

func TestDebt0824OwnershipParser(t *testing.T) {
	for _, conf := range []string{
		"server { server_name b.example.org a.example.org; }\nserver_name a.example.org;\n",
		"# server_name ignored.example.org;\nserver_name a.example.org\n b.example.org;\n",
		"upstream api { server localhost:80; }\n",
	} {
		f := newFakeRemote(t)
		root := t.TempDir()
		const rel = "nginx/sites/api.conf"
		writeFile(t, filepath.Join(root, rel), markedConf+conf, 0o644)
		names := map[string][]string{rel: serverNameSet(conf)}
		if _, err := f.Run(context.Background(), shScript(frontOwnershipCheck(root, []string{rel}, names))); err != nil {
			t.Fatalf("ownership parser disagrees for %q: %v", conf, err)
		}
	}
}

func TestDebt0824BuildContextBase(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "fragment-only", true: "project-base"}[exists], func(t *testing.T) {
			f := newFakeRemote(t)
			stage, host := t.TempDir(), t.TempDir()
			writeFile(t, filepath.Join(stage, "compose.yml"), "services: {}\n", 0o644)
			writeFile(t, filepath.Join(stage, "plugins", "compose.yml"), "services:\n  app:\n    build: ./src\n", 0o644)
			base := filepath.Join(host, "plugins")
			if exists {
				base = host
			}
			writeFile(t, filepath.Join(base, "src", "Dockerfile"), "FROM scratch\n", 0o644)
			err := CheckBuildContexts(context.Background(), f, ArtifactSet{StageDir: stage, RemotePath: host, Compose: []string{host + "/compose.yml", host + "/plugins/compose.yml"}})
			if (err == nil) != exists {
				t.Fatalf("context must resolve from first Compose file: %v", err)
			}
		})
	}
}

func TestDebt0824MountDestination(t *testing.T) {
	for _, dst := range []string{"/tmp/nginx", "/etc/nginx-other"} {
		f := newFakeRemote(t)
		f.handler = func(string) (string, bool) { return "bind|/srv/web/nginx|" + dst + "\n", true }
		if err := (frontContainer{ID: "aaa1"}).checkMounts(context.Background(), f, "/srv/web", "/srv/web", nil); err == nil {
			t.Errorf("accepted nginx source mounted at %s", dst)
		}
	}
}

func TestDebt0824ShippedTempSymlink(t *testing.T) {
	f := newFakeRemote(t)
	root := t.TempDir()
	victim := filepath.Join(t.TempDir(), "keep")
	writeFile(t, victim, "untouched", 0o600)
	tmp := filepath.Join(root, ShippedFile) + ".new"
	if err := os.MkdirAll(filepath.Dir(tmp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, tmp); err != nil {
		t.Fatal(err)
	}
	if err := writeShipped(context.Background(), f, ArtifactSet{RemotePath: root, Files: []string{"compose.yml"}}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, victim); got != "untouched" {
		t.Fatalf("symlink target overwritten: %q", got)
	}
}

func TestDebt0824FrontFindErrors(t *testing.T) {
	f := newFakeRemote(t)
	front := t.TempDir()
	writeFile(t, filepath.Join(front, "ssl", "certificates", "cert", "fullchain.pem"), "pem", 0o600)
	bin := t.TempDir()
	writeFile(t, filepath.Join(bin, "find"), "#!/bin/sh\nexit 7\n", 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := TakeFrontSSL(context.Background(), f, front); err == nil {
		t.Fatal("find failure was treated as a complete certificate snapshot")
	}
}

func TestDebt0824FrontCertDirNotDirectory(t *testing.T) {
	f := newFakeRemote(t)
	front := t.TempDir()
	writeFile(t, filepath.Join(front, "ssl", "certificates"), "not a directory", 0o600)
	if _, err := TakeFrontSSL(context.Background(), f, front); err == nil {
		t.Fatal("an existing non-directory certificate path was treated as empty")
	}
}

func TestDebt0824FrontPruneRechecksNames(t *testing.T) {
	x := frontFixture(t)
	const rel = "nginx/sites/old.conf"
	host := x.hostSite(t, "old.conf", markedConf+"server_name old.example.org;\n")
	x.o.Set.Front.Previous = map[string][]string{"fake-host": {rel}}
	x.o.Set.Front.PreviousNames = map[string]map[string][]string{"fake-host": {rel: {"old.example.org"}}}
	previous := x.f.handler
	x.f.handler = func(script string) (string, bool) {
		if strings.Contains(script, "act()") && strings.Contains(script, "rm -f") && strings.Contains(script, rel) {
			writeFile(t, host, markedConf+"server_name sibling.example.org;\n", 0o644)
		}
		return previous(script)
	}
	_, _ = ApplyRemote(context.Background(), x.o)
	data, err := os.ReadFile(host)
	if err != nil || !strings.Contains(string(data), "sibling.example.org") {
		t.Fatalf("sibling file was deleted or restored over: %q, %v", data, err)
	}
}

func TestDebt0824FrontRollbackRechecksNames(t *testing.T) {
	for _, name := range []string{"api.conf", "new.conf"} {
		t.Run(name, func(t *testing.T) {
			x := frontFixture(t)
			res, err := ApplyRemote(context.Background(), x.o)
			if err != nil {
				t.Fatal(err)
			}
			host := x.hostSite(t, name, markedConf+"server_name sibling.example.org;\n")
			fb := &frontRollback{root: x.front}
			err = fb.restore(context.Background(), RollbackOptions{Remote: x.f, RemotePath: x.rp,
				FrontTar: res.FrontTar, FrontChanged: res.FrontChanged, FrontAbsent: res.FrontAbsent, FrontNames: res.FrontNames, Out: io.Discard})
			if err == nil {
				t.Error("rollback accepted another project's server_name set")
			}
			data, readErr := os.ReadFile(host)
			if readErr != nil || !strings.Contains(string(data), "sibling.example.org") {
				t.Fatalf("rollback changed sibling file: %q, %v", data, readErr)
			}
		})
	}
}
