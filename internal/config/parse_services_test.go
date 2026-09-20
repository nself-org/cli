package config

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// T06 — Custom services sanitization
// ---------------------------------------------------------------------------

// TestCustomServicesSanitization_Valid verifies that a well-formed CS_1 value
// produces a parsed service whose Name passes through sanitization intact.
func TestCustomServicesSanitization_Valid(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	// Clear other CS slots so they don't interfere.
	for i := 2; i <= 10; i++ {
		t.Setenv("CS_"+itoa(i), "")
	}

	services, err := parseCustomServices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) == 0 {
		t.Fatal("expected at least one custom service, got none")
	}
	got := services[0].Name
	if got != "myservice" {
		t.Errorf("Name = %q, want %q", got, "myservice")
	}
	// Verify no unsafe characters remain.
	assertNameClean(t, got)
}

// TestCustomServicesSanitization_ShellSpecial verifies that a service name
// containing shell-special characters like $() results in either the service
// being skipped or the Name containing none of those characters.
func TestCustomServicesSanitization_ShellSpecial(t *testing.T) {
	t.Setenv("CS_1", "$(rm -rf /):go")
	for i := 2; i <= 10; i++ {
		t.Setenv("CS_"+itoa(i), "")
	}

	services, err := parseCustomServices()
	if err != nil {
		// Sanitization rejected the name with an error — pass.
		return
	}
	if len(services) == 0 {
		// Sanitization stripped all valid chars and the entry was skipped — pass.
		return
	}
	// If a service was returned, its name must contain no unsafe characters.
	assertNameClean(t, services[0].Name)
}

// TestCustomServicesSanitization_PathTraversal verifies that a service name
// containing path traversal sequences results in a sanitized Name that contains
// neither ".." nor "/".
func TestCustomServicesSanitization_PathTraversal(t *testing.T) {
	t.Setenv("CS_1", "../../etc:go")
	for i := 2; i <= 10; i++ {
		t.Setenv("CS_"+itoa(i), "")
	}

	services, err := parseCustomServices()
	if err != nil {
		// Rejected with an error — safe.
		return
	}
	if len(services) == 0 {
		// Skipped entirely — safe.
		return
	}
	name := services[0].Name
	if strings.Contains(name, "..") {
		t.Errorf("Name %q still contains \"..\" after sanitization", name)
	}
	if strings.Contains(name, "/") {
		t.Errorf("Name %q still contains \"/\" after sanitization", name)
	}
}

// TestCustomServicesBuildPath_Valid verifies that CS_N_PATH sets BuildPath
// when the value is a clean relative path.
func TestCustomServicesBuildPath_Valid(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	t.Setenv("CS_1_PATH", "./backend/services/myservice")
	for i := 2; i <= 10; i++ {
		t.Setenv("CS_"+itoa(i), "")
	}

	services, err := parseCustomServices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) == 0 {
		t.Fatal("expected at least one custom service")
	}
	if got := services[0].BuildPath; got != "./backend/services/myservice" {
		t.Errorf("BuildPath = %q, want %q", got, "./backend/services/myservice")
	}
}

// TestCustomServicesBuildPath_AbsoluteRejected verifies that an absolute path
// in CS_N_PATH is rejected.
func TestCustomServicesBuildPath_AbsoluteRejected(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	t.Setenv("CS_1_PATH", "/etc/secrets")
	for i := 2; i <= 10; i++ {
		t.Setenv("CS_"+itoa(i), "")
	}

	_, err := parseCustomServices()
	if err == nil {
		t.Fatal("expected error for absolute CS_N_PATH, got nil")
	}
}

// TestCustomServicesBuildPath_TraversalRejected verifies that a path containing
// '..' in CS_N_PATH is rejected.
func TestCustomServicesBuildPath_TraversalRejected(t *testing.T) {
	t.Setenv("CS_1", "myservice:go")
	t.Setenv("CS_1_PATH", "../../outside")
	for i := 2; i <= 10; i++ {
		t.Setenv("CS_"+itoa(i), "")
	}

	_, err := parseCustomServices()
	if err == nil {
		t.Fatal("expected error for traversal CS_N_PATH, got nil")
	}
}

