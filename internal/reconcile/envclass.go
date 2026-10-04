package reconcile

import "strings"

// EnvClassName is the `env_class` enum of the change-plan contract.
type EnvClassName string

// EnvClassName values.
const (
	ClassProd EnvClassName = "prod"
	ClassDev  EnvClassName = "dev"
)

// NormalizeEnv returns the normalised ENV name used in Plan.Env.
//
// Inputs: a raw ENV value. Outputs: trimmed, lower-cased, with the aliases
// the config package folds (development/develop/devel to dev, production to
// prod, stage to staging). Unknown names pass through lower-cased; empty stays
// empty. Mirrors internal/config normalizeEnv without importing it (this
// package stays free of config and filesystem edges).
func NormalizeEnv(env string) string {
	switch e := strings.ToLower(strings.TrimSpace(env)); e {
	case "development", "develop", "devel":
		return "dev"
	case "production":
		return "prod"
	case "stage":
		return "staging"
	default:
		return e
	}
}

// EnvClass classes an environment name per EPIC D3.
//
// Inputs: a raw or normalised ENV (also the --env of a remote db command).
// Outputs: ClassProd when the normalised name is prod or staging, else
// ClassDev. Empty and unknown names are dev: a running stack does not make
// an env prod-class, so local changes never teach --yes by reflex.
func EnvClass(env string) EnvClassName {
	switch NormalizeEnv(env) {
	case "prod", "staging":
		return ClassProd
	default:
		return ClassDev
	}
}
