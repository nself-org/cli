package config

import "testing"

// D-0743: credentials in values without `scheme://`. Fake values only.
func TestMaskURLCredentials_Schemeless(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"go mysql dsn", "app:fakepw@tcp(db:3306)/app", "****@tcp(db:3306)/app"},
		{"dsn password with @", "app:fa@ke:pw@tcp(db:3306)/app", "****@tcp(db:3306)/app"},
		{"dsn unix socket", "app:fakepw@unix(/run/mysql.sock)/app", "****@unix(/run/mysql.sock)/app"},
		{"dsn after key", "dsn=app:fakepw@tcp(db:3306)/app", "dsn=****@tcp(db:3306)/app"},
		{"dsn in prose", "connect failed for app:fakepw@tcp(db:3306)/app: refused", "connect failed for ****@tcp(db:3306)/app: refused"},
		{"dsn without password", "app@tcp(db:3306)/app", "app@tcp(db:3306)/app"},
		{"bare user:pass@host", "app:fakepw@db.internal:5432/app", "****@db.internal:5432/app"},
		{"libpq keyword", "host=db user=app password=fakepw dbname=app", "host=db user=app password=**** dbname=app"},
		{"libpq quoted", "host=db password='fake pw' dbname=app", "host=db password=**** dbname=app"},
		{"libpq double quoted", `host=db password="fake pw" dbname=app`, `host=db password=**** dbname=app`},
		{"libpq unclosed quote", "host=db password='fake pw", "host=db password=****"},
		{"libpq spaced and cased", "host=db PWD = fakepw", "host=db PWD = ****"},
		{"libpq sslpassword", "sslpassword=fakepw host=db", "sslpassword=**** host=db"},
		{"url unchanged rule", "postgres://app:fakepw@db:5432/app", "postgres://****@db:5432/app"},
		{"url then keyword", "postgres://db/app?x=1 password=fakepw", "postgres://db/app?x=1 password=****"},
		{"url param keeps neighbours", "https://h/?password=fakepw&x=1", "https://h/?password=****&x=1"},
		{"idempotent", "password=****", "password=****"},
		// Must stay as written.
		{"plain user@host", "app@db.internal", "app@db.internal"},
		{"scp target", "git@github.com:org/repo", "git@github.com:org/repo"},
		{"email", "ops@example.com", "ops@example.com"},
		{"word ending in password", "mypassword=fake-keep", "mypassword=fake-keep"},
		{"host:port", "db.internal:5432", "db.internal:5432"},
		{"plain text", "hello world", "hello world"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := MaskURLCredentials(tc.in); got != tc.want {
				t.Errorf("MaskURLCredentials(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsSecretKey_AuthSuffix(t *testing.T) {
	for _, k := range []string{"BASIC_AUTH", "basic_auth", "REDISCLI_AUTH", "DB_AUTH", "AUTH"} {
		if !IsSecretKey(k) {
			t.Errorf("IsSecretKey(%q) = false, want true", k)
		}
	}
	// Hasura/auth config keys that must stay visible.
	for _, k := range []string{"HASURA_AUTH_URL", "AUTH_SERVER_URL", "AUTH_JWT_ALG", "RATE_LIMIT_AUTH_RPS", "POSTGRES_HOST_AUTH_METHOD", "SSH_AUTH_SOCK", "BASIC_AUTH_USER", "HASURA_GRAPHQL_AUTH_HOOK"} {
		if IsSecretKey(k) {
			t.Errorf("IsSecretKey(%q) = true, want false", k)
		}
	}
}

// A caller's own placeholder must survive a second masking pass.
func TestMaskURLCredentials_KeepsExistingPlaceholders(t *testing.T) {
	for _, in := range []string{`password = "[REDACTED]"`, "pwd=****", "password = \x00redacted\x00"} {
		if got := MaskURLCredentials(in); got != in {
			t.Errorf("MaskURLCredentials(%q) = %q, want unchanged", in, got)
		}
	}
}
