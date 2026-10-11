package commands

// Purpose: verify recipient command argument and class guards.
// Inputs: fake recipient arguments.
// Outputs: assertions on typed refusals.
// Constraints: no secret store access before guards.
// SPORT: P7-TRUST-03.

import (
	"bytes"
	"context"
	"testing"
)

// TestSecretsRecipientsGuards verifies the secret contract.
func TestSecretsRecipientsGuards(t *testing.T) {
	hub := newSecretsRecipientsCmd()
	for _, action := range []string{"add", "remove", "list"} {
		c, _, err := hub.Find([]string{action})
		if err != nil || c.Name() != action {
			t.Fatal("missing recipient leaf")
		}
		if err = c.Flags().Set("file", "team"); err != nil {
			t.Fatal(err)
		}
		c.SetContext(context.Background())
		c.SetOut(&bytes.Buffer{})
		args := []string{"fake-recipient-trust03"}
		if action == "list" {
			args = nil
		}
		resolveErrorCode(t, runSecretsRecipients(c, args), "E540")
		if action != "list" {
			if err = c.Flags().Set("file", "prod"); err != nil {
				t.Fatal(err)
			}
			resolveErrorCode(t, runSecretsRecipients(c, args), "E544")
		}
	}
}
