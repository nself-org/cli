package invoke

// Purpose: the confirmation gate of contract:cli.machine-request v1 (EPIC D3):
// decide, on the effective side effect, whether a request runs, needs a
// confirmation, or is refused.
// Inputs: the machine registry, an exposed command, a Request, Options (Guard).
// Outputs: a Decision: run (with the invoker-owned flags to add), a
// Confirmation to return, or a refusal (an invoker error or the child's own
// error envelope from the plan exec, passed through unchanged).
// Constraints: consent and plan-id flags are invoker-owned: a request that
// sets one is E425 before anything else. read runs. write without a plan form
// runs, with no consent flags. A plan form without confirm is executed once
// (--<plan.flag>); the child's data.plan_id and data.requires_confirmation
// decide between a kind plan confirmation (confirm = plan_id) and one run with
// --<id_flag>=<plan_id>. remote and destructive without a plan form get a
// kind request confirmation backed by a Guard nonce. A nil Guard refuses
// remote and destructive commands (E421, fail closed).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
)

// Decision kinds; they are also the audit "decision" values.
const (
	DecisionRun     = "run"
	DecisionConfirm = "confirmation_required"
	DecisionRefuse  = "refused"
)

// Confirmation kinds.
const (
	ConfirmKindPlan    = "plan"
	ConfirmKindRequest = "request"
)

// Decision is the outcome of Gate.
type Decision struct {
	// Kind is DecisionRun, DecisionConfirm or DecisionRefuse.
	Kind string
	// SideEffect is the effective side effect (flag escalation applied).
	SideEffect string
	// ConfirmKind is plan or request when a confirmation was asked for or
	// consumed, else empty.
	ConfirmKind string
	// Flags are the invoker-owned argv items to add before the "--" separator
	// (consent flags, --<id_flag>=<id>) when Kind is DecisionRun.
	Flags []string
	// Confirmation is the document to return when Kind is DecisionConfirm.
	Confirmation *Confirmation
	// Refusal is the invoker error when Kind is DecisionRefuse and Result is nil.
	Refusal error
	// Result, when set with DecisionRefuse, is the response to return as is:
	// the child's error envelope from the plan exec, or an E422/E423 for it.
	Result *Result
}

// Gate applies EPIC D3 to req for cmd. The error return is a context error only.
func Gate(ctx context.Context, reg *cmdregistry.Registry, cmd *cmdregistry.Command, req Request, o Options) (Decision, error) {
	d := Decision{Kind: DecisionRun}
	if cmd == nil {
		return refused(d, errs.New("E432", "no such command")), nil
	}
	set := SetFlags(req)
	d.SideEffect = cmdregistry.EffectiveSideEffect(cmd, set)
	if name := ownedFlagSet(cmd, set); name != "" {
		return refused(d, errs.New("E425", "flag "+quoteName(name)+" is set by the invoker, not the request")), nil
	}
	gated := d.SideEffect == canon.SideEffectRemote || d.SideEffect == canon.SideEffectDestructive
	switch {
	case d.SideEffect == canon.SideEffectRead:
		return d, nil
	case !gated && d.SideEffect != canon.SideEffectWrite:
		return refused(d, errs.Newf("E421", "command %s is not exposed", quoteName(barePath(cmd.Path))).
			WithWhy("no gate: unknown side effect "+quoteName(d.SideEffect))), nil
	case gated && cmd.Confirm == nil:
		return refused(d, errs.Newf("E421", "command %s is not exposed", quoteName(barePath(cmd.Path))).
			WithWhy("no gate: the registry row has no confirm block")), nil
	case gated && o.Guard == nil:
		return refused(d, errs.Newf("E421", "command %s is not exposed", quoteName(barePath(cmd.Path))).
			WithWhy("no gate: this server has no confirmation Guard, so remote and destructive commands are refused")), nil
	}
	hasPlan := cmd.Confirm != nil && cmd.Confirm.Plan != nil
	switch {
	case hasPlan:
		return planGate(ctx, cmd, req, o, d, gated)
	case !gated:
		return d, nil
	}
	return requestGate(cmd, req, o, d), nil
}

// refused marks d as an invoker refusal.
func refused(d Decision, err error) Decision {
	d.Kind, d.Refusal = DecisionRefuse, err
	return d
}

// ownedFlagSet returns the first (sorted) consent or plan flag the request sets.
func ownedFlagSet(cmd *cmdregistry.Command, set map[string]bool) string {
	owned := ownedFlags(cmd)
	var hit []string
	for name := range set {
		if owned[name] {
			hit = append(hit, name)
		}
	}
	if len(hit) == 0 {
		return ""
	}
	sort.Strings(hit)
	return hit[0]
}

