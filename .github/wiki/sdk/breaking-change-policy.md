# Plugin SDK Breaking-Change Policy

Applies to all three Plugin SDK languages: Go, TypeScript, and Python.

---

## Semantic Versioning

All SDKs follow [Semantic Versioning](https://semver.org/):

- **MAJOR** — breaking changes. Existing code may stop compiling or behave differently.
- **MINOR** — additive changes. New APIs, new options, backward-compatible.
- **PATCH** — bug fixes. No API changes.

---

## What Counts as a Breaking Change

A breaking change is anything that can break existing plugin code without any changes on the plugin author's side:

- Removing or renaming an exported symbol (function, type, class, method, constant)
- Changing a function or method signature (parameter types, order, return type)
- Changing data types in serialized formats (JSON field names, protobuf field numbers, wire format)
- Removing a previously documented behavior that plugins depend on
- Changing required vs. optional status of a parameter

Non-breaking changes (safe to ship as MINOR or PATCH):

- Adding new optional parameters with defaults
- Adding new exported symbols
- Adding new optional fields to serialized structs
- Fixing documented bugs

---

## Deprecation Cycle

Breaking changes follow a minimum two-step process:

1. **Deprecation notice** — the symbol or behavior is marked deprecated in the current MINOR release. Documentation and language-native annotations (`@deprecated`, `//nolint`, docstring warnings) are added. The old API still works.
2. **Removal** — the deprecated symbol is removed in the next MAJOR release, no sooner than one full MINOR version after deprecation.

Skipping the deprecation step is only allowed for critical security fixes. Any such skip must be called out explicitly in the release notes.

---

## Migration Guides

Every MAJOR version bump requires a `MIGRATION.md` file committed to the SDK repo before the release tag is created. The file must cover:

- Which symbols were removed or renamed, and what to use instead
- Which data types changed, with before/after examples
- A step-by-step upgrade checklist for plugin authors
- Any tooling or scripts that automate parts of the migration

---

## Cross-SDK Version Alignment

All three SDKs (Go, TypeScript, Python) maintain synchronized MAJOR versions. If one SDK ships a breaking change, all three SDKs bump to the same MAJOR version in the same release cycle.

MINOR and PATCH versions may diverge between SDKs based on individual language needs.

---

## Pre-Release Testing

Before any MAJOR release, RC (release candidate) tags are required:

1. Tag at least one RC: `sdk/go/v2.0.0-rc.1`, `sdk/ts/v2.0.0-rc.1`, etc.
2. Allow a minimum of two weeks between first RC and final release.
3. Publish RC docs and notify plugin authors via the changelog and release notes.
4. Only promote to final after at least one plugin author has tested and confirmed compatibility.

---

## Changelog Requirements

Every SDK release, regardless of version bump type, requires a CHANGELOG entry in the SDK's `CHANGELOG.md`. Each entry must include:

- Version number and release date
- Summary of changes grouped by type: Breaking, Added, Changed, Fixed, Deprecated, Removed
- Links to relevant PRs or issues

CHANGELOG entries are mandatory. A release without one is incomplete.

---

## Language-Specific Notes

### Go (`sdk/go/`)

- Exported identifiers follow Go naming conventions. Any rename is a breaking change.
- Module path changes (e.g., `nself-org/cli/sdk/go/v2`) are required for MAJOR bumps.

### TypeScript (`sdk/ts/`)

- Type exports are part of the public API. Narrowing a type is a breaking change.
- ESM/CJS format changes count as breaking.

### Python (`sdk/py/`)

- Public functions, classes, and module-level constants are part of the API.
- Dropping a supported Python version counts as a breaking change.

---

## Summary Table

| Rule | Requirement |
|---|---|
| Deprecation period | Min 1 MINOR version before removal |
| Migration guide | Required for every MAJOR bump |
| Cross-SDK MAJOR sync | All 3 SDKs bump together |
| RC tags | Required before any MAJOR release |
| RC window | Min 2 weeks before final |
| Changelog | Required for every release |
