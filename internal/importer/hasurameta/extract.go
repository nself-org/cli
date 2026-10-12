package hasurameta

// Purpose: the extract flow shared by the nhost and hasura-cloud sources: one postgres snapshot
// bundle, plus the source's Hasura metadata, plus whatever the caller adds (storage objects).
// Inputs: the postgres source, its input, the stage path, the Hasura endpoint and admin secret.
// Outputs: a sealed bundle at stage. Constraints: the intermediate postgres bundle (it holds
// the full dump) is removed on every path; errors carry no secret.
import (
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/portable"
)

// Extract exports the metadata first (a wrong admin secret fails before the dump starts), runs
// the postgres snapshot extract into a sibling directory, then recomposes it into stage with
// hasura/metadata.json, the producer source and the optional extra members.
func Extract(ctx context.Context, pg importer.Source, pgIn importer.Input, stage, source string, base *url.URL, adminSecret string, extra func(*portable.Writer, *portable.Manifest) error) (importer.Extracted, error) {
	raw, err := Export(ctx, base, adminSecret)
	if err != nil {
		return importer.Extracted{}, err
	}
	pgStage := filepath.Join(filepath.Dir(stage), "stage-pg")
	defer func() { _ = os.RemoveAll(pgStage) }()
	if _, err = pg.Extract(ctx, pgIn, pgStage); err != nil {
		return importer.Extracted{}, err
	}
	err = Recompose(pgStage, stage, source, func(w *portable.Writer, m *portable.Manifest) error {
		filtered, counts, e := Filter(raw, m.DB)
		if e != nil {
			return e
		}
		if _, e = w.WriteFile("hasura/metadata.json", bytes.NewReader(filtered)); e != nil {
			return errs.New("E520", msgMetadataMember)
		}
		m.Hasura = counts
		if extra != nil {
			return extra(w, m)
		}
		return nil
	})
	if err != nil {
		return importer.Extracted{}, err
	}
	return importer.Extracted{Stage: stage}, nil
}
