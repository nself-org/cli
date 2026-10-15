package verify

// Purpose: read expected identities and metadata from an integrity-checked bundle.
// Inputs: portable Reader. Outputs: user records and table metadata.
// Constraints: streaming reads recheck hashes; decoding errors never quote values.
import (
	"encoding/json"
	"errors"
	"github.com/nself-org/cli/internal/portable"
	"io"
)

func HasMember(r *portable.Reader, name string) bool {
	for _, f := range r.Manifest().Files {
		if f.Path == name {
			return true
		}
	}
	return false
}
func ReadUsers(r *portable.Reader) ([]User, error) {
	if !HasMember(r, "auth/users.jsonl") {
		if r.Manifest().Auth.Users > 0 {
			return nil, errors.New("bundle user member missing")
		}
		return nil, nil
	}
	f, err := r.Open("auth/users.jsonl")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(f)
	dec.UseNumber()
	out := []User{}
	seen := map[string]bool{}
	for {
		var u User
		err = dec.Decode(&u)
		if err == io.EOF {
			break
		}
		if err != nil || u.ID == "" || seen[u.ID] {
			return nil, errors.New("invalid bundle user record")
		}
		seen[u.ID] = true
		out = append(out, u)
	}
	if len(out) != r.Manifest().Auth.Users {
		return nil, errors.New("bundle user count differs")
	}
	return out, nil
}
func ReadMetadata(r *portable.Reader) (Metadata, error) {
	out := Metadata{}
	// Plain Postgres has no Hasura document; every imported table still needs tracking.
	for _, t := range r.Manifest().DB.Tables {
		out[t.Schema+"."+t.Name] = map[string]any{"table": map[string]any{"schema": t.Schema, "name": t.Name}}
	}
	if !HasMember(r, "hasura/metadata.json") {
		return out, nil
	}
	f, err := r.Open("hasura/metadata.json")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	tables, err := ParseMetadata(b)
	if err != nil {
		return nil, err
	}
	for id, t := range tables {
		if _, ok := out[id]; ok {
			out[id] = t
		}
	}
	return out, nil
}
func ParseMetadata(b []byte) (Metadata, error) {
	var doc struct {
		Sources []struct {
			Name   string           `json:"name"`
			Tables []map[string]any `json:"tables"`
		} `json:"sources"`
	}
	if json.Unmarshal(b, &doc) != nil {
		return nil, errors.New("invalid metadata document")
	}
	out := Metadata{}
	for _, src := range doc.Sources {
		if src.Name != "default" {
			continue
		}
		for _, t := range src.Tables {
			ref, ok := t["table"].(map[string]any)
			if !ok {
				return nil, errors.New("invalid metadata table")
			}
			schema, sok := ref["schema"].(string)
			name, nok := ref["name"].(string)
			if !sok || !nok {
				return nil, errors.New("invalid metadata table")
			}
			out[schema+"."+name] = t
		}
	}
	return out, nil
}
