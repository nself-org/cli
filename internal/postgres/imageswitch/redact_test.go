package imageswitch

import "testing"

func TestRedactRestorePayloads(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"double quote", "pg_restore: error: COPY failed: ERROR: invalid input syntax for type integer: \"private-value\"\nprivate-continuation\nCommand was: COPY users FROM stdin;", "pg_restore: error: COPY failed: ERROR: invalid input syntax for type integer:"},
		{"single quote", "ERROR: invalid input syntax: 'private-value'\nCommand was: INSERT INTO users VALUES (42);", "ERROR: invalid input syntax:"},
		{"command payload", "restore failed\nCommand was: INSERT INTO users\nVALUES (123456789);\n", "restore failed"},
		{"unquoted error", "ERROR: permission denied for table users\nCommand was: SELECT 123456789;", "ERROR: permission denied for table users"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactTail(tc.input, 4); got != tc.want {
				t.Errorf("redactTail() = %q, want %q", got, tc.want)
			}
		})
	}
}