// requestSecrets lists what a child could echo back that must not leave: the
// registry-secret values (SecretValues) plus the values of flags whose name
// matches config.IsSecretKey, longest first, for scrubbing stderr tails.
func requestSecrets(cmd *cmdregistry.Command, req Request) []string {
	vals := SecretValues(cmd, req)
	for name, v := range req.Flags {
		if !config.IsSecretKey(name) {
			continue
		}
		switch x := v.(type) {
		case []string:
			vals = append(vals, x...)
		case []any:
			for _, e := range x {
				if s, ok := e.(string); ok {
					vals = append(vals, s)
				}
			}
		case string:
			vals = append(vals, x)
		}
	}
	out := vals[:0:0]
	for _, v := range vals {
		if v != "" && v != Redacted {
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// consentFlags renders the command's consent flags as argv items.
func consentFlags(cmd *cmdregistry.Command) []string {
	var out []string
	for _, name := range cmd.Confirm.Flags {
		out = append(out, "--"+name+"=true") // never bare: see flagItems
	}
	return out
}

// requestGate is the nonce round trip for remote and destructive commands
// without a plan form.
func requestGate(cmd *cmdregistry.Command, req Request, o Options, d Decision) Decision {
	command := barePath(cmd.Path)
	digest, ok := requestDigest(command, req)
	if !ok {
		return refused(d, badRequest("the request cannot be encoded"))
	}
	d.ConfirmKind = ConfirmKindRequest
	if req.Confirm == "" {
		nonce, err := o.Guard.issue(digest)
		if err != nil {
			return refused(d, errs.New("E422", "could not issue a confirmation").WithWhy("the system random source failed"))
		}
		d.Kind = DecisionConfirm
		d.Confirmation = &Confirmation{ConfirmationRequired: true, Kind: ConfirmKindRequest, Confirm: nonce,
			SideEffect: d.SideEffect, Reason: reasonFor(d.SideEffect, ConfirmKindRequest)}
		return d
	}
	if !o.Guard.consume(req.Confirm, digest) {
		return refused(d, errs.New("E424", "the confirmation does not match this request"))
	}
	d.Flags = consentFlags(cmd)
	return d
}

// reasonFor is the Confirmation.Reason text.
func reasonFor(side, kind string) string {
	if kind == ConfirmKindPlan {
		return "This " + side + " command is bound to the plan below. Repeat the identical request with confirm set to the plan id to apply it."
	}
	return "This " + side + " command changes state outside this process. Repeat the identical request with confirm set to the confirm value, within 10 minutes, to run it."
}

// planGate handles a command with a plan form.
func planGate(ctx context.Context, cmd *cmdregistry.Command, req Request, o Options, d Decision, gated bool) (Decision, error) {
	plan := cmd.Confirm.Plan
	if req.Confirm != "" {
		if !confirmPattern.MatchString(req.Confirm) {
			return refused(d, errs.New("E424", "the confirmation does not match this request")), nil
		}
		d.ConfirmKind = ConfirmKindPlan
		d.Flags = append(consentFlags(cmd), "--"+plan.IDFlag+"="+req.Confirm)
		return d, nil
	}
	transport := transportOf(o)
	argv, err := BuildArgvFor(cmd, req, transport)
	if err != nil {
		return refused(d, err), nil
	}
	argv = withFlags(argv, "--"+plan.Flag+"=true")
	ran, out, err := runChild(ctx, Result{}, barePath(cmd.Path), specFor(o, transport, argv), requestSecrets(cmd, req), false)
	if err != nil {
		return d, err
	}
	if out != nil {
		d.Kind, d.Result = DecisionRefuse, out
		return d, nil
	}
	facts, childErr, problem := parsePlan(ran.Stdout)
	switch {
	case childErr:
		d.Kind, d.Result = DecisionRefuse, &Result{Stdout: ran.Stdout, ExitCode: ran.ExitCode}
	case problem != "":
		d = refused(d, errs.Newf("E422", "%s produced no usable plan", quoteName(barePath(cmd.Path))).WithWhy(problem))
	case facts.requires || gated:
		d.Kind, d.ConfirmKind = DecisionConfirm, ConfirmKindPlan
		d.Confirmation = &Confirmation{ConfirmationRequired: true, Kind: ConfirmKindPlan, Confirm: facts.id,
			SideEffect: d.SideEffect, Reason: reasonFor(d.SideEffect, ConfirmKindPlan), Plan: facts.data}
	default:
		d.Flags = []string{"--" + plan.IDFlag + "=" + facts.id}
	}
	return d, nil
}

// planFacts is what the gate reads from the child's plan document.
type planFacts struct {
	id       string
	requires bool
	data     json.RawMessage
}

// parsePlan reads the plan exec's envelope. childErr is true when it is the
// child's own error envelope; problem is a short cause when plan_id or
// requires_confirmation is missing or invalid.
func parsePlan(stdout []byte) (f planFacts, childErr bool, problem string) {
	var doc struct {
		Data  json.RawMessage `json:"data"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &doc); err != nil {
		return f, false, "the plan output is not a JSON envelope"
	}
	if len(doc.Error) > 0 && string(doc.Error) != "null" {
		return f, true, ""
	}
	if d := bytes.TrimSpace(doc.Data); len(d) == 0 || d[0] != '{' {
		return f, false, "the plan output has no data object"
	}
	var data struct {
		PlanID   *string `json:"plan_id"`
		Requires *bool   `json:"requires_confirmation"`
	}
	if err := json.Unmarshal(doc.Data, &data); err != nil {
		return f, false, "the plan output has no data object"
	}
	switch {
	case data.PlanID == nil:
		return f, false, "the plan output has no data.plan_id"
	case !confirmPattern.MatchString(*data.PlanID):
		return f, false, "data.plan_id is not 64 lowercase hex characters"
	case data.Requires == nil:
		return f, false, "the plan output has no data.requires_confirmation boolean"
	}
	return planFacts{id: *data.PlanID, requires: *data.Requires, data: doc.Data}, false, ""
}

// confirmationResult writes c as a success envelope for command into res.
func confirmationResult(res Result, command string, c *Confirmation) Result {
	var buf bytes.Buffer
	if err := output.EmitData(output.Writer{Out: &buf, Err: io.Discard}, command, c); err != nil {
		return refuse(res, command, errs.New("E422", "could not write the confirmation").WithWhy(errors.Unwrap(err).Error()))
	}
	res.Stdout, res.ExitCode, res.ErrorCode = buf.Bytes(), 0, ""
	return res
}
