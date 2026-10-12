package nhost

// Purpose: build the nhost bundle: postgres snapshot (tables, auth users with bcrypt), Hasura
// metadata, hasura-storage objects. Inputs: flags, started target. Outputs: sealed bundle.
// Constraints: see package doc; the shared flow lives in hasurameta.Extract.
import (
	"context"

	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/hasurameta"
	"github.com/nself-org/cli/internal/importer/postgres"
)

// Extract writes the bundle at stage.
func (Source) Extract(ctx context.Context, in importer.Input, stage string) (importer.Extracted, error) {
	base, err := hasurameta.Base(in.Values["hasura-url"])
	if err != nil {
		return importer.Extracted{}, err
	}
	secret, err := in.Secret("admin-secret-env")
	if err != nil {
		return importer.Extracted{}, err
	}
	return hasurameta.Extract(ctx, postgres.Source{}, pgInput(in), stage, "nhost", base, secret, addStorage(ctx, in))
}
