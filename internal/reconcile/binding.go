package reconcile

// binding.go — the plan id of a render that generates secrets (P7-LIVE D9,
// P7-SHIP-02 gap 2).
//
// Purpose: a first build generates secrets from a random seed, so its content
// id changes with every render and no plan id could bind it (E451). Such a
// render gets an id that its inputs reproduce: the content id of a second,
// read-only render of the same inputs with a fixed public seed. The id so binds
// the inputs and the whole plan (artifacts, actions, modes, effects, container
// impact); the secret values are the declared secrets-persist step, drawn fresh
// at write time and never part of any plan, as they never were shown in one.
// Inputs: the Request and the computed real render. Outputs: the render's
// PlanID replaced by the binding id and bindable set, or the render unchanged
// when the two renders disagree on anything but the random bytes.
// Constraints: the binding render runs in plan mode and writes nothing, so the
// fixed seed's values never reach the disk; it only feeds a hash. A shape
// mismatch keeps the content id and leaves bindable false, so --plan-id is
// still refused with E451 (fail closed).

import (
	"bytes"
	"context"
	"crypto/sha256"
)

// bindingSeed is the public seed of the binding render. Its values are hashed,
// never written.
var bindingSeed = sha256.Sum256([]byte("nself change-plan binding seed v1"))

// bindGenerated sets c.plan.PlanID to the binding id of a render that
// generates secrets, when the binding render has the same plan shape.
func bindGenerated(ctx context.Context, req Request, c *computed) error {
	b := req
	b.Seed = bindingSeed[:]
	b.DiffOut = nil
	b.Stderr = nil
	b.binding = true
	bc, err := compute(ctx, b)
	if err != nil {
		return err
	}
	same, err := sameShape(*c.plan, *bc.plan)
	if err != nil || !same {
		return err
	}
	c.plan.PlanID = bc.plan.PlanID
	c.bindable = true
	return nil
}

// sameShape reports whether two plans are equal in everything but plan_id.
func sameShape(a, b Plan) (bool, error) {
	ja, err := CanonicalJSON(a)
	if err != nil {
		return false, err
	}
	jb, err := CanonicalJSON(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(ja, jb), nil
}
