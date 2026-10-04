package reconcile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CanonicalJSON returns the canonical encoding of a plan (EPIC D9).
//
// Inputs: a Plan (finalised or not). Outputs: compact json.Marshal bytes of
// a copy whose PlanID is "" and whose nil slices are empty, with HTML escaping
// off ("<", ">", "&" are written literally). Field order is
// the struct order, which is the contract's key order; the type has no maps,
// so the bytes depend only on the values. The input is not modified.
func CanonicalJSON(p Plan) ([]byte, error) {
	c := p
	c.PlanID = ""
	if c.Artifacts == nil {
		c.Artifacts = []Artifact{}
	}
	if c.Effects == nil {
		c.Effects = []Effect{}
	}
	if c.Containers.Items == nil {
		c.Containers.Items = []ContainerItem{}
	}
	if c.DestructiveReasons == nil {
		c.DestructiveReasons = []string{}
	}
	return encode(c, "")
}

// encode marshals p with HTML escaping off, so "<", ">" and "&" stay literal
// and a non-Go consumer can recompute plan_id with any JSON encoder that does
// not escape them. indent "" gives compact output without the trailing newline.
func encode(p Plan, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(p); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	if indent == "" {
		b = bytes.TrimSuffix(b, []byte("\n"))
	}
	return b, nil
}

// PlanID returns the plan_id of a plan: lowercase hex sha256 of its
// CanonicalJSON. Two computations over the same plan are equal, and any change
// to an artifact, effect, container item or header field changes the value.
func PlanID(p Plan) (string, error) {
	b, err := CanonicalJSON(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// MarshalPlan renders a plan as the contract's JSON: 2-space indent, keys in
// contract order, trailing newline. This is the `data` of the v1 envelope and
// the form golden files pin. Nil slices render as [] (see CanonicalJSON).
func MarshalPlan(p Plan) ([]byte, error) {
	if p.Artifacts == nil {
		p.Artifacts = []Artifact{}
	}
	if p.Effects == nil {
		p.Effects = []Effect{}
	}
	if p.Containers.Items == nil {
		p.Containers.Items = []ContainerItem{}
	}
	if p.DestructiveReasons == nil {
		p.DestructiveReasons = []string{}
	}
	return encode(p, "  ")
}
