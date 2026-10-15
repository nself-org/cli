package verify

import (
	"reflect"
	"slices"
)

// Purpose: preserve the primary role while accepting legacy portable users.
// Inputs: a portable user. Outputs: its default role, historically user when omitted.
// Constraints: an explicit role is never replaced with a more privileged fallback.
func (u User) EffectiveDefaultRole() string {
	if u.DefaultRole == "" {
		return "user"
	}
	return u.DefaultRole
}

// Purpose: detect authorization drift without exposing user metadata in reports.
// Inputs: independent bundle/target users. Outputs: equality of portable attributes.
// Constraints: roles are unordered; NULL metadata may load as an empty NOT NULL object.
func sameUserAttributes(want, got User) bool {
	wantRoles, gotRoles := slices.Clone(want.Roles), slices.Clone(got.Roles)
	slices.Sort(wantRoles)
	slices.Sort(gotRoles)
	metadataEqual := len(want.Metadata) == 0 && len(got.Metadata) == 0 || reflect.DeepEqual(want.Metadata, got.Metadata)
	return want.Disabled == got.Disabled && want.EmailVerified == got.EmailVerified &&
		want.EffectiveDefaultRole() == got.DefaultRole && slices.Equal(wantRoles, gotRoles) && metadataEqual
}
