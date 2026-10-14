package commands

// Purpose: rotate a symmetric key embedded in a Hasura JWT JSON value.
// Inputs: project root, secrets environment, key name and window mode.
// Outputs: a replacement JSON value in the encrypted store, or a safe error.
// Constraints: retain every field except key; refuse asymmetric/JWK configs
// and dual-window JSON rotation before changing the store.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/nself-org/cli/internal/secrets"
)

func rotateCommandSecret(root, env, name string, dualWindow bool) (string, error) {
	if name != "HASURA_GRAPHQL_JWT_SECRET" {
		if dualWindow {
			return "", secrets.RotateDualWindow(root, env, name)
		}
		return secrets.Rotate(root, env, name)
	}
	old, err := secrets.Get(root, env, name)
	if err != nil {
		return "", err
	}
	value, err := rotatedJWTJSON(old)
	if err != nil {
		return "", err
	}
	if dualWindow {
		return "", fmt.Errorf("JSON JWT rotation does not support --dual-window; existing value left unchanged")
	}
	if err := secrets.Set(root, env, name, value); err != nil {
		return "", err
	}
	return value, nil
}

func rotatedJWTJSON(value string) (string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &fields); err != nil || fields == nil {
		return "", fmt.Errorf("HASURA_GRAPHQL_JWT_SECRET must be a JSON object; existing value left unchanged")
	}
	var algorithm, oldKey string
	if json.Unmarshal(fields["type"], &algorithm) != nil ||
		json.Unmarshal(fields["key"], &oldKey) != nil || oldKey == "" {
		return "", fmt.Errorf("JWT JSON requires string type and nonempty key fields; existing value left unchanged")
	}
	if algorithm != "HS256" && algorithm != "HS384" && algorithm != "HS512" {
		return "", fmt.Errorf("JWT type requires provider rotation; only HS256, HS384 and HS512 can rotate locally (existing value left unchanged)")
	}
	key := make([]byte, 64)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generating JWT key: %w", err)
	}
	fields["key"], _ = json.Marshal(hex.EncodeToString(key))
	encoded, err := json.Marshal(fields)
	return string(encoded), err
}
