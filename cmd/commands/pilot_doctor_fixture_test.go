package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
)

// pilotDoctorProject supplies stable credentials for the live doctor's repeated
// config loads. It keeps the shared config-pilot fixtures and goldens separate.
func pilotDoctorProject(t *testing.T) string {
	t.Helper()
	dir := pilotProject(t)
	path := filepath.Join(dir, ".env")
	env, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Public fixture value, long enough and free of doctor's insecure patterns.
	env = append(env, []byte("HASURA_GRAPHQL_ADMIN_SECRET=XZ8q2fLm-vn4KpR7gHn2v-4Qx9BmCd8Pz6\n")...)
	if err := os.WriteFile(path, env, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestPilotDoctorFixtureSecret prevents random defaults from changing password
// warnings between the envelope and legacy doctor runs (D-0734).
func TestPilotDoctorFixtureSecret(t *testing.T) {
	env, err := godotenv.Read(filepath.Join(pilotDoctorProject(t), ".env"))
	if err != nil {
		t.Fatal(err)
	}
	secret := env["HASURA_GRAPHQL_ADMIN_SECRET"]
	if len(secret) < 32 || isWeakPassword(secret) {
		t.Fatal("doctor fixture must pin a strong Hasura admin secret instead of generating one per load")
	}
}
