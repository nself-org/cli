package verify

// Purpose: one canonical form of a user's nullable identity fields.
// Inputs: email, phone, display name (nil means SQL NULL). Outputs: a comparable string.
// Constraints: NULL and empty text stay distinct for email and phone, because hasura-auth
// keeps email unique; a NULL display name equals "" because the column may be NOT NULL.
func Identity(email, phone, display *string) string {
	field := func(p *string) string {
		if p == nil {
			return "N"
		}
		return "V" + *p
	}
	name := ""
	if display != nil {
		name = *display
	}
	return field(email) + "\x00" + field(phone) + "\x00" + name
}

// authMatches reports whether a target user carries the auth state the bundle
// record asks for: the same default role (empty reads as DefaultRoleFallback),
// email_verified, disabled and role set. A target role beyond the bundle's is a
// mismatch, except the default role itself, which a target may add on its own.
func authMatches(want User, got UserAuth) bool {
	wantDefault := want.DefaultRole
	if wantDefault == "" {
		wantDefault = DefaultRoleFallback
	}
	if got.DefaultRole != wantDefault || got.EmailVerified != want.EmailVerified || got.Disabled != want.Disabled {
		return false
	}
	have := map[string]bool{}
	for _, r := range got.Roles {
		have[r] = true
	}
	expect := map[string]bool{}
	for _, r := range want.Roles {
		expect[r] = true
		if !have[r] {
			return false
		}
	}
	for r := range have {
		if !expect[r] && r != wantDefault {
			return false
		}
	}
	return true
}
