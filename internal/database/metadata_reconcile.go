package database

// Purpose: nself db reconcile — push repo-declared Hasura metadata (table
// tracking, relationships, permissions) to a live instance. Ported from
// ntask/backend/scripts/metadata-reconcile.sh's proven design.
// Inputs: context, *config.Config, project directory.
// Outputs: a ReconcilePlan describing every change, or an error.
// Constraints:
//   - Never issues `hasura metadata apply` / replace_metadata: that REPLACES
//     the whole document and would untrack hasura-auth's own tables
//     (verified against ntask production 2026-08-22: repo declares 28
//     tables, production tracks 35). Every op here is a targeted per-object
//     call instead, and the whole plan is sent as ONE Hasura `bulk` call so a
//     failure rolls back instead of leaving a partial apply — this exact
//     partial-apply outage happened once by hand on ntask's
//     np_member_profiles.
//   - Relationships are ordered before permissions: a permission whose
//     filter/check traverses a relationship fails with "Inconsistent object"
//     if the relationship doesn't exist yet, and if the OLD permission was
//     already dropped in the same batch the table is left with none — an
//     outage. Table tracking is ordered first: neither relationships nor
//     permissions can attach to an untracked table.
//   - refuseUnsafeReconcile (called unconditionally, no --force escape)
//     blocks the two most dangerous failure modes from the 2026-08-21/08-22
//     inbox reports: reconciling toward a table that does not exist in
//     Postgres at all, and reconciling while live has a non-hasura-auth
//     table the repo does not declare (a strong signal the repo metadata
//     itself is stale/incomplete, not that live is wrong).

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// ReconcileChange is one human-readable line of a reconcile plan.
type ReconcileChange struct {
	Table       string
	Description string
}

// ReconcilePlan is dry-run output: the exact bulk operations Apply would
// send, plus a human-readable change list.
type ReconcilePlan struct {
	BulkOps []map[string]interface{}
	Changes []ReconcileChange
}

// BuildReconcilePlan loads repo + live metadata, refuses on either unsafe
// condition (see package doc), and returns the ordered, targeted plan to
// bring live into agreement with the repo. It never touches a live table the
// repo doesn't declare, and never removes anything.
func BuildReconcilePlan(ctx context.Context, cfg *config.Config, projectDir string) (ReconcilePlan, error) {
	repo, err := loadRepoTables(projectDir)
	if err != nil {
		return ReconcilePlan{}, err
	}
	live, err := fetchLiveTables(ctx, cfg)
	if err != nil {
		return ReconcilePlan{}, err
	}
	if err := refuseUnsafeReconcile(ctx, cfg, repo, live); err != nil {
		return ReconcilePlan{}, err
	}
	return buildReconcilePlanPure(repo, live), nil
}

// refuseUnsafeReconcile blocks reconcile before any write when either unsafe
// condition from the package doc is present. Unconditional — no bypass flag
// exists that could reintroduce the ntask hasura-auth-untrack outage.
func refuseUnsafeReconcile(ctx context.Context, cfg *config.Config, repo, live map[string]hasuraTable) error {
	for key, rt := range repo {
		if _, ok := live[key]; ok {
			continue
		}
		exists, err := postgresTableExists(ctx, cfg, rt.Table.Schema, rt.Table.Name)
		if err != nil {
			return fmt.Errorf("refuse-check table existence for %s: %w", key, err)
		}
		if !exists {
			return fmt.Errorf(
				"refusing to reconcile: repo metadata declares %s but no such table exists in "+
					"Postgres — fix the repo metadata or run migrations before reconciling", key)
		}
	}

	for key := range live {
		if hasuraAuthOwnedTables[key] {
			continue
		}
		if _, ok := repo[key]; !ok {
			return fmt.Errorf(
				"refusing to reconcile: live Hasura tracks %s but repo metadata does not declare "+
					"it — reconcile never untracks a table, but proceeding while metadata is this "+
					"far out of sync risks masking a bigger drift; update the repo metadata first", key)
		}
	}
	return nil
}

// postgresTableExists checks the underlying Postgres table exists (distinct
// from "tracked by Hasura" — a table can exist untracked, which is fine).
func postgresTableExists(ctx context.Context, cfg *config.Config, schema, name string) (bool, error) {
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	sqlText := fmt.Sprintf(
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema=%s AND table_name=%s)",
		sqlQuoteLiteral(schema), sqlQuoteLiteral(name),
	)
	out, err := querySQL(ctx, cfg, db, sqlText)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "t", nil
}

func sqlQuoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// buildReconcilePlanPure is the deterministic planning logic, split out from
// BuildReconcilePlan so it can be unit tested without live Hasura/Postgres.
func buildReconcilePlanPure(repo, live map[string]hasuraTable) ReconcilePlan {
	return buildReconcilePlan(repo, live, false)
}

