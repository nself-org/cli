package database

// Purpose: add imported functions without replacing live configuration or grants.
// Inputs: default-source metadata records. Outputs: track and permission operations.
// Constraints: conflicting live definitions fail before any metadata write.
import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"

	"github.com/nself-org/cli/internal/config"
)

func fetchAdditiveMetadata(ctx context.Context, cfg *config.Config) (map[string]hasuraTable, []map[string]any, error) {
	secret, err := readHasuraAdminSecretFromContainer(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	b, err := postMetadataAdmin(ctx, cfg, secret, "export_metadata", struct{}{})
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		Sources []struct {
			Name      string           `json:"name"`
			Tables    []hasuraTable    `json:"tables"`
			Functions []map[string]any `json:"functions"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, nil, err
	}
	for _, src := range doc.Sources {
		if src.Name == "default" {
			return tablesByKey(src.Tables), src.Functions, nil
		}
	}
	return nil, nil, errors.New("default metadata source missing")
}

type importFunction struct {
	Function      hasuraTableRef `json:"function"`
	Configuration map[string]any `json:"configuration"`
	Permissions   []vtRole       `json:"permissions"`
}

func importFunctions(records []map[string]any) (map[string]importFunction, error) {
	out := map[string]importFunction{}
	for _, raw := range records {
		b, err := json.Marshal(raw)
		if err != nil {
			return nil, errors.New("invalid function metadata")
		}
		var f importFunction
		if json.Unmarshal(b, &f) != nil {
			return nil, errors.New("invalid function metadata")
		}
		if _, err := SanitizeIdentifier(f.Function.Schema); err != nil {
			return nil, errors.New("invalid function schema")
		}
		if _, err := SanitizeIdentifier(f.Function.Name); err != nil {
			return nil, errors.New("invalid function name")
		}
		for _, p := range f.Permissions {
			if p.Role == "" {
				return nil, errors.New("invalid function permission")
			}
		}
		if len(f.Configuration) == 0 {
			f.Configuration = nil
		}
		out[f.Function.key()] = f
	}
	return out, nil
}

func additiveFunctionOps(records, live []map[string]any) ([]map[string]any, error) {
	want, err := importFunctions(records)
	if err != nil {
		return nil, err
	}
	got, err := importFunctions(live)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var ops []map[string]any
	for _, k := range keys {
		w := want[k]
		g, exists := got[k]
		if exists && !reflect.DeepEqual(w.Configuration, g.Configuration) {
			return nil, errors.New("import metadata conflicts with existing function configuration")
		}
		ref := map[string]any{"schema": w.Function.Schema, "name": w.Function.Name}
		if !exists {
			args := map[string]any{"source": "default", "function": ref}
			if w.Configuration != nil {
				args["configuration"] = w.Configuration
			}
			ops = append(ops, map[string]any{"type": "pg_track_function", "args": args})
		}
		roles := map[string]bool{}
		for _, p := range w.Permissions {
			roles[p.Role] = true
		}
		for _, p := range g.Permissions {
			if !roles[p.Role] {
				return nil, errors.New("import metadata conflicts with existing function permissions")
			}
			delete(roles, p.Role)
		}
		names := make([]string, 0, len(roles))
		for role := range roles {
			names = append(names, role)
		}
		sort.Strings(names)
		for _, role := range names {
			ops = append(ops, map[string]any{"type": "pg_create_function_permission", "args": map[string]any{"source": "default", "function": ref, "role": role}})
		}
	}
	return ops, nil
}
