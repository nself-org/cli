package config

import "testing"

// TestIsSecretKey pins IsSecretKey: the basis rule of cmd/commands isSecretKey
// at cli f6c45e2f (SECRET, PASSWORD, KEY, TOKEN) widened by P7-SURF-12 with
// PASS, CREDENTIAL, WEBHOOK, PRIVATE, DSN and SALT, for 30 keys.
func TestIsSecretKey(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"POSTGRES_PASSWORD", true},
		{"HASURA_GRAPHQL_ADMIN_SECRET", true},
		{"HASURA_GRAPHQL_JWT_SECRET", true},
		{"STRIPE_SECRET_KEY", true},
		{"NSELF_PLUGIN_LICENSE_KEY", true},
		{"NSELF_MCP_TOKEN", true},
		{"MINIO_ROOT_PASSWORD", true},
		{"S3_ACCESS_KEY", true},
		{"postgres_password", true},
		{"value", false},
		{"api-key", true},
		{"monkey", true},
		{"tokens", true},
		{"PROJECT_NAME", false},
		{"ENV", false},
		{"BASE_DOMAIN", false},
		{"POSTGRES_USER", false},
		{"HASURA_GRAPHQL_ENABLE_CONSOLE", false},
		{"PASSWD", true},
		{"", false},
		{"AUTH_SMTP_PASS", true},
		{"SLACK_WEBHOOK_URL", true},
		{"SENTRY_DSN", true},
		{"TLS_PRIVATE_DIR", true},
		{"PASSWORD_SALT", true},
		{"DB_CREDENTIAL_FILE", true},
		{"AUTH_JWT_ALGO", false},
		{"HASURA_GRAPHQL_ENABLE_CONSOLE", false},
		{"SMTP_HOST", false},
		{"BASE_URL", false},
	}
	if len(cases) != 30 {
		t.Fatalf("table has %d keys, want 30", len(cases))
	}
	for _, c := range cases {
		if got := IsSecretKey(c.key); got != c.want {
			t.Errorf("IsSecretKey(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}

// TestMaskURLCredentials: user and password are masked up to the last @, so a
// password with @, /, : or a space never leaks; query, matrix and fragment
// parameters are matched on their percent-decoded name (D-0430).
func TestMaskURLCredentials(t *testing.T) {
	cases := []struct{ in, want string }{
		{"postgres://app:pw@db:5432/app", "postgres://****@db:5432/app"},
		{"postgres://app:FAKE@AT444@db/x", "postgres://****@db/x"},
		{"redis://u:FAKE/SL555@cache:6379", "redis://****@cache:6379"},
		{"amqp://u:FAKE SP666@mq/vhost", "amqp://****@mq/vhost"},
		{"mongodb://u:a:b:c@h/db?x=1", "mongodb://****@h/db?x=1"},
		{"postgresql://postgres:ab/cd+ef@postgres/db", "postgresql://****@postgres/db"},
		{"postgres://u:p@ss@h/db", "postgres://****@h/db"},
		{"postgres://h/db?sslmode=require&password=x", "postgres://h/db?sslmode=require&password=****"},
		{"https://h/p?SSLPassword=x&API_KEY=y#f", "https://h/p?SSLPassword=****&API_KEY=****#f"},
		{"https://example.test/p?access_token=FAKETOK2&x=1", "https://example.test/p?access_token=****&x=1"},
		{"https://h/cb?client_secret=FAKE3;api-key=FAKE4", "https://h/cb?client_secret=****;api-key=****"},
		{"https://s3.test/o?X-Amz-Signature=FAKE5&auth=FAKE6", "https://s3.test/o?X-Amz-Signature=****&auth=****"},
		{"https://h/p?page=2&sort=name", "https://h/p?page=2&sort=name"},
		// D-0430: percent-encoded names and fragment parameters.
		{"https://example.test/?access%5Ftoken=FAKETOK7", "https://example.test/?access%5Ftoken=****"},
		{"https://example.test/#access_token=FAKETOK8&state=s1", "https://example.test/#access_token=****&state=s1"},
		{"https://example.test/cb#state=s1&id%5Ftoken=FAKE9", "https://example.test/cb#state=s1&id%5Ftoken=****"},
		{"https://h/?access%255Ftoken=FAKE10", "https://h/?access%255Ftoken=****"},
		{"https://h/?%61uth=FAKE11&x=1", "https://h/?%61uth=****&x=1"},
		{"https://h/?%zz%74oken=FAKE12", "https://h/?%zz%74oken=****"},
		{"https://h/#/route?refresh_token=FAKE13", "https://h/#/route?refresh_token=****"},
		{"https://h/p;Api%2DKey=FAKE14", "https://h/p;Api%2DKey=****"},
		{"https://h/p?page=2#section=intro", "https://h/p?page=2#section=intro"},
		{"https://h/?q=%5Ftoken&next=%2Fpass", "https://h/?q=%5Ftoken&next=%2Fpass"},
		{"https://h/?%5F=1", "https://h/?%5F=1"},
		{"https://h/?redirect=https://x/?token=FAKE15", "https://h/?redirect=https://x/?token=****"},
		{"https://h/?foo=bar?access%5Ftoken=FAKE16&y=2", "https://h/?foo=bar?access%5Ftoken=****&y=2"},
		{"https://h/?access_token%3DFAKE17&y=2", "https://h/?access_token%3D****&y=2"},
		{"https://h/?token=FAKE SP18&y=2", "https://h/?token=****&y=2"},
		{"https://h/?token=FAKE19?refresh_token=FAKE20#x", "https://h/?token=****#x"},
		{"https://example.test/path", "https://example.test/path"},
		{"user@example.test", "user@example.test"},
		{"plain-value", "plain-value"},
		{"", ""},
	}
	for _, c := range cases {
		if got := MaskURLCredentials(c.in); got != c.want {
			t.Errorf("MaskURLCredentials(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
