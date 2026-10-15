package verify

// Purpose: compare portable auth state and resolve defaults for older bundles.
// Inputs: decoded source and target users. Outputs: equality or legacy defaults.
// Constraints: comparisons never expose metadata, role names or credentials.
import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
)

// EffectiveDefaultRole resolves bundles predating the additive auth fields.
func (u User) EffectiveDefaultRole() string {
	if u.DefaultRole == "" {
		return "user"
	}
	return u.DefaultRole
}

// EffectiveLocale resolves the locale used by the loader for legacy bundles.
func (u User) EffectiveLocale() string {
	if u.Locale == "" {
		return "en"
	}
	return u.Locale
}

func sameAuth(want, got User) bool {
	if want.EffectiveDefaultRole() != got.DefaultRole || want.EffectiveLocale() != got.Locale || want.EmailVerified != got.EmailVerified || want.Disabled != got.Disabled || want.IsAnonymous != got.IsAnonymous || want.PhoneNumberVerified != got.PhoneNumberVerified {
		return false
	}
	roles := func(in []string) []string {
		out := slices.Clone(in)
		slices.Sort(out)
		return slices.Compact(out)
	}
	if !slices.Equal(roles(want.Roles), roles(got.Roles)) {
		return false
	}
	return sameMetadata(want.Metadata, got.Metadata)
}

// observeAuthRows records independently queried users, including role memberships.
func observeAuthRows(rows pgx.Rows, state *State) error {
	for rows.Next() {
		var raw []byte
		var u User
		if rows.Scan(&raw) != nil {
			return errors.New("target user read failed")
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if dec.Decode(&u) != nil {
			return errors.New("target user read failed")
		}
		u.HashAlgorithm = "none"
		if u.PasswordHash != nil && len(*u.PasswordHash) >= 4 && (*u.PasswordHash)[:2] == "$2" {
			u.HashAlgorithm = "bcrypt"
		}
		state.Users[u.ID] = u.HashAlgorithm
		state.UserIdent[u.ID] = Identity(u.Email, u.Phone, u.DisplayName)
		u.PasswordHash = nil
		state.UserAuth[u.ID] = u
	}
	if rows.Err() != nil {
		return errors.New("target user read failed")
	}
	return nil
}