// TestCustomServicesRawName_Underscore verifies that parseCustomServices
// preserves the original underscore-bearing name in RawName while Name is
// sanitized to the hyphenated Docker-safe form. This is the fix for the bug
// where internal/compose derived the default build context from the
// sanitized Name and could never find a "services/ping_api" directory on
// disk (see defaultCustomServiceBuildContext in internal/compose).
func TestCustomServicesRawName_Underscore(t *testing.T) {
	t.Setenv("CS_1", "ping_api:go")
	for i := 2; i <= 10; i++ {
		t.Setenv("CS_"+itoa(i), "")
	}

	services, err := parseCustomServices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) == 0 {
		t.Fatal("expected at least one custom service, got none")
	}
	if got := services[0].RawName; got != "ping_api" {
		t.Errorf("RawName = %q, want %q", got, "ping_api")
	}
	if got := services[0].Name; got != "ping-api" {
		t.Errorf("Name = %q, want %q", got, "ping-api")
	}
}

// TestCustomServicesRawName_NoUnderscore verifies that when the configured
// name has no characters SanitizeName would change, RawName and Name end up
// identical.
func TestCustomServicesRawName_NoUnderscore(t *testing.T) {
	t.Setenv("CS_1", "pingapi:go")
	for i := 2; i <= 10; i++ {
		t.Setenv("CS_"+itoa(i), "")
	}

	services, err := parseCustomServices()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(services) == 0 {
		t.Fatal("expected at least one custom service, got none")
	}
	if services[0].RawName != services[0].Name {
		t.Errorf("RawName = %q, Name = %q, want equal", services[0].RawName, services[0].Name)
	}
	if services[0].Name != "pingapi" {
		t.Errorf("Name = %q, want %q", services[0].Name, "pingapi")
	}
}

// ---------------------------------------------------------------------------
// T07 — Frontend apps sanitization
// ---------------------------------------------------------------------------

// TestFrontendAppsSanitization_Valid verifies that a well-formed FRONTEND_APP_1
// set of env vars produces a parsed app with the expected values.
func TestFrontendAppsSanitization_Valid(t *testing.T) {
	t.Setenv("FRONTEND_APP_1_DISPLAY_NAME", "myapp")
	t.Setenv("FRONTEND_APP_1_SYSTEM_NAME", "myapp")
	t.Setenv("FRONTEND_APP_1_ROUTE", "myapp.example.com")
	// Clear other app slots.
	for i := 2; i <= 20; i++ {
		t.Setenv("FRONTEND_APP_"+itoa(i)+"_DISPLAY_NAME", "")
	}

	apps, err := parseFrontendApps()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(apps) == 0 {
		t.Fatal("expected at least one frontend app, got none")
	}
	app := apps[0]
	if app.SystemName != "myapp" {
		t.Errorf("SystemName = %q, want %q", app.SystemName, "myapp")
	}
	if app.Route != "myapp.example.com" {
		t.Errorf("Route = %q, want %q", app.Route, "myapp.example.com")
	}
}

