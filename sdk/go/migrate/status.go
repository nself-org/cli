package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Progress is the migration progress a plugin serves in its /health JSON.
// Applied counts the current migration files recorded in the ledger;
// Expected counts the current migration files.
type Progress struct {
	Applied  int `json:"applied"`
	Expected int `json:"expected"`
}

// Ready reports whether every expected migration is applied. The CLI waits
// for this (applied == expected) after install; it never applies plugin SQL.
func (s Progress) Ready() bool { return s.Applied == s.Expected }

// HealthField renders the status as the JSON object member the contract
// requires in /health: "migrations":{"applied":n,"expected":m}. Embed it
// between braces, or marshal Progress under the key "migrations".
func HealthField(s Progress) string {
	return fmt.Sprintf(`"migrations":{"applied":%d,"expected":%d}`, s.Applied, s.Expected)
}

// Status reads the ledger without locking or writing anything. A missing
// ledger counts as zero applied. It uses the same Options as Apply.
func Status(ctx context.Context, pool *pgxpool.Pool, opts Options) (Progress, error) {
	p, err := newPlan(opts)
	if err != nil {
		return Progress{}, err
	}
	names, err := p.listNames()
	if err != nil {
		return Progress{}, err
	}
	st := Progress{Expected: len(names)}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, p.ledgerTable()).Scan(&exists); err != nil {
		return st, fmt.Errorf("migrate: status: %w", err)
	}
	if !exists {
		return st, nil
	}
	rows, err := pool.Query(ctx, `SELECT filename FROM `+p.ledgerTable())
	if err != nil {
		return st, fmt.Errorf("migrate: status: %w", err)
	}
	defer rows.Close()
	recorded := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return st, fmt.Errorf("migrate: status: %w", err)
		}
		recorded[n] = true
	}
	if err := rows.Err(); err != nil {
		return st, fmt.Errorf("migrate: status: %w", err)
	}
	for _, n := range names {
		if recorded[n] {
			st.Applied++
		}
	}
	return st, nil
}
