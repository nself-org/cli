package load

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nself-org/cli/internal/importer/verify"
)

// A restricted source account must not acquire the target's ordinary user role.
func TestUserRowPreservesAuthFields(t *testing.T) {
	var u verify.User
	if err := json.Unmarshal([]byte(`{"default_role":"restricted","roles":["restricted"],"locale":"fr","phone_number_verified":true,"is_anonymous":true,"email_verified":true,"disabled":true}`), &u); err != nil {
		t.Fatal(err)
	}
	row := userRow(pgtype.UUID{}, time.Unix(1, 0).UTC(), nil, u, nil)
	for column, want := range map[int]any{2: true, 6: true, 11: "fr", 12: true, 13: "restricted", 14: true} {
		if row[column] != want {
			t.Errorf("column %d = %v, want %v", column, row[column], want)
		}
	}
}
