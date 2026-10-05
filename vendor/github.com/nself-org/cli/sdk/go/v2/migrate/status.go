package migrate

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Progress is the migration progress a plugin serves in its /health JSON.
//
//   - Expected counts the current migration files.
//   - Applied counts the current files recorded in the ledger with a matching
//     checksum. A drifted file is not counted, so applied < expected and the
//     CLI's applied == expected wait keeps failing while Apply would refuse.
//   - Drifted counts current files recorded with a different checksum.
//   - Deleted names recorded files that are no longer on disk.
//
// A legacy ledger without a checksum column is read by file name only: its
// files count as applied and drift cannot be seen until Baseline runs.
type Progress struct {
	Applied  int      `json:"applied"`
	Expected int      `json:"expected"`
	Drifted  int      `json:"drifted,omitempty"`
	Deleted  []string `json:"deleted,omitempty"`
}

// Ready reports whether every expected migration is applied, none drifted and
// none deleted. The CLI waits for applied == expected after install; it never
// applies plugin SQL.
func (s Progress) Ready() bool {
	return s.Applied == s.Expected && s.Drifted == 0 && len(s.Deleted) == 0
}

// HealthField renders the status as the JSON object member the contract
// requires in /health: "migrations":{"applied":n,"expected":m}. Drift and
// deleted counts are appended (`,"drifted":n,"deleted":k`) only when non-zero.
// Embed it between braces, or marshal Progress under the key "migrations".
func HealthField(s Progress) string {
	extra := ""
	if s.Drifted > 0 {
		extra += fmt.Sprintf(`,"drifted":%d`, s.Drifted)
	}
	if len(s.Deleted) > 0 {
		extra += fmt.Sprintf(`,"deleted":%d`, len(s.Deleted))
	}
	return fmt.Sprintf(`"migrations":{"applied":%d,"expected":%d%s}`, s.Applied, s.Expected, extra)
}

// Status reads the ledger without locking or writing anything. A missing
// ledger counts as zero applied. It uses the same Options as Apply.
func Status(ctx context.Context, pool *pgxpool.Pool, opts Options) (Progress, error) {
	p, err := newPlan(opts)
	if err != nil {
		return Progress{}, err
	}
	files, err := p.readFiles()
	if err != nil {
		return Progress{}, err
	}
	st := Progress{Expected: len(files)}
	exists, err := p.ledgerExists(ctx, pool)
	if err != nil || !exists {
		return st, err
	}
	hasSum, err := p.ledgerHasChecksum(ctx, pool)
	if err != nil {
		return st, err
	}
	q := `SELECT filename, '' FROM ` + p.ledgerTable()
	if hasSum {
		q = `SELECT filename, checksum FROM ` + p.ledgerTable()
	}
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return st, fmt.Errorf("migrate: status: %w", err)
	}
	defer rows.Close()
	recorded := map[string]string{}
	for rows.Next() {
		var n, sum string
		if err := rows.Scan(&n, &sum); err != nil {
			return st, fmt.Errorf("migrate: status: %w", err)
		}
		recorded[n] = sum
	}
	if err := rows.Err(); err != nil {
		return st, fmt.Errorf("migrate: status: %w", err)
	}
	onDisk := map[string]bool{}
	for _, f := range files {
		onDisk[f.name] = true
		sum, done := recorded[f.name]
		switch {
		case !done:
		case hasSum && sum != f.sum:
			st.Drifted++
		default:
			st.Applied++
		}
	}
	for n := range recorded {
		if !onDisk[n] {
			st.Deleted = append(st.Deleted, n)
		}
	}
	sort.Strings(st.Deleted)
	return st, nil
}