// buildReconcilePlan also plans computed fields when withComputed is set; only the additive
// import asks for them.
func buildReconcilePlan(repo, live map[string]hasuraTable, withComputed bool) ReconcilePlan {
	var trackOps, relOps, cfOps, permOps []map[string]interface{}
	var changes []ReconcileChange

	for _, key := range sortedTableKeys(repo) {
		rt := repo[key]
		tableArg := map[string]interface{}{"schema": rt.Table.Schema, "name": rt.Table.Name}
		lt, exists := live[key]
		if !exists {
			trackOps = append(trackOps, map[string]interface{}{
				"type": "pg_track_table",
				"args": map[string]interface{}{"source": "default", "table": tableArg},
			})
			changes = append(changes, ReconcileChange{key, "track (declared in repo, not tracked live)"})
			lt = hasuraTable{}
		}

		newRelOps, relChanges := planRelationshipOps(key, tableArg, lt, rt)
		relOps = append(relOps, newRelOps...)
		changes = append(changes, relChanges...)

		if withComputed {
			newCfOps, cfChanges := planComputedFieldOps(key, tableArg, lt, rt)
			cfOps = append(cfOps, newCfOps...)
			changes = append(changes, cfChanges...)
		}

		newPermOps, permChanges := planPermissionOps(key, tableArg, lt, rt)
		permOps = append(permOps, newPermOps...)
		changes = append(changes, permChanges...)
	}

	bulk := make([]map[string]interface{}, 0, len(trackOps)+len(relOps)+len(cfOps)+len(permOps))
	bulk = append(bulk, trackOps...)
	bulk = append(bulk, relOps...)
	bulk = append(bulk, cfOps...)
	bulk = append(bulk, permOps...)
	return ReconcilePlan{BulkOps: bulk, Changes: changes}
}

func planRelationshipOps(key string, tableArg map[string]interface{}, lt, rt hasuraTable) ([]map[string]interface{}, []ReconcileChange) {
	var ops []map[string]interface{}
	var changes []ReconcileChange
	for _, relSet := range []struct {
		api      string
		liveRels []hasuraRelationship
		repoRels []hasuraRelationship
	}{
		{"pg_create_object_relationship", lt.ObjectRelationships, rt.ObjectRelationships},
		{"pg_create_array_relationship", lt.ArrayRelationships, rt.ArrayRelationships},
	} {
		have := map[string]bool{}
		for _, r := range relSet.liveRels {
			have[r.Name] = true
		}
		for _, r := range relSet.repoRels {
			if have[r.Name] {
				continue
			}
			ops = append(ops, map[string]interface{}{
				"type": relSet.api,
				"args": map[string]interface{}{
					"source": "default", "table": tableArg,
					"name": r.Name, "using": r.Using,
				},
			})
			changes = append(changes, ReconcileChange{key, fmt.Sprintf("create relationship %q", r.Name)})
		}
	}
	return ops, changes
}

func planPermissionOps(key string, tableArg map[string]interface{}, lt, rt hasuraTable) ([]map[string]interface{}, []ReconcileChange) {
	var ops []map[string]interface{}
	var changes []ReconcileChange
	for _, pk := range permissionKinds {
		rperm := indexPermissionsByRole(rt.tablePermissions(pk.short))
		lperm := indexPermissionsByRole(lt.tablePermissions(pk.short))
		roles := make([]string, 0, len(rperm))
		for role := range rperm {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		for _, role := range roles {
			perm := rperm[role]
			existing, has := lperm[role]
			if has && permissionEqual(existing, perm) {
				continue
			}
			if has {
				ops = append(ops, map[string]interface{}{
					"type": fmt.Sprintf("pg_drop_%s_permission", pk.short),
					"args": map[string]interface{}{"source": "default", "table": tableArg, "role": role},
				})
			}
			ops = append(ops, map[string]interface{}{
				"type": fmt.Sprintf("pg_create_%s_permission", pk.short),
				"args": map[string]interface{}{
					"source": "default", "table": tableArg, "role": role, "permission": perm,
				},
			})
			action := "create"
			if has {
				action = "replace"
			}
			changes = append(changes, ReconcileChange{key,
				fmt.Sprintf("%s %s permission for role %q", action, pk.short, role)})
		}
	}
	return ops, changes
}

func sortedTableKeys(m map[string]hasuraTable) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Apply executes the plan's bulk operations against the live instance as ONE
// atomic Hasura `bulk` metadata call, so a failure rolls back rather than
// leaving a partial apply.
func (p ReconcilePlan) Apply(ctx context.Context, cfg *config.Config) error {
	if len(p.BulkOps) == 0 {
		return nil
	}
	secret, err := readHasuraAdminSecretFromContainer(ctx, cfg)
	if err != nil {
		return err
	}
	if _, err := postMetadataAdmin(ctx, cfg, secret, "bulk", p.BulkOps); err != nil {
		return fmt.Errorf("apply reconcile plan: %w", err)
	}
	return nil
}
