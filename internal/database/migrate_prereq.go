package database

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// Purpose: enforce that every ALTER TABLE target a pending migration batch
// touches will actually exist by the time that migration runs — either
// already present in the live schema, or created earlier in the same batch
// — and refuse with an actionable message before applying anything when it
// won't.
//
// This closes a real gap (cli issue, verified against nself-org/web): a
// Hasura-CLI-managed migration (hasura/migrations/default/<ts>_name/up.sql)
// creates a table, and a later, entirely separate numbered migration
// (e.g. backend/migrations/009_licensing_tiers.sql) only ALTERs it. Both
// migration chains are tracked in their own ledgers, so replaying the
// numbered chain alone against an empty database — the path a fresh
// self-hosting install takes — fails with "relation does not exist" even
// though staging/production have long since had it applied out of band.
//
// Inputs: pending migration file paths (already sorted apply order) and a
// *config.Config for the one live-catalog round trip.
// Outputs: nil when every ALTER target is satisfied; otherwise an error
// wrapping errs.ErrMigrationPrerequisiteMissing that names every missing
// object, the migration that needs it, and (when a Hasura migrations
// directory is present) the exact command to apply it.
// Constraints: read-only detection — reuses ExtractCreatedObjects/
// queryExistingObjects from migrate_detect*.go rather than a second,
// divergent implementation (cli#392 added those for `db migrate
// baseline`/`status --detect`). Never applies the other migration system's
// SQL itself: two independent migration systems must not silently gain
// shared write authority over the same objects — see the ASI nSelf-First /
// generated-artifact doctrine this would otherwise violate.

// alterTableRe matches ALTER TABLE targets, capturing whether the statement
// already guards non-existence with IF EXISTS (group 2) — a guarded ALTER is
// a no-op against a missing table, so it carries no prerequisite risk and is
// excluded from the objects ExtractAlteredObjects returns. The target
// (group 3) uses qualifiedIdent (migrate_detect.go) so both a bare name and
// a schema-qualified and/or double-quoted one are recognized — never a
// stray token like "ENABLE" or "ADD" from unrelated text a line or two
// later, since the whole ALTER TABLE ... <name> sequence must match
// contiguously.
var alterTableRe = regexp.MustCompile(`(?i)\bALTER\s+TABLE\s+(?:ONLY\s+)?(IF\s+EXISTS\s+)?(` + qualifiedIdent + `)`)

// ExtractAlteredObjects returns every table sqlContent's ALTER TABLE
// statements target, skipping any guarded with IF EXISTS (those cannot fail
// on a missing table, so have no prerequisite to check). Scans a comment-
// and dollar-quoted-body-stripped copy (cleanSQLForObjectScan, shared with
// ExtractCreatedObjects) so two classes of false positive are impossible:
//   - commentary text such as "-- ALTER TABLE\n--   ENABLE ROW LEVEL
//     SECURITY" being read as a real, unconditional ALTER TABLE ENABLE
//     (this produced the bogus "ENABLE"/"ADD"/"in" prerequisite names seen
//     against a real prod-shaped database);
//   - an ALTER TABLE inside a "DO $$ IF EXISTS (...) THEN ... END $$;"
//     runtime existence guard being read as unconditional — its target is
//     never a hard prerequisite, since the guard already handles a missing
//     table at runtime.
func ExtractAlteredObjects(sqlContent string) []ObjectRef {
	cleaned := cleanSQLForObjectScan(sqlContent)
	var out []ObjectRef
	for _, m := range alterTableRe.FindAllStringSubmatch(cleaned, -1) {
		if m[1] != "" { // IF EXISTS present — guarded, not a prerequisite risk
			continue
		}
		out = append(out, ObjectRef{Kind: ObjectTable, Name: m[2]})
	}
	return out
}

// MissingPrerequisite is one ALTER TABLE target that will not exist by the
// time the migration needing it runs.
type MissingPrerequisite struct {
	Object      ObjectRef
	MigrationID string // ledger key of the migration whose ALTER TABLE needs it
}

// parsedMigration is one migration file's extracted CREATE/ALTER targets,
// keyed by its ledger identity.
type parsedMigration struct {
	Name    string
	Created []ObjectRef
	Altered []ObjectRef
}

// parseMigrationFile reads f and extracts the objects it creates and alters.
// Pure aside from the single file read.
func parseMigrationFile(f string) (parsedMigration, error) {
	data, err := os.ReadFile(f)
	if err != nil {
		return parsedMigration{}, fmt.Errorf("read migration %s: %w", migrationKey(f), err)
	}
	content := string(data)
	return parsedMigration{
		Name:    migrationKey(f),
		Created: ExtractCreatedObjects(content),
		Altered: ExtractAlteredObjects(content),
	}, nil
}

