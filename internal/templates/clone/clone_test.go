package clone

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAll_ReturnsSixTemplates verifies exactly six clone templates are bundled.
func TestAll_ReturnsSixTemplates(t *testing.T) {
	templates := All()
	if len(templates) != 6 {
		t.Errorf("expected 6 clone templates, got %d: %v", len(templates), templates)
	}
}

// TestAll_AllNonEmpty verifies no template name is empty.
func TestAll_AllNonEmpty(t *testing.T) {
	for _, name := range All() {
		if name == "" {
			t.Error("All() contains empty template name")
		}
	}
}

// TestIsCloneTemplate_Known verifies all documented templates are recognised.
func TestIsCloneTemplate_Known(t *testing.T) {
	known := []string{
		"airbnb-clone",
		"discord-clone",
		"notion-clone",
		"slack-clone",
		"substack-clone",
		"zoom-clone",
	}
	for _, name := range known {
		t.Run(name, func(t *testing.T) {
			if !IsCloneTemplate(name) {
				t.Errorf("IsCloneTemplate(%q) = false, want true", name)
			}
		})
	}
}

// TestIsCloneTemplate_Unknown verifies unknown names return false.
func TestIsCloneTemplate_Unknown(t *testing.T) {
	unknown := []string{"", "myapp", "twitter-clone", "custom-template"}
	for _, name := range unknown {
		t.Run(name, func(t *testing.T) {
			if IsCloneTemplate(name) {
				t.Errorf("IsCloneTemplate(%q) = true, want false", name)
			}
		})
	}
}

// TestGetManifest_AllBundled verifies that every bundled template has a
// readable, non-empty manifest.
func TestGetManifest_AllBundled(t *testing.T) {
	for _, name := range All() {
		t.Run(name, func(t *testing.T) {
			m, err := GetManifest(name)
			if err != nil {
				t.Fatalf("GetManifest(%q): %v", name, err)
			}
			if m.Name == "" {
				t.Errorf("manifest Name is empty for %q", name)
			}
			if m.Version == "" {
				t.Errorf("manifest Version is empty for %q", name)
			}
		})
	}
}

// TestGetManifest_Unknown verifies an unknown template returns an error.
func TestGetManifest_Unknown(t *testing.T) {
	_, err := GetManifest("does-not-exist")
	if err == nil {
		t.Error("GetManifest for unknown template should return error, got nil")
	}
}

// TestScaffold_CreatesFiles verifies that Scaffold creates files in the output
// directory.
func TestScaffold_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	templateName := All()[0] // use first available template

	opts := ScaffoldOptions{TargetDir: dir}
	written, err := Scaffold(templateName, opts)
	if err != nil {
		t.Fatalf("Scaffold(%q): %v", templateName, err)
	}
	if len(written) == 0 {
		t.Error("Scaffold wrote no files")
	}

	// The output dir should contain at least one file.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("Scaffold created no files in output dir")
	}
	_ = filepath.Join(dir, templateName) // ensure output dir structure is understood
}

// TestScaffold_DryRun verifies that DryRun returns file list without writing.
func TestScaffold_DryRun(t *testing.T) {
	dir := t.TempDir()
	templateName := All()[0]

	opts := ScaffoldOptions{TargetDir: dir, DryRun: true}
	written, err := Scaffold(templateName, opts)
	if err != nil {
		t.Fatalf("Scaffold DryRun: %v", err)
	}
	if len(written) == 0 {
		t.Error("Scaffold DryRun should return file list")
	}

	// In dry run, no files should be written.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("DryRun wrote files: %v", entries)
	}
}

// TestScaffold_InvalidTemplate verifies that an unknown template returns an error.
func TestScaffold_InvalidTemplate(t *testing.T) {
	dir := t.TempDir()
	opts := ScaffoldOptions{TargetDir: dir}
	_, err := Scaffold("does-not-exist", opts)
	if err == nil {
		t.Error("Scaffold with invalid template should return error, got nil")
	}
}

// TestScaffold_BackendGettingStarted guards the instructions and files actually
// delivered to users of every bundled clone template.
func TestScaffold_BackendGettingStarted(t *testing.T) {
	for _, name := range All() {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			files, err := Scaffold(name, ScaffoldOptions{TargetDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range files {
				if strings.HasPrefix(filepath.ToSlash(file), "flutter/") {
					t.Errorf("obsolete Flutter starter scaffolded: %s", file)
				}
			}
			data, err := os.ReadFile(filepath.Join(dir, "README.md"))
			if err != nil {
				t.Fatal(err)
			}
			readme := string(data)
			for _, want := range []string{"nself db hasura metadata apply\n", "nself add ", "No frontend starter is bundled"} {
				if !strings.Contains(readme, want) {
					t.Errorf("README missing %q", want)
				}
			}
			for _, obsolete := range []string{"nself hasura", "--file hasura/metadata.json", "nself plugin install", "flutter run", "Flutter starter with"} {
				if strings.Contains(readme, obsolete) {
					t.Errorf("README still contains %q", obsolete)
				}
			}
		})
	}
}
