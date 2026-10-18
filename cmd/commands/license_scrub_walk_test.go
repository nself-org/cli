package commands

// Purpose: the scrubber's reflect walk (D-0981) reaches every place a secret
// can sit in a document value: map keys, byte slices, json.RawMessage,
// promoted fields of an unexported embed; it ends on a cycle; and no
// registered account document type holds a shape the walk cannot scrub.
// Inputs: hand-built document values; accountEnvelopeTypes.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

type scrubEmbed struct {
	Note string `json:"note"`
}

type scrubWalkDoc struct {
	scrubEmbed
	ByKey  map[string]string `json:"by_key"`
	Raw    json.RawMessage   `json:"raw"`
	Blob   []byte            `json:"blob"`
	Public string            `json:"public"`
}

type scrubNode struct {
	Label string
	Next  *scrubNode
	Kids  []any
}

// TestScrubWalkReachesEverySecretPlace: the hostile key sits in a map key, a
// raw JSON field, a byte slice and a promoted field of an unexported embed.
func TestScrubWalkReachesEverySecretPlace(t *testing.T) {
	in := scrubWalkDoc{
		scrubEmbed: scrubEmbed{Note: hostileOtherKey},
		ByKey:      map[string]string{hostileOtherKey: "ok"},
		Raw:        json.RawMessage(`{"k":"` + hostileOtherKey + `"}`),
		Blob:       []byte(hostileOtherKey),
		Public:     "plain",
	}
	got := scrubDoc(in).(scrubWalkDoc)
	if got.Public != "plain" || got.ByKey == nil || !json.Valid(got.Raw) {
		t.Fatalf("scrub broke the shape: %+v", got)
	}
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	noSecrets(t, "scrubbed walk doc", string(wire), hostileOtherKey)
	if in.Note == "" || !strings.Contains(string(in.Blob), hostileOtherKey) {
		t.Error("scrub changed its input")
	}
}

// TestScrubWalkEndsOnCycle: a pointer cycle and a self-holding interface
// slice end, with the secret still scrubbed.
func TestScrubWalkEndsOnCycle(t *testing.T) {
	a := &scrubNode{Label: hostileOtherKey}
	a.Next = a
	a.Kids = []any{a, a}
	done := make(chan *scrubNode, 1)
	go func() { done <- scrubDoc(a).(*scrubNode) }()
	select {
	case got := <-done:
		noSecrets(t, "cyclic doc", fmt.Sprint(got.Label), hostileOtherKey)
	case <-time.After(10 * time.Second):
		t.Fatal("scrub did not end on a cyclic value")
	}
}

// scrubbableProblem names why the walk cannot scrub a type, or "".
func scrubbableProblem(t reflect.Type, seen map[reflect.Type]bool) string {
	if seen[t] {
		return ""
	}
	seen[t] = true
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return t.String() + " cannot be copied or scrubbed"
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return scrubbableProblem(t.Elem(), seen)
	case reflect.Map:
		if p := scrubbableProblem(t.Key(), seen); p != "" {
			return p
		}
		return scrubbableProblem(t.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() && !(f.Anonymous && f.Type.Kind() == reflect.Struct) {
				continue // unexported data never reaches the wire
			}
			if !f.IsExported() && f.Anonymous && f.Type.Kind() == reflect.Pointer {
				return t.String() + " embeds an unexported pointer: its promoted fields are out of reach"
			}
			if p := scrubbableProblem(f.Type, seen); p != "" {
				return p
			}
		}
	}
	return ""
}

// TestAccountDocTypesAreScrubbable: every registered account document type
// is a shape the scrubber walk can copy.
func TestAccountDocTypesAreScrubbable(t *testing.T) {
	for path, zero := range accountEnvelopeTypes {
		if p := scrubbableProblem(reflect.TypeOf(zero), map[reflect.Type]bool{}); p != "" {
			t.Errorf("%s: %s", path, p)
		}
	}
	type unreachable struct {
		*scrubEmbed
		C chan int
	}
	if scrubbableProblem(reflect.TypeOf(unreachable{}), map[reflect.Type]bool{}) == "" {
		t.Error("the check accepts a type holding a channel")
	}
}
