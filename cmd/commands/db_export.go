package commands

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/exporter"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

// Purpose: register read-only portable export. Inputs: output/schemas flags.
// Outputs: summary envelope. Constraints: no secret values in diagnostics.
var dbExportCmd = &cobra.Command{Use: "export", Short: "Export a portable bundle from a started project", Args: cobra.NoArgs, RunE: runDBExport}
var exportProject = exporter.Export

func init() {
	f := dbExportCmd.Flags()
	f.String("out", "", "New or empty output directory (required)")
	f.StringSlice("schemas", nil, "Schemas to export (default all user schemas)")
	f.Bool("no-storage", false, "Omit user storage objects")
	f.Bool("json", false, "Emit a v1 JSON envelope")
	_ = dbExportCmd.MarkFlagRequired("out")
	dbCmd.AddCommand(dbExportCmd)
}

func runDBExport(cmd *cobra.Command, _ []string) error {
	dir, err := resolveProjectDir()
	if err != nil {
		return err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("export: project config could not be resolved")
	}
	out, _ := cmd.Flags().GetString("out")
	schemas, _ := cmd.Flags().GetStringSlice("schemas")
	skip, _ := cmd.Flags().GetBool("no-storage")
	values, keys := dbExportSecretValues(os.Environ())
	// Defaults do not necessarily appear as environment variables.
	for _, v := range []string{cfg.Postgres.Password, cfg.Hasura.AdminSecret, cfg.Hasura.JWTKey, cfg.Auth.SMTPPass, cfg.Minio.RootUser, cfg.Minio.RootPassword, cfg.Minio.S3AccessKey, cfg.Minio.S3SecretKey} {
		if len(v) >= 6 {
			values = append(values, v)
		}
	}
	summary, err := exportProject(cmd.Context(), cfg, exporter.Options{Out: out, Schemas: schemas, NoStorage: skip, SecretValues: values, Keys: keys, ProjectDir: dir})
	if err != nil {
		return err
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return output.EmitData(output.Default(), "db export", summary)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Exported %d tables, %d users and %d objects to %s\n%s\n", summary.Tables, summary.Users, summary.Objects, summary.Out, summary.StorageNote)
	return err
}

// Purpose: collect the widened secret class from the resolved environment.
// Inputs: KEY=value pairs. Outputs: sorted distinct values and names only.
// Constraints: URL userinfo passwords count even on an ordinary URL key.
func dbExportSecretValues(env []string) ([]string, []string) {
	values, names := map[string]bool{}, map[string]bool{}
	for _, pair := range env {
		key, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		names[key] = true
		secret := config.IsSecretKey(key)
		for _, term := range []string{"SECRET", "PASSWORD", "PASS", "KEY", "TOKEN", "CREDENTIAL", "PRIVATE", "DSN"} {
			if strings.Contains(strings.ToUpper(key), term) {
				secret = true
			}
		}
		if secret && len(v) >= 6 {
			values[v] = true
		}
		if u, err := url.Parse(v); err == nil && u.User != nil {
			if p, ok := u.User.Password(); ok && len(p) >= 6 {
				values[p] = true
			}
		}
	}
	vs, ks := []string{}, []string{}
	for v := range values {
		vs = append(vs, v)
	}
	for k := range names {
		ks = append(ks, k)
	}
	sort.Strings(vs)
	sort.Strings(ks)
	return vs, ks
}
