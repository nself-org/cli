package commands

// Purpose: expose Hasura's read-only schema and permission metadata as commands.
// Inputs: loaded project secret and Hasura endpoint. Outputs: table or v1 envelope.
// Constraints: never print the secret or row data; use the shared Hasura reader.
// SPORT: P7-SURF-32

import (
	"fmt"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/hasura"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

func init() {
	schema := &cobra.Command{Use: "schema", Short: "Read the Hasura GraphQL schema", Args: cobra.NoArgs, RunE: runDBHasuraSchema}
	permissions := &cobra.Command{Use: "permissions", Short: "Read Hasura table permissions", Args: cobra.NoArgs, RunE: runDBHasuraPermissions}
	schema.Flags().Bool("json", false, "Emit a v1 JSON envelope")
	permissions.Flags().Bool("json", false, "Emit a v1 JSON envelope")
	dbHasuraCmd.AddCommand(schema, permissions)
}

func runDBHasuraSchema(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	data, err := hasura.Schema(cmd.Context(), hasura.Endpoint(), cfg.Hasura.AdminSecret)
	if err != nil {
		return err
	}
	if !data.Valid {
		return errs.New("E200", "invalid Hasura schema response")
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		return output.EmitData(output.Default(), "db hasura schema", data)
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "KIND\tTYPE\tFIELDS"); err != nil {
		return err
	}
	for _, typ := range data.Types {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%d\n", typ.Kind, typ.Name, len(typ.Fields)); err != nil {
			return err
		}
	}
	return nil
}

func runDBHasuraPermissions(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	data, err := hasura.Permissions(cmd.Context(), hasura.Endpoint(), cfg.Hasura.AdminSecret)
	if err != nil {
		return err
	}
	if !data.Valid {
		return errs.New("E200", "invalid Hasura metadata response")
	}
	if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
		if data.Tables == nil {
			data.Tables = []hasura.TablePermissions{}
		}
		return output.EmitData(output.Default(), "db hasura permissions", data)
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "TABLE\tPERMISSION TYPES"); err != nil {
		return err
	}
	for _, table := range data.Tables {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%d\n", table.Table, len(table.Permissions)); err != nil {
			return err
		}
	}
	return nil
}
