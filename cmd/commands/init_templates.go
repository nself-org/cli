package commands

// Purpose: implements the app-clone and marketplace template scaffolding paths for `nself init`.
// Inputs: template name/slug, destination directory, and scaffolding flags (noSeed, dryRun, force, quiet).
// Outputs: a scaffolded project directory sourced from an embedded clone template or a downloaded marketplace template.
// Constraints: split out of init.go as a pure move (CLI-R12); the clone next-step hint
// prints only commands that exist (D-0127).

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/plugin/catalog"
	clonetemplate "github.com/nself-org/cli/internal/templates/clone"
	"github.com/nself-org/cli/internal/ui"
)

// runInitCloneTemplate scaffolds one of the six embedded app-clone templates
// into destDir. No network access is required.
func runInitCloneTemplate(name, destDir string, noSeed, dryRun, force, quiet bool) error {
	if !quiet {
		verb := "Scaffolding"
		if dryRun {
			verb = "Dry-run for"
		}
		ui.CommandHeader("nself init --template", fmt.Sprintf("%s clone template %q", verb, name))
	}

	m, err := clonetemplate.GetManifest(name)
	if err != nil {
		return fmt.Errorf("loading template manifest: %w", err)
	}

	if err := clonetemplate.Enforce(os.Stderr, name, m.RequiredPlugins, templateSource); err != nil {
		return err
	}

	// Resolve destination directory.
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolving destination: %w", err)
	}

	if !dryRun {
		if !force {
			if entries, readErr := os.ReadDir(absDest); readErr == nil && len(entries) > 0 {
				return fmt.Errorf("destination %s is not empty; use --force to overwrite", absDest)
			}
		}
		if mkErr := os.MkdirAll(absDest, 0755); mkErr != nil {
			return fmt.Errorf("creating destination directory: %w", mkErr)
		}
	}

	written, scaffoldErr := clonetemplate.Scaffold(name, clonetemplate.ScaffoldOptions{
		TargetDir: absDest,
		NoSeed:    noSeed,
		DryRun:    dryRun,
	})
	if scaffoldErr != nil {
		return fmt.Errorf("scaffolding template: %w", scaffoldErr)
	}

	if dryRun {
		fmt.Printf("\n  %s Files that would be written to %s:\n\n", ui.C(ui.Blue, ui.IconInfo), absDest)
		for _, f := range written {
			fmt.Printf("    %s\n", f)
		}
		fmt.Printf("\n  %d file(s). Run without --dry-run to write them.\n\n", len(written))
		return nil
	}

	if !quiet {
		fmt.Printf("\n  %s Template %q scaffolded into %s\n\n",
			ui.C(ui.Green, ui.IconSuccess), name, absDest)
		fmt.Printf("  Files written: %d\n", len(written))
		if len(m.RequiredPlugins) > 0 {
			fmt.Printf("  Required plugins: %s\n", strings.Join(m.RequiredPlugins, ", "))
			fmt.Printf("\n  Install plugins:\n")
			fmt.Printf("    %s\n", ui.C(ui.Cyan, fmt.Sprintf("nself plugin install %s", strings.Join(m.RequiredPlugins, " "))))
		}
		printCloneNextSteps(os.Stdout, absDest, m.RequiredPlugins)
	}
	return nil
}

// templateSource is the catalog and licence source of the template
// entitlement gate (P7-PLUG-25). Tests replace it; the default needs the network.
var templateSource clonetemplate.Source = func(ctx context.Context) (clonetemplate.Lookup, clonetemplate.Entitled, error) {
	res, err := catalog.Fetch(ctx, catalogURL, catalog.DefaultCacheDir())
	if err != nil {
		return nil, nil, err
	}
	find := func(slug string) (clonetemplate.PluginInfo, bool) {
		e, ok := res.Catalog.Lookup(slug)
		return clonetemplate.PluginInfo{Slug: e.Slug, Tier: e.Tier, Bundles: e.Bundles}, ok
	}
	entitled := func(ctx context.Context, bundle string) (bool, error) {
		if key := license.CollectLicenseKey(); key != "" { // no key: nothing is covered, no licence call
			return license.BundleEntitled(ctx, key, bundle)
		}
		return false, nil
	}
	return find, entitled, nil
}

// printCloneNextSteps writes the commands that bring a scaffolded clone
// template up. Metadata goes through `nself db hasura metadata apply`, which
// reads hasura/metadata.json from the project directory (there is no --file
// flag). The flutter/ starter is legacy (Flutter is eliminated for new
// surfaces), so it is pointed at, never run.
func printCloneNextSteps(w io.Writer, dest string, plugins []string) {
	_, _ = fmt.Fprintf(w, "\n  Next steps:\n")
	_, _ = fmt.Fprintf(w, "    cd %s\n", dest)
	_, _ = fmt.Fprintf(w, "    nself start\n")
	_, _ = fmt.Fprintf(w, "    nself db migrate\n")
	_, _ = fmt.Fprintf(w, "    nself db hasura metadata apply\n")
	if len(plugins) > 0 {
		_, _ = fmt.Fprintf(w, "    nself plugin install %s\n", strings.Join(plugins, " "))
	}
	_, _ = fmt.Fprintf(w, "\n  The flutter/ folder holds a legacy starter app; see README.md.\n\n")
}

