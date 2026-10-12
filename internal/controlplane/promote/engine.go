package promote

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/errs"
)

// Step is one promotion action and its compensating action.
type Step struct {
	Name string
	Run  func(context.Context, *State) error
	Undo func(context.Context, *State) error
}

// Observer receives synchronous promotion lifecycle events.
type Observer interface {
	Start(step string)
	Done(step string, d time.Duration)
	Failed(step string, err error)
	Skipped(step, reason string)
	Rollback(step string, err error)
}

// Engine executes the canonical promotion transaction.
//
// Purpose: enforce ordering, observability, and rollback invariants.
// Inputs: canonical Steps, optional Observer/Verify/clock, and mutable State.
// Outputs: a Result plus the triggering structured error on failure.
// Constraints: execution is pure orchestration; all I/O lives in injected hooks.
type Engine struct {
	Steps    []Step
	Observer Observer
	Verify   func(context.Context, *State) error
	Now      func() time.Time
}

// StepNames is the only accepted promotion execution order.
var StepNames = []string{
	"resolve", "confirm", "verify", "preflight", "source-health", "capture",
	"build", "compare", "snapshot", "save", "apply", "migrate", "health", "record",
}

var stepDescriptions = map[string]string{
	"resolve": "resolve source and target environments", "confirm": "confirm production promotion",
	"verify": "verify the release manifest", "preflight": "check target architecture and access",
	"source-health": "check source health", "capture": "capture source image identities",
	"build": "build target artifacts in scratch space", "compare": "compare per-platform image identities",
	"snapshot": "create and verify the target backup", "save": "save the target artifact set",
	"apply": "apply artifacts to the target", "migrate": "run target database migrations",
	"health": "check target health", "record": "record the completed promotion",
}

// Plan returns the deterministic canonical plan without executing a step.
func (e Engine) Plan() []StepResult {
	out := make([]StepResult, len(StepNames))
	for i, name := range StepNames {
		out[i] = StepResult{Name: name, Status: "planned", Detail: stepDescriptions[name]}
	}
	return out
}

// ValidateTiers rejects downward promotions and identical environments.
func ValidateTiers(source, target string, sourceTier, targetTier controlplane.Tier) error {
	if source == target {
		return errs.New("E401", "promotion source and target must differ")
	}
	if sourceTier != controlplane.TierLocal && sourceTier != controlplane.TierLocalServers && sourceTier != controlplane.TierProd {
		return errs.Newf("E401", "unknown promotion source tier %q", sourceTier)
	}
	if targetTier != controlplane.TierLocal && targetTier != controlplane.TierLocalServers && targetTier != controlplane.TierProd {
		return errs.Newf("E401", "unknown promotion target tier %q", targetTier)
	}
	if controlplane.TierRank(targetTier) < controlplane.TierRank(sourceTier) {
		return errs.Newf("E401", "cannot promote from tier %s to lower tier %s", sourceTier, targetTier)
	}
	return nil
}

// Run executes the canonical steps and compensates failures after snapshot.
func (e Engine) Run(ctx context.Context, st *State) (Result, error) {
	if st == nil {
		return Result{Status: "failed"}, fmt.Errorf("promote: state is nil")
	}
	result := resultFrom(st)
	if err := e.validate(); err != nil {
		result.Status = "failed"
		return result, err
	}
	if st.DryRun {
		result.Status, result.DryRun, result.Steps = "ok", true, e.Plan()
		return result, nil
	}
	now := e.Now
	if now == nil {
		now = time.Now
	}
	snapshotDone := false
	for i, step := range e.Steps {
		if step.Name == "verify" && e.Verify == nil {
			e.skipped(step.Name, "no release manifest")
			result.Steps = append(result.Steps, StepResult{Name: step.Name, Status: "skipped", Detail: "no release manifest"})
			continue
		}
		if step.Name == "migrate" {
			st.MigrateStarted = true
		}
		e.start(step.Name)
		started := now()
		err := e.runStep(ctx, step, st)
		if err == nil {
			e.done(step.Name, now().Sub(started))
			result.Steps = append(result.Steps, StepResult{Name: step.Name, Status: "ok"})
			snapshotDone = snapshotDone || step.Name == "snapshot"
			continue
		}
		e.failed(step.Name, err)
		result.Steps = append(result.Steps, StepResult{Name: step.Name, Status: "failed", Detail: err.Error()})
		result.Status, result.FailedStep = "failed", step.Name
		if step.Name == "record" {
			return finishResult(result, st), liveRecordFailure(st, err)
		}
		if !snapshotDone {
			return finishResult(result, st), err
		}
		return e.rollback(ctx, st, result, i, err)
	}
	result.Status = "ok"
	return finishResult(result, st), nil
}

// liveRecordFailure reports recovery evidence without undoing a healthy promotion.
// Purpose: keep the final record write outside the compensating transaction.
// Inputs: snapshot, backup, and host artifact evidence plus the write error.
// Outputs: an E489 error naming every pre-deploy tar for manual recovery.
// Constraints: the provisional record stays available for operator rollback.
func liveRecordFailure(st *State, cause error) error {
	hosts := make([]string, 0, len(st.PreDeployTar))
	for host := range st.PreDeployTar {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	tars := make([]string, 0, len(hosts))
	for _, host := range hosts {
		tars = append(tars, fmt.Sprintf("%s=%s", host, st.PreDeployTar[host]))
	}
	message := fmt.Sprintf("promotion is live but final record failed; snapshot %s, backup %s, pre-deploy tars [%s]; roll back by hand if needed", st.SnapshotTag, st.BackupID, strings.Join(tars, ", "))
	return errs.Wrap("E489", message, cause)
}

func (e Engine) validate() error {
	if len(e.Steps) != len(StepNames) {
		return fmt.Errorf("promote: got %d steps, want %d", len(e.Steps), len(StepNames))
	}
	for i := range StepNames {
		if e.Steps[i].Name != StepNames[i] {
			return fmt.Errorf("promote: step %d is %q, want %q", i, e.Steps[i].Name, StepNames[i])
		}
	}
	return nil
}

func (e Engine) runStep(ctx context.Context, step Step, st *State) error {
	if step.Name == "verify" && e.Verify != nil {
		return e.Verify(ctx, st)
	}
	if step.Run == nil {
		return nil
	}
	return step.Run(ctx, st)
}
