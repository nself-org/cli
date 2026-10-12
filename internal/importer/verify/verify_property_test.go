package verify

import (
	"fmt"
	"github.com/nself-org/cli/internal/portable"
	"math/rand/v2"
	"testing"
)

func TestVerifyProperties(t *testing.T) {
	for seed := uint64(0); seed < 300; seed++ {
		rng := rand.New(rand.NewPCG(seed, 0xa10))
		id := fmt.Sprintf("public.table_%d", rng.IntN(100000))
		user := fmt.Sprintf("user_%d", rng.Uint64())
		object := fmt.Sprintf("bucket/key_%d", rng.Uint64())
		rows := rng.Int64N(100000)
		hash := fmt.Sprint(rng.Int64N(1000000))
		table := portable.Table{Schema: "public", Name: id[7:], Rows: rows, Hash: hash}
		meta := map[string]any{"table": map[string]any{"schema": "public", "name": table.Name}, "select_permissions": []any{map[string]any{"role": "user", "permission": map[string]any{"columns": []any{"id"}, "filter": map[string]any{}}}}, "object_relationships": []any{map[string]any{"name": "owner", "using": map[string]any{"foreign_key_constraint_on": "owner_id"}}}}
		manifest := portable.Manifest{DB: portable.DB{Tables: []portable.Table{table}}, Storage: portable.Storage{Objects: []portable.Object{{Bucket: "bucket", Key: object[7:], SHA256: hash}}}, SourceCounts: []portable.SourceCount{{Kind: "table", ID: id, Count: rows}}}
		users := []User{{ID: user, HashAlgorithm: "bcrypt"}}
		expected := Metadata{id: meta}
		fresh := func() State {
			return State{Tables: map[string]portable.Table{id: table}, Users: map[string]string{user: "bcrypt"}, Objects: map[string]string{object: hash}, Metadata: Metadata{id: meta}, Counts: map[string]int64{"table\x00" + id: rows}}
		}
		if got := CompareState(fresh(), manifest, users, expected); len(got) != 0 {
			t.Fatalf("seed=%d identical: %+v", seed, got)
		}
		for _, kind := range []string{"row", "content", "user", "user_hash_algorithm", "object", "object_sha", "table_not_tracked", "permission", "relationship", "unverifiable", "source_count"} {
			state := fresh()
			m := manifest
			wantID := id
			group := "table"
			exUsers := users
			exMeta := expected
			switch kind {
			case "row":
				v := table
				v.Rows++
				state.Tables[id] = v
			case "content":
				v := table
				v.Hash = hash + "1"
				state.Tables[id] = v
			case "user":
				delete(state.Users, user)
				wantID = user
				group = "user"
			case "user_hash_algorithm":
				state.Users[user] = "argon2"
				wantID = user
				group = "user"
			case "object":
				delete(state.Objects, object)
				wantID = object
				group = "object"
			case "object_sha":
				state.Objects[object] = "different"
				wantID = object
				group = "object"
			case "table_not_tracked":
				delete(state.Metadata, id)
				group = "metadata"
			case "permission", "relationship":
				changed := map[string]any{}
				for k, v := range meta {
					changed[k] = v
				}
				key := "select_permissions"
				if kind == "relationship" {
					key = "object_relationships"
				}
				changed[key] = []any{}
				state.Metadata[id] = changed
				group = "metadata"
			case "unverifiable":
				v := table
				v.Name = "unsafe;name"
				m.DB = portable.DB{Tables: []portable.Table{v}}
				wantID = v.Schema + "." + v.Name
				exMeta = nil
				exUsers = nil
			case "source_count":
				state.Counts["table\x00"+id]++
			}
			got := CompareState(state, m, exUsers, exMeta)
			if len(got) != 1 || got[0].Kind != kind || got[0].ID != wantID {
				t.Fatalf("seed=%d class=%s: %+v", seed, kind, got)
			}
			m.Exemptions = []portable.Exemption{{Kind: group, ID: wantID, Reason: "seeded exemption"}}
			if got = CompareState(state, m, exUsers, exMeta); len(got) != 0 {
				t.Fatalf("seed=%d exempt %s: %+v", seed, kind, got)
			}
			// A different item must not suppress the planted difference.
			m.Exemptions[0].ID += "_other"
			if got = CompareState(state, m, exUsers, exMeta); len(got) != 1 {
				t.Fatalf("seed=%d broad exemption %s: %+v", seed, kind, got)
			}
		}
	}
}