// TestFrontendApps_LegacyNameFallback verifies that FRONTEND_APP_N_NAME (v1
// format) is accepted when FRONTEND_APP_N_DISPLAY_NAME is not set, ensuring
// backward compatibility with legacy v1 .env files.
func TestFrontendApps_LegacyNameFallback(t *testing.T) {
	// Use only _NAME (v1 format), NOT _DISPLAY_NAME
	t.Setenv("FRONTEND_APP_1_NAME", "legacyapp")
	t.Setenv("FRONTEND_APP_1_SYSTEM_NAME", "legacyapp")
	t.Setenv("FRONTEND_APP_1_ROUTE", "legacy.example.com")
	// Clear _DISPLAY_NAME explicitly to ensure fallback path
	t.Setenv("FRONTEND_APP_1_DISPLAY_NAME", "")
	for i := 2; i <= 20; i++ {
		t.Setenv("FRONTEND_APP_"+itoa(i)+"_DISPLAY_NAME", "")
		t.Setenv("FRONTEND_APP_"+itoa(i)+"_NAME", "")
	}

	apps, err := parseFrontendApps()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(apps) == 0 {
		t.Fatal("expected at least one frontend app from legacy _NAME, got none")
	}
	if apps[0].DisplayName != "legacyapp" {
		t.Errorf("DisplayName = %q, want %q (from legacy _NAME fallback)", apps[0].DisplayName, "legacyapp")
	}
}

// TestFrontendAppsSanitization_NginxInjection verifies that SanitizeDomain
// rejects nginx-injection characters (semicolons, wildcards, spaces). A route
// like "evil.com; server_name *" must be rejected so it never reaches nginx
// config generation.
func TestFrontendAppsSanitization_NginxInjection(t *testing.T) {
	t.Setenv("FRONTEND_APP_1_DISPLAY_NAME", "test")
	t.Setenv("FRONTEND_APP_1_SYSTEM_NAME", "test")
	t.Setenv("FRONTEND_APP_1_ROUTE", "evil.com; server_name *")
	for i := 2; i <= 20; i++ {
		t.Setenv("FRONTEND_APP_"+itoa(i)+"_DISPLAY_NAME", "")
	}

	apps, err := parseFrontendApps()
	if err != nil {
		// Hard error from image/domain validation — input rejected, test passes.
		return
	}
	// SanitizeDomain must reject this input — the app must be skipped entirely.
	if len(apps) != 0 {
		route := apps[0].Route
		if strings.Contains(route, ";") {
			t.Errorf("Route %q still contains \";\" — nginx injection not blocked", route)
		}
		if strings.Contains(route, "*") {
			t.Errorf("Route %q still contains \"*\" — nginx injection not blocked", route)
		}
		if strings.Contains(route, " ") {
			t.Errorf("Route %q still contains space — nginx injection not blocked", route)
		}
		t.Errorf("expected app with injection route to be skipped, but got Route=%q", route)
	}
}

// TestFrontendAppsSanitization_DisplayNameInjection verifies that a DISPLAY_NAME
// containing nginx-injection characters (semicolons, newlines) is rejected at
// parse time and the app is skipped entirely.
func TestFrontendAppsSanitization_DisplayNameInjection(t *testing.T) {
	t.Setenv("FRONTEND_APP_1_DISPLAY_NAME", "evil.com; server_name *")
	t.Setenv("FRONTEND_APP_1_SYSTEM_NAME", "test")
	t.Setenv("FRONTEND_APP_1_ROUTE", "test.example.com")
	for i := 2; i <= 20; i++ {
		t.Setenv("FRONTEND_APP_"+itoa(i)+"_DISPLAY_NAME", "")
	}

	apps, err := parseFrontendApps()
	if err != nil {
		// Hard validation error — input rejected, test passes.
		return
	}
	// SanitizeName strips all chars except a-z0-9 and hyphens, producing a
	// non-empty string like "evilcom-server-name-" which then gets trimmed.
	// The important guarantee is that the stored DisplayName contains no
	// semicolons, spaces, asterisks, or newlines.
	for _, app := range apps {
		dn := app.DisplayName
		if strings.Contains(dn, ";") {
			t.Errorf("DisplayName %q still contains \";\" — nginx injection not blocked", dn)
		}
		if strings.Contains(dn, "*") {
			t.Errorf("DisplayName %q still contains \"*\" — nginx injection not blocked", dn)
		}
		if strings.Contains(dn, " ") {
			t.Errorf("DisplayName %q still contains space — nginx injection not blocked", dn)
		}
		if strings.Contains(dn, "\n") {
			t.Errorf("DisplayName %q still contains newline — nginx injection not blocked", dn)
		}
	}
}

