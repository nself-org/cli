// Package sdk provides shared infrastructure for nSelf plugin services
// and CLI consumers written in Go.
//
// # Overview
//
// The nSelf Go SDK is a self-contained module under
// github.com/nself-org/cli/sdk/go/v2. It is consumed by every Go-based plugin
// service in the nSelf ecosystem (ai, claw, mux, voice, browser, notify,
// cron, chat, livekit, recording, bots) and by external integrators who
// build against the public nSelf surface.
//
// The SDK is intentionally small. It captures only the cross-plugin
// glue that would otherwise be reimplemented per service: HTTP server
// bootstrap, structured logging, env-driven config, postgres pool
// helpers, license validation, Prometheus wiring, and tracing context
// propagation. Plugin business logic lives in each plugin's own
// repository, not here.
//
// # Sub-packages
//
//	plugin    Base plugin types, lifecycle hooks, registration.
//	logger    slog wrappers with consistent JSON output for ingestion by Loki.
//	config    Env + file config loader with type-safe accessors.
//	db        pgx pool helpers, health checks, migration runner.
//	httpx     HTTP client with retries, timeouts, OpenTelemetry hooks.
//	metrics   Prometheus /metrics endpoint with per-plugin counters.
//	server    chi router setup with middleware defaults (recover, request id, log).
//	license   License grace period + offline validation helpers.
//	identity  Caller identity extraction from JWT/Hasura headers.
//	tracing   OTel context wiring (extract, inject, span helpers).
//	costmeter Per-request cost accounting hooks.
//	devkit    Local-development helpers (mock servers, fixtures).
//	testing   Test fixtures and assertions for plugin authors.
//	new-plugin
//	          Scaffolding helpers used by `nself plugin new`.
//
// # Versioning
//
// The SDK ships in lockstep with the nSelf CLI release tag. The cli
// repository's release tag corresponds to the same version of this
// module (currently v1.4.4). Consumers should pin to a CLI minor
// version and accept patch upgrades.
//
//	require github.com/nself-org/cli/sdk/go v1.4.4
//
// # Compatibility
//
// See COMPATIBILITY.md for the support matrix. The SDK guarantees API
// stability within a minor version (semver). Breaking changes ship in
// minor or major bumps and are documented in CHANGELOG.md.
//
// # Quick start
//
// A minimal plugin entrypoint:
//
//	package main
//
//	import (
//	    "context"
//
//	    "github.com/nself-org/cli/sdk/go/plugin"
//	    "github.com/nself-org/cli/sdk/go/server"
//	)
//
//	func main() {
//	    p := plugin.New("my-plugin", plugin.Version("1.0.0"))
//
//	    p.OnInstall(func(ctx context.Context, c *plugin.Context) error {
//	        c.Env.Require("EXAMPLE_API_KEY")
//	        return nil
//	    })
//
//	    p.OnStart(func(ctx context.Context, c *plugin.Context) error {
//	        return server.ListenAndServe(ctx, c, p.Routes())
//	    })
//
//	    p.Run()
//	}
//
// # Isolation boundary
//
// This module MUST NOT be imported by cli/cmd or cli/internal. The CI gate
// sdk-cross-import-check.yml enforces this boundary. The SDK is a public
// downstream surface. the CLI is its upstream packager.
//
// # Repository
//
// Source: https://github.com/nself-org/cli/tree/main/sdk/go
// Issues: https://github.com/nself-org/cli/issues
// Docs:   https://nself.org/docs/sdk/go
//
// # License
//
// MIT.
package sdk

// Version is the SDK release. Consumers can gate on this via compatibility.go.
//
// This value is updated by the sdk-version-sync workflow on every CLI tag
// to keep SDK and CLI versions in lockstep.
const Version = "1.4.4"
