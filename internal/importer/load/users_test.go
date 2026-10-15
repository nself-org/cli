package load

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nself-org/cli/internal/importer/verify"
)

// Two phone-only users (NULL email) must both load as NULL email, never as "".
func TestUserRowKeepsNullEmail(t *testing.T) {
	lines := `{"id":"11111111-1111-4111-8111-111111111111","email":null,"phone":"+15550001","display_name":null,"metadata":null}
{"id":"22222222-2222-4222-8222-222222222222","email":null,"phone":"+15550002","display_name":null,"metadata":null}`
	dec := json.NewDecoder(strings.NewReader(lines))
	nullable := map[string]bool{"email": true, "phone_number": true, "display_name": true, "metadata": true}
	for i := 0; i < 2; i++ {
		var u verify.User
		if err := dec.Decode(&u); err != nil {
			t.Fatal(err)
		}
		var id pgtype.UUID
		if err := id.Scan(u.ID); err != nil {
			t.Fatal(err)
		}
		row := userRow(id, time.Unix(1, 0).UTC(), nil, u, nullable)
		if row[1] != nil || row[4] != nil || row[8] != nil {
			t.Fatalf("user %d: NULL fields became %#v %#v %#v", i, row[1], row[4], row[8])
		}
		if row[3] == nil {
			t.Fatalf("user %d lost its phone", i)
		}
		// A target column that is NOT NULL gets the empty default instead of failing the COPY.
		strict := userRow(id, time.Unix(1, 0).UTC(), nil, u, map[string]bool{"email": true, "phone_number": true})
		if strict[4] != "" || strict[1] != nil {
			t.Fatalf("strict display name: %#v", strict[4])
		}
	}
	// An explicit empty string is not NULL.
	empty := ""
	row := userRow(pgtype.UUID{}, time.Unix(1, 0).UTC(), nil, verify.User{Email: &empty}, map[string]bool{"email": true})
	if row[1] != "" {
		t.Fatalf("empty email changed to %#v", row[1])
	}
}

func TestUserRowPreservesDefaultRole(t *testing.T) {
	var u verify.User
	if err := json.Unmarshal([]byte(`{"default_role":"restricted","roles":["restricted","user"]}`), &u); err != nil {
		t.Fatal(err)
	}
	row := userRow(pgtype.UUID{}, time.Unix(1, 0).UTC(), nil, u, nil)
	if row[13] != "restricted" {
		t.Fatalf("custom default role changed to %v", row[13])
	}
}
