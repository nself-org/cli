package commands

// Purpose: `nself db generate vector-search <schema>.<table>.<column>` (v1.4
// spelling `nself generate vector-search`) writes a pgvector k-NN SQL function
// migration and, with --track, tracks it through Hasura for the named roles
// (P7-ADOPT-07, EPIC ADOPT D7, ADR 0003).
// Inputs: the target, --distance, --roles, --track, --json, --migration-dir.
// Outputs: a human summary or one v1 envelope (vectorsearch.Result).
// Constraints: apps reach the function over GraphQL only; no Postgres role,
// grant or connection string is created; metadata is never replaced or applied.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/database/vectorsearch"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

var generateVectorSearchCmd = &cobra.Command{
	Use:   "vector-search <schema>.<table>.<column>",
	Short: "Generate a k-NN search function for a vector column and track it in Hasura",
	Long: `Write migrations/<timestamp>_vector_search_<table>_<column>.sql: a STABLE SQL
function <schema>.<table>_nearest_<column>(query vector(n)) that returns the
table's rows ordered by distance. It has no LIMIT: Hasura applies the GraphQL
limit after the table's select permission filter.

Apply the migration with 'nself db migrate up --migration-dir migrations', then run
the command again with --track to track the function with a permission per role.
--track sends one targeted Hasura call (never replace_metadata) and updates the
on-disk metadata only when it matches live Hasura.

Examples:
  nself db generate vector-search public.docs.embedding
  nself db generate vector-search public.docs.embedding --distance l2 --roles user,staff
  nself db generate vector-search public.docs.embedding --track`,
	Args: cobra.ExactArgs(1),
	RunE: runGenerateVectorSearch,
}

func init() {
	f := generateVectorSearchCmd.Flags()
	f.String("distance", "cosine", "Distance: cosine, l2 or ip")
	f.StringSlice("roles", nil, "Hasura roles that may call the function (default user)")
	f.Bool("track", false, "Track the function in Hasura (apply the migration first)")
	f.String("migration-dir", "", "Migration directory (default ./migrations)")
	f.Bool("json", false, "Emit a v1 JSON envelope")
	generateCmd.AddCommand(generateVectorSearchCmd)
}

func runGenerateVectorSearch(cmd *cobra.Command, args []string) error {
	spec, err := vectorsearch.ParseTarget(args[0])
	if err != nil {
		return err
	}
	dist, _ := cmd.Flags().GetString("distance")
	spec.Distance = vectorsearch.Distance(dist)
	spec.Roles, _ = cmd.Flags().GetStringSlice("roles")
	if err := spec.Validate(); err != nil {
		return err
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	track, _ := cmd.Flags().GetBool("track")
	migDir, _ := cmd.Flags().GetString("migration-dir")
	deps := vectorDepsFor(cfg)
	res, err := vectorsearch.Run(cmd.Context(), deps, vectorsearch.Options{Spec: spec, ProjectDir: dir, MigrationDir: migDir, Track: track})
	if err != nil {
		return err
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return output.EmitData(output.Default(), "db generate vector-search", res)
	}
	return printVectorResult(cmd, res)
}

// vectorDepsFor wires the real seams; tests replace it.
var vectorDepsFor = func(cfg *config.Config) vectorsearch.Deps {
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself" // the default database name, as in the rest of internal/database
	}
	return vectorsearch.Deps{
		Exec:          docker.ExecCapture,
		PG:            vectorsearch.PG{Container: cfg.ProjectName + "_postgres", User: user, DB: db},
		TableTracked:  database.MetadataTracksTable,
		ColumnVisible: database.CheckDiskColumnVisible,
		Drift: func(ctx context.Context, projectDir string) ([]string, error) {
			return database.VectorMetadataDrift(ctx, cfg, projectDir)
		},
		Track: func(ctx context.Context, schema, name, table, column string, roles []string) error {
			return database.TrackFunction(ctx, cfg, schema, name, table, column, roles)
		},
		Now: time.Now,
	}
}

// printVectorResult prints the human summary.
func printVectorResult(cmd *cobra.Command, res vectorsearch.Result) error {
	w := cmd.OutOrStdout()
	verb := "Wrote"
	if !res.MigrationNew {
		verb = "Kept"
	}
	_, _ = fmt.Fprintf(w, "%s %s\n  function %s(query vector(%d)), %s distance\n", verb, res.Migration, res.Function, res.Dim, res.Distance)
	if res.Tracked {
		_, _ = fmt.Fprintf(w, "Tracked in Hasura for role(s): %v\n", res.Roles)
	} else {
		_, _ = fmt.Fprintln(w, "Next: nself db migrate up --migration-dir migrations, then run this command again with --track.")
	}
	if res.Note != "" {
		_, _ = fmt.Fprintln(w, res.Note)
	}
	return nil
}
