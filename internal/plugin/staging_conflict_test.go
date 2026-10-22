package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConflictScansIgnoreStaging(t *testing.T) {
	for _, name := range []string{".staging-thirdparty-123", "installed"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, name)
			if err := os.Mkdir(p, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(p, "plugin.json"), []byte(`{"name":"other","version":"1.0.0","description":"fixture","category":"utilities","license":"MIT","tables":["np_chat_messages"],"apiEndpoints":["chat.example.test/messages"]}`), 0644); err != nil {
				t.Fatal(err)
			}
			wantConflict := name == "installed"
			if err := checkTablePrefixConflict(dir, "chat", []string{"np_chat_messages"}); (err != nil) != wantConflict {
				t.Fatalf("prefix conflict: %v", err)
			}
			if err := checkTableConflicts(dir, &PluginManifest{Name: "chat", Tables: []string{"np_chat_messages"}}); (err != nil) != wantConflict {
				t.Fatalf("table conflict: %v", err)
			}
			routes := collectInstalledPluginRoutes(dir, "chat")
			if got := len(routes) > len(baseServiceRoutes); got != wantConflict {
				t.Fatalf("route conflict: %v", got)
			}
		})
	}
}