// classifyAlterPrerequisites is the pure combinatorial half of
// checkAlterPrerequisites: given each pending file's extracted objects (in
// apply order) and a live-existing presence map (keyed by ObjectRef.Key()),
// returns every ALTER TABLE target that will not exist by the time its
// migration runs — i.e. it is neither already live, nor created earlier in
// an already-processed file, nor created by that same file (a single
// migration creating a table and then ALTERing it is the common, safe
// authoring order — CREATE runs before ALTER within one file). Deduped by
// object key, first occurrence wins, so a table altered by several pending
// migrations is only reported once (against the migration that first
// needed it).
func classifyAlterPrerequisites(files []parsedMigration, existing map[string]bool) []MissingPrerequisite {
	createdSoFar := make(map[string]bool)
	seen := make(map[string]bool)
	var missing []MissingPrerequisite
	for _, p := range files {
		// A file's own CREATE targets satisfy its own ALTER targets even
		// though createdSoFar isn't updated with them until after this
		// loop iteration — see TestClassifyAlterPrerequisites_SatisfiedByOwnFile.
		availableNow := make(map[string]bool, len(createdSoFar)+len(p.Created))
		for k := range createdSoFar {
			availableNow[k] = true
		}
		for _, obj := range p.Created {
			availableNow[obj.Key()] = true
		}

		for _, obj := range p.Altered {
			key := obj.Key()
			if existing[key] || availableNow[key] || seen[key] {
				continue
			}
			seen[key] = true
			missing = append(missing, MissingPrerequisite{Object: obj, MigrationID: p.Name})
		}
		for _, obj := range p.Created {
			createdSoFar[obj.Key()] = true
		}
	}
	return missing
}

// checkAlterPrerequisites verifies every ALTER TABLE target across files
// (already-sorted pending apply order) against the live schema plus each
// other, in a single catalog round trip. Returns an empty slice, not an
// error, when files is empty — callers should skip the check entirely when
// there is nothing pending (idempotency: a fully-applied batch must never
// re-evaluate, let alone refuse).
func checkAlterPrerequisites(ctx context.Context, cfg *config.Config, files []string) ([]MissingPrerequisite, error) {
	if len(files) == 0 {
		return nil, nil
	}

	parsedFiles := make([]parsedMigration, 0, len(files))
	var toCheck []ObjectRef
	for _, f := range files {
		p, err := parseMigrationFile(f)
		if err != nil {
			return nil, err
		}
		parsedFiles = append(parsedFiles, p)
		toCheck = append(toCheck, p.Altered...)
	}

	existing, err := queryExistingObjects(ctx, cfg, toCheck)
	if err != nil {
		return nil, fmt.Errorf("query live schema for migration prerequisites: %w", err)
	}

	return classifyAlterPrerequisites(parsedFiles, existing), nil
}

// hasuraMigrationsDirIfPresent returns the Hasura CLI migrations directory
// ("hasura/migrations/default" or the flat "hasura/migrations") relative to
// the working directory, if one exists. Mirrors migrationsDir's own
// candidate order (migrate_ledger.go) so both agree on what counts as "this
// project has Hasura-managed migrations".
func hasuraMigrationsDirIfPresent() (string, bool) {
	for _, c := range []string{"hasura/migrations/default", "hasura/migrations"} {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c, true
		}
	}
	return "", false
}

// findHasuraCreator returns the ledger key of the migration in hasuraDir
// that creates obj, if any. Read-only, best-effort: a scan failure or no
// match just means the refusal message falls back to naming the directory
// generically instead of a specific migration.
func findHasuraCreator(hasuraDir string, obj ObjectRef) (string, bool) {
	files, err := scanMigrations(hasuraDir)
	if err != nil {
		return "", false
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, created := range ExtractCreatedObjects(string(data)) {
			if created.Key() == obj.Key() {
				return migrationKey(f), true
			}
		}
	}
	return "", false
}

// prerequisiteError builds the refusal checkAlterPrerequisites' non-empty
// result becomes. Names every missing object, which migration needs it,
// and — when this project has a Hasura migrations directory — the specific
// Hasura migration that creates it (when found) plus the exact command to
// apply it before retrying.
func prerequisiteError(missing []MissingPrerequisite) error {
	hasuraDir, hasHasura := hasuraMigrationsDirIfPresent()

	lines := make([]string, 0, len(missing))
	for _, m := range missing {
		line := fmt.Sprintf("  - %s %q (needed by migration %s)", m.Object.Kind, m.Object.Name, m.MigrationID)
		if hasHasura {
			if creator, ok := findHasuraCreator(hasuraDir, m.Object); ok {
				line += fmt.Sprintf(" — created by Hasura migration %s in %s", creator, hasuraDir)
			} else {
				line += fmt.Sprintf(" — not created by any migration in %s either; check what else is expected to create it", hasuraDir)
			}
		}
		lines = append(lines, line)
	}

	msg := "refusing to run migrations: the following object(s) do not exist and are not created by any pending migration in this batch:\n" +
		strings.Join(lines, "\n")

	if hasHasura {
		msg += fmt.Sprintf(
			"\nThese normally come from the Hasura-managed migrations in %s — a separate migration system this command does not apply.\n"+
				"Fix: apply them first, then re-run this command:\n"+
				"  cd hasura && hasura migrate apply --database-name default\n"+
				"  nself db migrate up",
			hasuraDir,
		)
	} else {
		msg += "\nNo Hasura migrations directory was found alongside this one. Verify the object is created by some other process before running this migration."
	}

	return fmt.Errorf("%s: %w", msg, errs.ErrMigrationPrerequisiteMissing)
}
