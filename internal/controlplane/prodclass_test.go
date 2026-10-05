package controlplane

import "testing"

// TestIsProdClass: prod and production are prod-class in any case; anything
// else (including empty and a prod-looking suffix) is not, until the tier
// lookup of P7-DEPL-13 lands.
func TestIsProdClass(t *testing.T) {
	cases := map[string]bool{
		"prod": true, "production": true, "PROD": true, " Production ": true,
		"staging": false, "qa": false, "local": false, "": false,
		"prod-eu": false, "preprod": false,
	}
	for env, want := range cases {
		if got := IsProdClass(nil, env); got != want {
			t.Errorf("IsProdClass(nil, %q) = %v, want %v", env, got, want)
		}
		if got := IsProdClass(threeEnvInventory(), env); got != want {
			t.Errorf("IsProdClass(inv, %q) = %v, want %v", env, got, want)
		}
	}
}