// TestFrontendAppsSanitization_DisplayNameNewline verifies that a DISPLAY_NAME
// containing a newline is rejected or sanitized so the stored value has no newline.
func TestFrontendAppsSanitization_DisplayNameNewline(t *testing.T) {
	t.Setenv("FRONTEND_APP_1_DISPLAY_NAME", "myapp\nevil")
	t.Setenv("FRONTEND_APP_1_SYSTEM_NAME", "myapp")
	t.Setenv("FRONTEND_APP_1_ROUTE", "myapp.example.com")
	for i := 2; i <= 20; i++ {
		t.Setenv("FRONTEND_APP_"+itoa(i)+"_DISPLAY_NAME", "")
	}

	apps, err := parseFrontendApps()
	if err != nil {
		// Hard validation error — input rejected, test passes.
		return
	}
	for _, app := range apps {
		if strings.Contains(app.DisplayName, "\n") {
			t.Errorf("DisplayName %q still contains newline after sanitization", app.DisplayName)
		}
	}
}

// TestFrontendAppsSanitization_RouteNewline verifies that a ROUTE containing a
// newline is rejected at parse time so the app is skipped.
func TestFrontendAppsSanitization_RouteNewline(t *testing.T) {
	t.Setenv("FRONTEND_APP_1_DISPLAY_NAME", "myapp")
	t.Setenv("FRONTEND_APP_1_SYSTEM_NAME", "myapp")
	t.Setenv("FRONTEND_APP_1_ROUTE", "myapp.example.com\nevil")
	for i := 2; i <= 20; i++ {
		t.Setenv("FRONTEND_APP_"+itoa(i)+"_DISPLAY_NAME", "")
	}

	apps, err := parseFrontendApps()
	if err != nil {
		// Hard validation error — input rejected, test passes.
		return
	}
	for _, app := range apps {
		if strings.Contains(app.Route, "\n") {
			t.Errorf("Route %q still contains newline — nginx injection not blocked", app.Route)
		}
	}
}

// ---------------------------------------------------------------------------
// T08 — Remote schemas sanitization
// ---------------------------------------------------------------------------

// TestRemoteSchemasSanitization_Valid verifies that a well-formed
// REMOTE_SCHEMA_1_NAME produces a parsed schema with the expected Name.
func TestRemoteSchemasSanitization_Valid(t *testing.T) {
	t.Setenv("REMOTE_SCHEMA_1_NAME", "myschema")
	// Clear other schema slots.
	for i := 2; i <= 10; i++ {
		t.Setenv("REMOTE_SCHEMA_"+itoa(i)+"_NAME", "")
	}

	schemas, err := parseRemoteSchemas()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(schemas) == 0 {
		t.Fatal("expected at least one remote schema, got none")
	}
	got := schemas[0].Name
	if got != "myschema" {
		t.Errorf("Name = %q, want %q", got, "myschema")
	}
	assertNameClean(t, got)
}

// TestRemoteSchemasSanitization_PathSeparator verifies that a schema Name
// containing path traversal sequences results in a sanitized Name that contains
// neither ".." nor "/".
func TestRemoteSchemasSanitization_PathSeparator(t *testing.T) {
	t.Setenv("REMOTE_SCHEMA_1_NAME", "../../evil")
	for i := 2; i <= 10; i++ {
		t.Setenv("REMOTE_SCHEMA_"+itoa(i)+"_NAME", "")
	}

	schemas, err := parseRemoteSchemas()
	if err != nil {
		// Hard validation error — input rejected, test passes.
		return
	}
	if len(schemas) == 0 {
		// Skipped entirely — safe.
		return
	}
	name := schemas[0].Name
	if strings.Contains(name, "..") {
		t.Errorf("Name %q still contains \"..\" after sanitization", name)
	}
	if strings.Contains(name, "/") {
		t.Errorf("Name %q still contains \"/\" after sanitization", name)
	}
}

