package commands

// Purpose: "nself db verify --role <role>" — role-scoped GraphQL
// introspection against a live Hasura instance, answering "is this feature
// reachable by an actual role" without a hand-rolled curl command.
// Inputs: the cobra command/args. Outputs: printed query/mutation counts for
// the role, or an error.
// Constraints: see internal/database/metadata_verify.go — sent with the
// X-Hasura-Role header and no admin secret.

import (
	"fmt"

	"github.com/nself-org/cli/internal/database"
	"github.com/spf13/cobra"
)

var dbVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify what a Hasura role can actually reach via role-scoped introspection",
	Long: `Runs a GraphQL introspection query against the live Hasura instance using
Hasura's role-impersonation mechanism (admin secret + X-Hasura-Role: <role>),
so the result reflects exactly what that role — and only that role — can
reach. The admin secret is read from the running Hasura container, never
from .env, and is used only to authenticate the impersonation; a bare role
header with no secret is not honored by Hasura and always falls back to the
unauthorized role.`,
	RunE: dataEnv(runDBVerify),
}

func init() {
	dbVerifyCmd.Flags().String("role", "", "Hasura role to introspect (required)")
	dbCmd.AddCommand(dbVerifyCmd)
}

func runDBVerify(cmd *cobra.Command, _ []string) error {
	role, _ := cmd.Flags().GetString("role")
	if role == "" {
		return fmt.Errorf("--role is required, e.g. 'nself db verify --role user'")
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	result, err := database.VerifyRoleReachability(cmd.Context(), cfg, role)
	if err != nil {
		return fmt.Errorf("verify role %q: %w", role, err)
	}

	setData(cmd, dbVerifyData{Role: result.Role, Queries: result.Queries, Mutations: result.Mutations})
	fmt.Printf("Role %q can reach:\n", result.Role)
	fmt.Printf("  queries:   %d\n", result.Queries)
	fmt.Printf("  mutations: %d\n", result.Mutations)
	return nil
}
