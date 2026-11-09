package controlplane

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestRequireExplicitNonProd(t *testing.T) {
	inv := &Inventory{SchemaVersion: 2, Environments: map[string]Environment{
		"local":    {Name: "local", Kind: "local", Tier: TierLocal},
		"staging":  {Name: "staging", Kind: "remote", Tier: TierLocalServers},
		"live":     {Name: "live", Kind: "remote"},
		"stageprd": {Name: "stageprd", Kind: "remote", Tier: TierProd},
		"prod":     {Name: "prod", Kind: "remote", Tier: TierLocalServers},
		"odd":      {Name: "odd", Kind: "remote", Tier: Tier("qa")},
	}}
	inv.Environments["staging-prod"] = Environment{Name: "staging", Kind: "remote", Tier: TierProd}

	cases := []struct {
		name string
		inv  *Inventory
		env  string
		code string // empty means the guard passes
	}{
		{"staging local-servers passes", inv, "staging", ""},
		{"local tier passes", inv, "local", ""},
		{"derived tier refused", inv, "live", "E485"},
		{"staging with tier prod refused", inv, "staging-prod", "E485"},
		{"explicit prod tier refused", inv, "stageprd", "E485"},
		{"prod name with local-servers refused", inv, "prod", "E485"},
		{"unknown tier value refused", inv, "odd", "E485"},
		{"nil inventory refused", nil, "staging", "E483"},
		{"unknown env refused", inv, "nope", "E483"},
		{"empty env refused", inv, "", "E483"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireExplicitNonProd(tc.inv, tc.env)
			if tc.code == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want %s, got nil", tc.code)
			}
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != tc.code {
				t.Fatalf("want code %s, got %v", tc.code, err)
			}
			if tc.env != "" && !strings.Contains(err.Error(), tc.env) {
				t.Errorf("error does not name env %q: %v", tc.env, err)
			}
		})
	}
}