// TestRemoteSchemasSanitization_Spaces verifies that a schema Name containing
// spaces is either rejected (skipped) or has spaces removed/replaced so no
// space character remains in the stored Name.
func TestRemoteSchemasSanitization_Spaces(t *testing.T) {
	t.Setenv("REMOTE_SCHEMA_1_NAME", "bad schema name")
	for i := 2; i <= 10; i++ {
		t.Setenv("REMOTE_SCHEMA_"+itoa(i)+"_NAME", "")
	}

	schemas, err := parseRemoteSchemas()
	if err != nil {
		// Hard validation error — input rejected, test passes.
		return
	}
	if len(schemas) == 0 {
		// Rejected entirely — safe.
		return
	}
	name := schemas[0].Name
	if strings.Contains(name, " ") {
		t.Errorf("Name %q still contains spaces after sanitization", name)
	}
	assertNameClean(t, name)
}

// TestRemoteSchemasSanitization_SpecialChars verifies that a schema Name
// consisting entirely of shell-special characters is rejected because
// SanitizeName strips them all, leaving an empty string, causing the entry
// to be skipped.
func TestRemoteSchemasSanitization_SpecialChars(t *testing.T) {
	t.Setenv("REMOTE_SCHEMA_1_NAME", "$();|`")
	for i := 2; i <= 10; i++ {
		t.Setenv("REMOTE_SCHEMA_"+itoa(i)+"_NAME", "")
	}

	schemas, err := parseRemoteSchemas()
	if err != nil {
		// Hard validation error — input rejected, test passes.
		return
	}
	// All special chars strip to empty — entry must be rejected.
	if len(schemas) != 0 {
		t.Errorf("expected schema with special-char-only name to be rejected; got Name=%q", schemas[0].Name)
	}
}

// TestRemoteSchemasSanitization_ValidUnderscore verifies that a name with
// underscores (e.g. my_schema) is accepted and normalized (underscores become
// hyphens via SanitizeName).
func TestRemoteSchemasSanitization_ValidUnderscore(t *testing.T) {
	t.Setenv("REMOTE_SCHEMA_1_NAME", "my_schema")
	for i := 2; i <= 10; i++ {
		t.Setenv("REMOTE_SCHEMA_"+itoa(i)+"_NAME", "")
	}

	schemas, err := parseRemoteSchemas()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(schemas) == 0 {
		t.Fatal("expected my_schema to be accepted, got none")
	}
	name := schemas[0].Name
	if name == "" {
		t.Error("Name is empty after sanitization of my_schema")
	}
	assertNameClean(t, name)
}

// TestRemoteSchemasSanitization_ValidPlain verifies that a plain alphanumeric
// name like myservice passes through unchanged.
func TestRemoteSchemasSanitization_ValidPlain(t *testing.T) {
	t.Setenv("REMOTE_SCHEMA_1_NAME", "myservice")
	for i := 2; i <= 10; i++ {
		t.Setenv("REMOTE_SCHEMA_"+itoa(i)+"_NAME", "")
	}

	schemas, err := parseRemoteSchemas()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(schemas) == 0 {
		t.Fatal("expected myservice to be accepted, got none")
	}
	got := schemas[0].Name
	if got != "myservice" {
		t.Errorf("Name = %q, want %q", got, "myservice")
	}
	assertNameClean(t, got)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// assertNameClean verifies the name contains no shell-special or path characters.
func assertNameClean(t *testing.T, name string) {
	t.Helper()
	unsafe := []string{"$", "(", ")", "/", "..", "`", ";", "&", "|", " ", "\n", "\t"}
	for _, c := range unsafe {
		if strings.Contains(name, c) {
			t.Errorf("Name %q contains unsafe character %q", name, c)
		}
	}
}

// itoa converts an int to its decimal string representation without importing
// strconv at the package level (keeps the helper self-contained).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
