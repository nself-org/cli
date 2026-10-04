// Package reconcile is the pure half of the change-plan engine (P7-LIVE-01).
//
// Purpose: one in-memory model of "what a command is about to change" that
// every mutating command (build, config set, doctor --fix, plugin and bundle
// install/remove, db hasura apply-ref) renders the same way. The package
// holds the Plan type (contract:cli.change-plan v1), the artifact diff, the
// destructive/confirmation classification, the env class, the canonical JSON
// and plan_id, and a deterministic human renderer.
//
// Constraints: stdlib plus internal/observability only. No filesystem,
// docker, generator or command wiring lives here; P7-LIVE-03 adds the build
// edge. Nothing in a Plan's JSON depends on time or map iteration order.
package reconcile

import (
	"fmt"
	"sort"
)

// SchemaVersion is the change-plan contract version carried in every Plan.
const SchemaVersion = "1"

// Command is the `command` enum of the change-plan contract.
type Command string

// Command values, exactly as written in the contract.
const (
	CmdBuild            Command = "build"
	CmdConfigSet        Command = "config-set"
	CmdDoctorFix        Command = "doctor-fix"
	CmdPluginInstall    Command = "plugin-install"
	CmdPluginRemove     Command = "plugin-remove"
	CmdBundleInstall    Command = "bundle-install"
	CmdBundleRemove     Command = "bundle-remove"
	CmdDBHasuraApplyRef Command = "db-hasura-apply-ref"
)

// TriggerKind is the `trigger.kind` enum.
type TriggerKind string

// TriggerKind values.
const (
	TriggerBuild    TriggerKind = "build"
	TriggerConfig   TriggerKind = "config"
	TriggerPlugin   TriggerKind = "plugin"
	TriggerBundle   TriggerKind = "bundle"
	TriggerDrift    TriggerKind = "drift"
	TriggerMetadata TriggerKind = "metadata"
)

// ArtifactKind is the `artifacts[].kind` enum.
type ArtifactKind string

// ArtifactKind values.
const (
	KindCompose      ArtifactKind = "compose"
	KindNginx        ArtifactKind = "nginx"
	KindSSL          ArtifactKind = "ssl"
	KindEnv          ArtifactKind = "env"
	KindHasura       ArtifactKind = "hasura"
	KindMonitoring   ArtifactKind = "monitoring"
	KindPostgresInit ArtifactKind = "postgres-init"
	KindState        ArtifactKind = "state"
	KindLock         ArtifactKind = "lock"
	KindOther        ArtifactKind = "other"
)

// Action is the `artifacts[].action` enum.
type Action string

// Action values.
const (
	ActionAdd    Action = "add"
	ActionChange Action = "change"
	ActionRemove Action = "remove"
)

// EffectKind is the `effects[].kind` enum.
type EffectKind string

// EffectKind values.
const (
	EffectHosts          EffectKind = "hosts"
	EffectTrustStore     EffectKind = "trust-store"
	EffectCertificates   EffectKind = "certificates"
	EffectPluginFragment EffectKind = "plugin-fragment"
	EffectPluginInstall  EffectKind = "plugin-install"
	EffectPluginRemove   EffectKind = "plugin-remove"
	EffectSecretsPersist EffectKind = "secrets-persist"
	EffectNginxSitesBkp  EffectKind = "nginx-sites-backup"
	EffectOrphanRemove   EffectKind = "orphan-remove"
	EffectBuildLock      EffectKind = "build-lock"
)

// ContainerAction is the `containers.items[].action` enum. ContainerNone
// items are dropped by Finalize ("none omitted").
type ContainerAction string

// ContainerAction values.
const (
	ContainerRecreate ContainerAction = "recreate"
	ContainerNone     ContainerAction = "none"
	ContainerUnknown  ContainerAction = "unknown"
)

// AppliedBy is the `containers.items[].applied_by` enum.
type AppliedBy string

// AppliedBy values.
const (
	AppliedNextStart   AppliedBy = "next-start"
	AppliedThisCommand AppliedBy = "this-command"
)

// Trigger records what started the plan; Subject is a key, plugin or bundle
// name, git ref, or "".
type Trigger struct {
	Kind    TriggerKind `json:"kind"`
	Subject string      `json:"subject"`
}

// Artifact is one changed file. Unchanged files never appear. Path is
// project-relative with "/" separators, or "@fronting/<rel>" for the fronting
// stack. DiffLines is inserted plus deleted lines, or -1 when a side exceeds
// MaxDiffLines.
type Artifact struct {
	Kind       ArtifactKind `json:"kind"`
	Path       string       `json:"path"`
	Action     Action       `json:"action"`
	Generated  bool         `json:"generated"`
	HandEdited bool         `json:"hand_edited"`
	DiffLines  int          `json:"diff_lines"`
	Redacted   bool         `json:"redacted"`
}

