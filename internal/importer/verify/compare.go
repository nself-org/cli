// Package verify defines the importer's zero-mismatch rule.
// Purpose: compare independent observations. Inputs: manifest and target snapshot.
// Outputs: deterministic item-level mismatches. Constraints: never report content or secrets.
package verify

import (
	"context"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/portable"
	"reflect"
	"sort"
)

type Mismatch struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Detail string `json:"detail"`
}
type User struct {
	ID            string         `json:"id"`
	Email         *string        `json:"email"`
	EmailVerified bool           `json:"email_verified"`
	Phone         *string        `json:"phone"`
	DisplayName   *string        `json:"display_name"`
	CreatedAt     string         `json:"created_at"`
	Disabled      bool           `json:"disabled"`
	PasswordHash  *string        `json:"password_hash"`
	HashAlgorithm string         `json:"hash_algorithm"`
	Metadata      map[string]any `json:"metadata"`
	Roles         []string       `json:"roles"`
	DefaultRole   string         `json:"default_role,omitempty"`
}
type Metadata map[string]map[string]any
type State struct {
	Tables   map[string]portable.Table
	Users    map[string]string
	Objects  map[string]string
	Metadata Metadata
	Counts   map[string]int64
	// UserIdent holds Identity() of each target user; nil skips the identity check.
	UserIdent map[string]string
	// UserAttributes independently observes authorization fields; nil skips this check.
	UserAttributes map[string]User
}
type Target interface {
	Snapshot(context.Context, portable.Manifest) (State, error)
	Expected() ([]User, Metadata)
}

func Compare(ctx context.Context, target Target, m portable.Manifest) ([]Mismatch, error) {
	state, err := target.Snapshot(ctx, m)
	if err != nil {
		return nil, err
	}
	users, metadata := target.Expected()
	return CompareState(state, m, users, metadata), nil
}
func CompareState(state State, m portable.Manifest, users []User, metadata Metadata) []Mismatch {
	out := []Mismatch{}
	exemptions := map[string]bool{}
	for _, e := range m.Exemptions {
		exemptions[e.Kind+"\x00"+e.ID] = true
	}
	add := func(kind, id, detail, group string) {
		if !exemptions[group+"\x00"+id] {
			out = append(out, Mismatch{kind, id, detail})
		}
	}
	for _, t := range m.DB.Tables {
		id := t.Schema + "." + t.Name
		_, se := database.SanitizeIdentifier(t.Schema)
		_, ne := database.SanitizeIdentifier(t.Name)
		if se != nil || ne != nil {
			add("unverifiable", id, "unsupported identifier", "table")
			continue
		}
		got, ok := state.Tables[id]
		if !ok || got.Rows != t.Rows {
			add("row", id, "row count differs", "table")
		} else if got.Hash != t.Hash {
			add("content", id, "content hash differs", "table")
		}
	}
	for _, u := range users {
		algorithm, ok := state.Users[u.ID]
		wantAlgorithm := u.HashAlgorithm
		for _, id := range m.Auth.ResetRequired {
			if id == u.ID {
				wantAlgorithm = "none"
			}
		}
		if !ok {
			add("user", u.ID, "user is missing", "user")
		} else if algorithm != wantAlgorithm {
			add("user_hash_algorithm", u.ID, "password hash algorithm differs", "user")
		} else if got, have := state.UserIdent[u.ID]; state.UserIdent != nil && (!have || got != Identity(u.Email, u.Phone, u.DisplayName)) {
			add("user", u.ID, "user identity fields differ", "user")
		} else if got, have := state.UserAttributes[u.ID]; state.UserAttributes != nil && (!have || !sameUserAttributes(u, got)) {
			add("user", u.ID, "user authorization fields differ", "user")
		}
	}
	for _, o := range m.Storage.Objects {
		id := o.Bucket + "/" + o.Key
		sha, ok := state.Objects[id]
		if !ok {
			add("object", id, "object is missing", "object")
		} else if sha != o.SHA256 {
			add("object_sha", id, "object hash differs", "object")
		}
	}
	for id, want := range metadata {
		got, ok := state.Metadata[id]
		if !ok {
			add("table_not_tracked", id, "table is not tracked", "metadata")
			continue
		}
		for _, fields := range []struct {
			kind string
			keys []string
		}{
			{"permission", []string{"insert_permissions", "select_permissions", "update_permissions", "delete_permissions"}},
			{"relationship", []string{"object_relationships", "array_relationships"}},
			{"computed_field", []string{"computed_fields"}},
		} {
			differs := false
			for _, k := range fields.keys {
				if !reflect.DeepEqual(normalizeList(want[k]), normalizeList(got[k])) {
					differs = true
				}
			}
			if differs {
				add(fields.kind, id, "metadata differs", "metadata")
			}
		}
	}
	for _, c := range m.SourceCounts {
		group, id := c.Kind, c.ID
		if group == "users" {
			group = "user"
		}
		if group == "objects" {
			group = "object"
		}
		count, ok := state.Counts[c.Kind+"\x00"+id]
		if c.Kind == "users" {
			for _, u := range users {
				if exemptions["user\x00"+u.ID] {
					if _, exists := state.Users[u.ID]; !exists {
						count++
					}
				}
			}
		}
		if c.Kind == "objects" {
			for _, o := range m.Storage.Objects {
				if !objectInCount(id, m, o) {
					continue
				}
				key := o.Bucket + "/" + o.Key
				if exemptions["object\x00"+key] {
					if _, exists := state.Objects[key]; !exists {
						count++
					}
				}
			}
		}
		if !ok || count != c.Count {
			add("source_count", id, "independent source count differs", group)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].ID < out[j].ID
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}
func objectInCount(id string, m portable.Manifest, o portable.Object) bool {
	for _, item := range m.Storage.Objects {
		if item.Bucket == id {
			return o.Bucket == id
		}
	}
	return true
}
func normalizeList(v any) any {
	if v == nil {
		return []any{}
	}
	if a, ok := v.([]any); ok && len(a) == 0 {
		return []any{}
	}
	return v
}
