package database

// Purpose: import only the bundle's metadata without altering unrelated live tables.
// Inputs: raw per-table Hasura records. Outputs: one additive reconcile bulk plan.
// Constraints: conflicts fail before Apply; never untrack, drop or replace_metadata.
import (
	"context"
	"encoding/json"
	"errors"
	"github.com/nself-org/cli/internal/config"
	"strings"
)

func BuildAdditivePlan(ctx context.Context, cfg *config.Config, tables []map[string]any) (ReconcilePlan, error) {
	live, err := fetchLiveTables(ctx, cfg)
	if err != nil {
		return ReconcilePlan{}, errors.New("live metadata read failed")
	}
	return additivePlan(tables, live)
}
func additivePlan(tables []map[string]any, live map[string]hasuraTable) (ReconcilePlan, error) {
	repo := map[string]hasuraTable{}
	for _, raw := range tables {
		b, err := json.Marshal(raw)
		if err != nil {
			return ReconcilePlan{}, errors.New("invalid import metadata")
		}
		var t hasuraTable
		if json.Unmarshal(b, &t) != nil {
			return ReconcilePlan{}, errors.New("invalid import metadata")
		}
		if _, err := SanitizeIdentifier(t.Table.Schema); err != nil {
			return ReconcilePlan{}, errors.New("invalid metadata schema")
		}
		if _, err := SanitizeIdentifier(t.Table.Name); err != nil {
			return ReconcilePlan{}, errors.New("invalid metadata table")
		}
		repo[t.Table.key()] = t
	}
	subset := map[string]hasuraTable{}
	for key := range repo {
		if t, ok := live[key]; ok {
			subset[key] = t
		}
	}
	plan := buildReconcilePlan(repo, subset, true)
	for _, op := range plan.BulkOps {
		kind, _ := op["type"].(string)
		if strings.Contains(kind, "drop") || strings.Contains(kind, "untrack") || kind == "replace_metadata" {
			return ReconcilePlan{}, errors.New("import metadata conflicts with existing permissions")
		}
	}
	// Existing relationships are never overwritten; reject conflicting definitions.
	for key, want := range repo {
		got := subset[key]
		if computedFieldConflict(got, want) {
			return ReconcilePlan{}, errors.New("import metadata conflicts with existing computed fields")
		}
		for _, pair := range [][2][]hasuraRelationship{{got.ObjectRelationships, want.ObjectRelationships}, {got.ArrayRelationships, want.ArrayRelationships}} {
			for _, w := range pair[1] {
				for _, g := range pair[0] {
					if w.Name == g.Name {
						wb, _ := json.Marshal(w.Using)
						gb, _ := json.Marshal(g.Using)
						if string(wb) != string(gb) {
							return ReconcilePlan{}, errors.New("import metadata conflicts with existing relationships")
						}
					}
				}
			}
		}
	}
	return plan, nil
}
func ApplyAdditivePlan(ctx context.Context, cfg *config.Config, plan ReconcilePlan) error {
	if err := plan.Apply(ctx, cfg); err != nil {
		return errors.New("additive metadata apply failed")
	}
	return nil
}
