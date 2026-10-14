package build

// Purpose: D-0334: LiteralSecretLeaks also finds a secret that appears in its
//          percent-encoded form (connection URLs encode the password).

import "testing"

func TestLiteralSecretLeaksEncodedForm(t *testing.T) {
	secrets := map[string]string{"POSTGRES_PASSWORD": "p@ss:w/rd#1234"}
	leaky := []byte("services:\n  hasura:\n    environment:\n      DATABASE_URL: postgresql://u:p%40ss%3Aw%2Frd%231234@postgres:5432/db\n")
	if got := LiteralSecretLeaks(leaky, secrets); len(got) != 1 || got[0] != "POSTGRES_PASSWORD" {
		t.Fatalf("encoded literal not reported, got %v", got)
	}
	clean := []byte("services:\n  hasura:\n    environment:\n      DATABASE_URL: postgresql://u:${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}@postgres:5432/db\n")
	if got := LiteralSecretLeaks(clean, secrets); len(got) != 0 {
		t.Fatalf("reference reported as a leak: %v", got)
	}
}
