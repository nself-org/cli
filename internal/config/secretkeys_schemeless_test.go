package config

import "testing"

func TestMaskSchemelessCredentials(t *testing.T) {
	cases := []struct{ in, want string }{
		{"user:private@tcp(localhost:3306)/db", "****@tcp(localhost:3306)/db"},
		{"user:p@ss:/word@unix(/tmp/mysql.sock)/db", "****@unix(/tmp/mysql.sock)/db"},
		{"user:private@db.example:5432/db", "****@db.example:5432/db"},
		{"user:p://word@tcp(db)/db", "****@tcp(db)/db"},
		{"user:private password=more@tcp(db)/db", "****@tcp(db)/db"},
		{"host=db user=alice password=private dbname=app", "host=db user=alice password=**** dbname=app"},
		{"password = 'two words' host=db", "password = **** host=db"},
		{`host=db password='two\' words' pwd=second`, "host=db password=**** pwd=****"},
		{`password=two\ words host=db`, "password=**** host=db"},
		{"PWD=private", "PWD=****"},
		{"password='unterminated secret", "password=****"},
		{"host=db password='https://user:private@db'", "host=db password=****"},
		{"user@host", "user@host"},
		{"alice@example.com", "alice@example.com"},
		{"host=db user=alice dbname=app", "host=db user=alice dbname=app"},
		{"somepassword=visible", "somepassword=visible"},
		{"https://example.com/path", "https://example.com/path"},
	}
	for _, c := range cases {
		if got := MaskURLCredentials(c.in); got != c.want {
			t.Errorf("MaskURLCredentials(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAuthCredentialKeys(t *testing.T) {
	for key, want := range map[string]bool{
		"BASIC_AUTH": true, "HTTP_BASIC_AUTH": true, "basic-auth": true,
		"AUTH": true, "JWT": true, "SERVICE_JWT": true,
		"AUTH_ENABLED": false, "AUTH_JWT_ALGO": false, "AUTH_JWT_TYPE": false,
		"HASURA_GRAPHQL_AUTH_HOOK": false, "HASURA_GRAPHQL_AUTH_HOOK_MODE": false,
		"HASURA_GRAPHQL_UNAUTHORIZED_ROLE": false, "AUTH_SMTP_HOST": false,
	} {
		if got := IsSecretKey(key); got != want {
			t.Errorf("IsSecretKey(%q) = %v, want %v", key, got, want)
		}
	}
}
