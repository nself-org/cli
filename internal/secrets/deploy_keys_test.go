package secrets

// Purpose: prove DeployKeys names the store's keys (sorted) on the local-store
// path and agrees with DecryptForDeploy, including for a multi-line value.
// Inputs: fake values and an ephemeral age identity (trustFixture).
// Outputs: test failures. Skipped when age is not installed, as its siblings.
// Constraints: no real secrets.

import (
	"reflect"
	"strings"
	"testing"
)

func TestDeployKeysLocalStore(t *testing.T) {
	root, _, _ := trustFixture(t)
	t.Setenv("DEPLOY_AGE_KEY", "")
	pem := "-----BEGIN FAKE KEY-----\nFakeTailChunkAbc123==\n-----END FAKE KEY-----"
	store := &SecretStore{Secrets: map[string]SecretEntry{
		"ZED_KEY": {Value: trustFakeValue},
		"ALPHA":   {Value: pem},
	}}
	if err := saveStore(root, "dev", store); err != nil {
		t.Fatal(err)
	}
	keys, err := DeployKeys(root, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"ALPHA", "ZED_KEY"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("DeployKeys = %v, want %v", keys, want)
	}
	text, err := DecryptForDeploy(root, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if want := "ALPHA=" + pem + "\nZED_KEY=" + trustFakeValue; text != want {
		t.Errorf("DecryptForDeploy = %q, want %q", text, want)
	}
	if strings.Contains(strings.Join(keys, ","), "FakeTailChunk") {
		t.Error("a value line became a key")
	}
}

func TestDeployKeysNoStore(t *testing.T) {
	root, _, _ := trustFixture(t)
	t.Setenv("DEPLOY_AGE_KEY", "")
	keys, err := DeployKeys(root, "dev")
	if err != nil || len(keys) != 0 {
		t.Fatalf("DeployKeys on a missing store = %v, %v; want empty and nil", keys, err)
	}
}