// Effect is a host-level side effect. Detail never carries secret values.
type Effect struct {
	Kind   EffectKind `json:"kind"`
	Target string     `json:"target"`
	Detail string     `json:"detail"`
}

// ContainerItem is the impact on one compose service.
type ContainerItem struct {
	Service   string          `json:"service"`
	Action    ContainerAction `json:"action"`
	Stateful  bool            `json:"stateful"`
	AppliedBy AppliedBy       `json:"applied_by"`
}

// Containers is the running-container impact. Known is false when the
// running state could not be read; Items is never nil after Finalize.
type Containers struct {
	Known bool            `json:"known"`
	Items []ContainerItem `json:"items"`
}

// Plan implements contract:cli.change-plan v1. The struct field order is the
// contract's key order and is also the canonical-JSON order; there are no
// maps in the type, so encoding is deterministic.
type Plan struct {
	SchemaVersion        string       `json:"schema_version"`
	PlanID               string       `json:"plan_id"`
	Command              Command      `json:"command"`
	Trigger              Trigger      `json:"trigger"`
	Env                  string       `json:"env"`
	EnvClass             EnvClassName `json:"env_class"`
	Artifacts            []Artifact   `json:"artifacts"`
	Effects              []Effect     `json:"effects"`
	Containers           Containers   `json:"containers"`
	Destructive          bool         `json:"destructive"`
	DestructiveReasons   []string     `json:"destructive_reasons"`
	RequiresConfirmation bool         `json:"requires_confirmation"`
	Empty                bool         `json:"empty"`
}

// Finalize normalises the plan and computes every derived field.
//
// Inputs: a Plan whose Command, Trigger, Env, Artifacts, Effects and
// Containers are filled in by the caller. Outputs: the same Plan with
// SchemaVersion set, Env normalised, EnvClass derived, arrays non-nil and
// sorted (artifacts by path; effects by kind, target; containers by service;
// reasons lexically), container items with action "none" dropped, Empty,
// Destructive, DestructiveReasons and RequiresConfirmation computed, and
// PlanID set last. Finalize is idempotent. The error is a JSON encoding
// failure, which cannot occur for well-formed plans.
func (p *Plan) Finalize() error {
	p.SchemaVersion = SchemaVersion
	p.Env = NormalizeEnv(p.Env)
	p.EnvClass = EnvClass(p.Env)
	p.normalizeArrays()
	p.sortArrays()
	p.Empty = len(p.Artifacts) == 0 && len(p.Effects) == 0 && len(p.Containers.Items) == 0
	p.DestructiveReasons = destructiveReasons(p)
	p.Destructive = len(p.DestructiveReasons) > 0
	p.RequiresConfirmation = p.EnvClass == ClassProd && !p.Empty
	id, err := PlanID(*p)
	if err != nil {
		return err
	}
	p.PlanID = id
	return nil
}

// normalizeArrays replaces nil slices with empty ones (JSON [] not null) and
// drops container items whose action is "none".
func (p *Plan) normalizeArrays() {
	if p.Artifacts == nil {
		p.Artifacts = []Artifact{}
	}
	if p.Effects == nil {
		p.Effects = []Effect{}
	}
	kept := make([]ContainerItem, 0, len(p.Containers.Items))
	for _, it := range p.Containers.Items {
		if it.Action != ContainerNone {
			kept = append(kept, it)
		}
	}
	p.Containers.Items = kept
	if p.DestructiveReasons == nil {
		p.DestructiveReasons = []string{}
	}
}

// sortArrays orders every array into contract order. Ties on the primary key
// fall back to the remaining fields so the order never depends on input order.
func (p *Plan) sortArrays() {
	sort.SliceStable(p.Artifacts, func(i, j int) bool {
		a, b := p.Artifacts[i], p.Artifacts[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Action != b.Action {
			return a.Action < b.Action
		}
		return fmt.Sprint(a) < fmt.Sprint(b)
	})
	sort.SliceStable(p.Effects, func(i, j int) bool {
		a, b := p.Effects[i], p.Effects[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.Detail < b.Detail
	})
	sort.SliceStable(p.Containers.Items, func(i, j int) bool {
		a, b := p.Containers.Items[i], p.Containers.Items[j]
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		if a.Action != b.Action {
			return a.Action < b.Action
		}
		return fmt.Sprint(a) < fmt.Sprint(b)
	})
}