// runInitMarketplaceTemplate downloads a marketplace template tarball, verifies
// its SHA256 checksum, and extracts it into destDir.
func runInitMarketplaceTemplate(slug, destDir string, force, quiet bool) error {
	if !quiet {
		ui.CommandHeader("nself init --template", fmt.Sprintf("Installing marketplace template %q", slug))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	baseURL := resolveTemplateRegistryURL()
	t, err := fetchTemplateSingle(ctx, baseURL, slug)
	if err != nil {
		return fmt.Errorf("template %q not found in registry.\n"+
			"Browse available templates: nself template list\n"+
			"Or visit: https://nself.org/templates\n"+
			"Original error: %w", slug, err)
	}

	// Resolve destination directory.
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolving destination: %w", err)
	}
	if !force {
		if entries, readErr := os.ReadDir(absDest); readErr == nil && len(entries) > 0 {
			return fmt.Errorf("destination %s is not empty; use --force to overwrite", absDest)
		}
	}
	if mkErr := os.MkdirAll(absDest, 0755); mkErr != nil {
		return fmt.Errorf("creating destination directory: %w", mkErr)
	}

	// License check for paid templates.
	if t.PriceUSD > 0 {
		licenseKey := os.Getenv("NSELF_LICENSE_KEY")
		if licenseKey == "" {
			return fmt.Errorf(
				"template %q costs $%.2f and requires a valid license.\n"+
					"Set your license key: nself license set nself_pro_<your-key>\n"+
					"Get a license at: https://nself.org/pricing",
				slug, t.PriceUSD,
			)
		}
	}

	if !quiet {
		ui.Info(fmt.Sprintf("Downloading %s v%s...", t.DisplayName, t.TemplateVersion))
	}

	// Download tarball.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.TarballURL, nil)
	if err != nil {
		return fmt.Errorf("preparing download request: %w", err)
	}
	req.Header.Set("User-Agent", "nself-cli")
	if licKey := os.Getenv("NSELF_LICENSE_KEY"); licKey != "" {
		req.Header.Set("X-Nself-License", licKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading template tarball: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("access denied for template %q — check your license key", slug)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Buffer the body so we can verify SHA256 before extraction.
	tarData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading tarball: %w", err)
	}

	// Verify SHA256.
	if t.TarballSHA256 != "" {
		h := sha256.New()
		h.Write(tarData)
		got := hex.EncodeToString(h.Sum(nil))
		if got != t.TarballSHA256 {
			return fmt.Errorf(
				"SHA256 mismatch for template %q\n  expected: %s\n  got:      %s\nthe download may be corrupted or tampered with",
				slug, t.TarballSHA256, got,
			)
		}
		if !quiet {
			ui.Success("Checksum verified.")
		}
	}

	// Extract tarball into destDir.
	if !quiet {
		ui.Info(fmt.Sprintf("Extracting into %s...", absDest))
	}

	gr, err := gzip.NewReader(strings.NewReader(string(tarData)))
	if err != nil {
		return fmt.Errorf("opening gzip stream: %w", err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	for {
		header, tarErr := tr.Next()
		if tarErr == io.EOF {
			break
		}
		if tarErr != nil {
			return fmt.Errorf("reading tarball entry: %w", tarErr)
		}

		// Guard against path traversal.
		target := filepath.Join(absDest, filepath.Clean("/" + header.Name)[1:])
		if !strings.HasPrefix(target, absDest+string(os.PathSeparator)) && target != absDest {
			return fmt.Errorf("illegal path in tarball: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if mkErr := os.MkdirAll(target, 0755); mkErr != nil {
				return fmt.Errorf("creating directory %s: %w", target, mkErr)
			}
		case tar.TypeReg:
			if mkErr := os.MkdirAll(filepath.Dir(target), 0755); mkErr != nil {
				return fmt.Errorf("creating parent directory: %w", mkErr)
			}
			f, createErr := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if createErr != nil {
				return fmt.Errorf("creating file %s: %w", target, createErr)
			}
			if _, copyErr := io.Copy(f, tr); copyErr != nil {
				_ = f.Close()
				return fmt.Errorf("writing file %s: %w", target, copyErr)
			}
			_ = f.Close()
		}
	}

	if !quiet {
		ui.Success(fmt.Sprintf("Template %q installed in %s", slug, absDest))
		fmt.Println()
		if len(t.RequiredPlugins) > 0 {
			fmt.Printf("Required plugins: %s\n", strings.Join(t.RequiredPlugins, ", "))
			fmt.Printf("Install with: nself plugin install %s\n", strings.Join(t.RequiredPlugins, " "))
			fmt.Println()
		}
		fmt.Println("Next steps:")
		fmt.Printf("  cd %s\n", destDir)
		fmt.Println("  nself start")
	}

	return nil
}
