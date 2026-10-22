package appwrite

// Purpose: register the appwrite source and run its Detect/Extract steps.
// Inputs: --endpoint, --project, --api-key-env (name of the variable holding the API key).
// Outputs: a portable bundle at the stage path. Constraints: read-only against the source; the key
// is read once from the environment; failures surface only fixed credential-free messages.
import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/scratch"
)

// Source is the appwrite importer.
type Source struct{}

func init()                 { importer.Register(Source{}) }
func (Source) Name() string { return "appwrite" }
func (Source) Flags() []importer.FlagSpec {
	return []importer.FlagSpec{
		{Name: "endpoint", Usage: "Appwrite API endpoint, e.g. https://cloud.appwrite.io/v1"},
		{Name: "project", Usage: "Appwrite project ID"},
		{Name: "api-key-env", Usage: "Environment variable containing the Appwrite API key", EnvVarName: true},
	}
}

func connect(in importer.Input) (*client, error) {
	key, err := in.Secret("api-key-env")
	if err != nil {
		return nil, err
	}
	return newClient(in.Values["endpoint"], in.Values["project"], key)
}

func (Source) Detect(ctx context.Context, in importer.Input) (importer.Detection, error) {
	c, err := connect(in)
	if err != nil {
		return importer.Detection{}, err
	}
	d := importer.Detection{Kind: "appwrite", Version: "unknown", Notes: []string{}, Schemas: []string{}}
	var ver struct {
		Version string `json:"version"`
	}
	if c.getJSON(ctx, "/health/version", nil, &ver) == nil && ver.Version != "" {
		d.Version = ver.Version
	}
	_, err = c.each(ctx, "/databases", "databases", func(raw json.RawMessage) error {
		var db struct {
			ID string `json:"$id"`
		}
		if json.Unmarshal(raw, &db) != nil {
			return errs.New("E517", "source schema lookup failed")
		}
		name, e := schemaName(db.ID)
		if e != nil {
			return errs.New("E517", "source schema lookup failed")
		}
		d.Schemas = append(d.Schemas, name)
		return nil
	})
	if err != nil {
		return d, err
	}
	var users page
	if users, err = c.listPageLimit(ctx, "/users", "users", "", 0, 1); err != nil {
		return d, errs.New("E517", "source auth lookup failed")
	}
	d.SourceUsers = int(users.Total)
	if d.Objects, err = c.listObjects(ctx); err != nil {
		return d, err
	}
	d.Notes = append(d.Notes, fmt.Sprintf("%d users; document permissions are reported, not mapped", users.Total))
	return d, nil
}

// build is scratch.Build; tests replace it so no container is needed.
var build = scratch.Build

func (s Source) Extract(ctx context.Context, in importer.Input, stage string) (importer.Extracted, error) {
	if in.Target == nil {
		return importer.Extracted{}, errs.New("E520", "target config required for snapshot extraction")
	}
	c, err := connect(in)
	if err != nil {
		return importer.Extracted{}, err
	}
	if err = os.MkdirAll(filepath.Dir(stage), 0o700); err != nil {
		return importer.Extracted{}, errs.New("E520", "snapshot staging unavailable")
	}
	work, err := os.MkdirTemp(filepath.Dir(stage), ".appwrite-")
	if err != nil {
		return importer.Extracted{}, errs.New("E520", "snapshot staging unavailable")
	}
	defer func() { _ = os.RemoveAll(work) }()
	cols, err := c.schema(ctx)
	if err != nil {
		return importer.Extracted{}, err
	}
	if err = c.assemble(ctx, in, cols, work, stage); err != nil {
		return importer.Extracted{}, err
	}
	return importer.Extracted{Stage: stage}, nil
}

// schema lists databases, collections and attributes and maps them to tables.
func (c *client) schema(ctx context.Context) ([]*collection, error) {
	var cols []*collection
	names, schemaOwner := map[string]string{}, map[string]string{}
	_, err := c.each(ctx, "/databases", "databases", func(raw json.RawMessage) error {
		var db struct {
			ID string `json:"$id"`
		}
		if json.Unmarshal(raw, &db) != nil {
			return errs.New("E520", "source tables unavailable")
		}
		schema, e := schemaName(db.ID)
		if e != nil {
			return e
		}
		if owner, dup := schemaOwner[schema]; dup && owner != db.ID {
			return errs.New("E520", "source tables unavailable")
		}
		schemaOwner[schema] = db.ID
		path := "/databases/" + url.PathEscape(db.ID) + "/collections"
		_, e = c.each(ctx, path, "collections", func(raw json.RawMessage) error {
			var meta struct {
				ID    string   `json:"$id"`
				Perms []string `json:"$permissions"`
			}
			if json.Unmarshal(raw, &meta) != nil {
				return errs.New("E520", "source tables unavailable")
			}
			table, e := tableName(meta.ID)
			if e != nil {
				return e
			}
			if prev, dup := names[schema+"."+table]; dup && prev != meta.ID {
				return errs.New("E520", "source tables unavailable")
			}
			names[schema+"."+table] = meta.ID
			var attrs []attribute
			if _, e = c.walk(ctx, path+"/"+url.PathEscape(meta.ID)+"/attributes", "attributes", false, func(raw json.RawMessage) error {
				var a attribute
				if json.Unmarshal(raw, &a) != nil {
					return errs.New("E520", "source tables unavailable")
				}
				attrs = append(attrs, a)
				return nil
			}); e != nil {
				return e
			}
			mapped, notes, e := mapAttributes(attrs)
			if e != nil {
				return e
			}
			cols = append(cols, &collection{ID: meta.ID, DB: db.ID, Schema: schema, Table: table, Columns: mapped, Permissions: meta.Perms, notes: notes})
			return nil
		})
		return e
	})
	sort.Slice(cols, func(i, j int) bool { return cols[i].Schema+"."+cols[i].Table < cols[j].Schema+"."+cols[j].Table })
	return cols, err
}
