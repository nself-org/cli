package credmask

import "testing"

// D-0743: the schemeless rules, directly. Fake values only.
func TestSchemeless(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"app:fakepw@tcp(db:3306)/app", "****@tcp(db:3306)/app"},
		{"app:fakepw@db.internal:5432/app", "****@db.internal:5432/app"},
		{"host=db password='fake pw' dbname=app", "host=db password=**** dbname=app"},
		{"app@db.internal", "app@db.internal"},
		{"password=****", "password=****"},
	} {
		if got := Schemeless(tc.in); got != tc.want {
			t.Errorf("Schemeless(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestURL_MasksSchemeAndSchemeless(t *testing.T) {
	if got, want := URL("postgres://app:fakepw@db/app?sslpassword=fakepw2&x=1"), "postgres://****@db/app?sslpassword=****&x=1"; got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}
