package controlplane

import (
	"strings"
	"testing"
)

func TestDataPolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		env     Environment
		envName string
		wantErr string
	}{
		{
			name:    "unknown rule",
			envName: "staging",
			env: Environment{
				Name: "staging",
				Tier: TierLocalServers,
				DataPolicy: &DataPolicy{
					Mask: []MaskRule{{Table: "users", Column: "email", Rule: "unknown_rule"}},
				},
			},
			wantErr: "unknown rule",
		},
		{
			name:    "accept on prod",
			envName: "prod",
			env: Environment{
				Name: "prod",
				Tier: TierProd,
				DataPolicy: &DataPolicy{
					Accept: []string{"seed"},
				},
			},
			wantErr: "prod tier must not set accept",
		},
		{
			name:    "mask on tenant_id",
			envName: "staging",
			env: Environment{
				Name: "staging",
				Tier: TierLocalServers,
				DataPolicy: &DataPolicy{
					Mask: []MaskRule{{Table: "users", Column: "tenant_id", Rule: "null"}},
				},
			},
			wantErr: "tenant_id",
		},
		{
			name:    "mask on source_account_id",
			envName: "staging",
			env: Environment{
				Name: "staging",
				Tier: TierLocalServers,
				DataPolicy: &DataPolicy{
					Mask: []MaskRule{{Table: "users", Column: "source_account_id", Rule: "null"}},
				},
			},
			wantErr: "source_account_id",
		},
		{
			name:    "mask on id",
			envName: "staging",
			env: Environment{
				Name: "staging",
				Tier: TierLocalServers,
				DataPolicy: &DataPolicy{
					Mask: []MaskRule{{Table: "users", Column: "id", Rule: "null"}},
				},
			},
			wantErr: "id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDataPolicy(tt.env, tt.envName)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestDataPolicyDefaults(t *testing.T) {
	env := Environment{
		Name: "staging",
		Tier: TierLocalServers,
	}
	accept := EffectiveAccept(env)
	if len(accept) != 1 || accept[0] != "seed" {
		t.Errorf("expected [seed] for staging with no policy, got %v", accept)
	}

	prodEnv := Environment{
		Name: "prod",
		Tier: TierProd,
	}
	prodAccept := EffectiveAccept(prodEnv)
	if len(prodAccept) != 0 {
		t.Errorf("expected empty accept for prod with no policy, got %v", prodAccept)
	}
}
