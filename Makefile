BINARY := nself
MODULE := github.com/nself-org/cli
DIST_DIR := dist
VERSION := $(shell cat .github/VERSION 2>/dev/null || git describe --tags 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
NSELF_LICENSE_PUBKEY_HEX ?=
LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.BuildDate=$(BUILD_DATE) \
	-X $(MODULE)/internal/license.licensePubKeyHex=$(NSELF_LICENSE_PUBKEY_HEX)
BUILDFLAGS := -trimpath

.PHONY: build clean test vet install cross dist verify-prod sport-f21 sport-f02 cmd-inventory registry core-services wiki-commands wiki-check flag-drift-audit parity schemas schemas-check contract-check sbom man fmt fmt-check

verify-prod:
	@bash scripts/prod-verify/p87-verification.sh

sport-f21:
	@bash scripts/sport/generate-f21.sh

## cmd-inventory — CLI-R06. Regenerate the command inventory from the cobra tree.
## Writes .github/command-inventory.json, .github/command-registry.json (the
## command registry, contract cli.command-registry v1) and the generated block
## of .github/wiki/Commands.md.
## Set NSELF_SPORT_DIR to also refresh SPORT F02 (it lives in the PPI, not here).
## internal/repoqa asserts the committed copies match, so run this after adding,
## renaming, or removing any command.
cmd-inventory:
	@bash scripts/sport/generate-f02.sh

sport-f02: cmd-inventory

## registry — P7-REG-07. Alias of cmd-inventory: regenerates the command registry
## .github/command-registry.json together with the inventory and wiki block.
registry: cmd-inventory

## core-services — CLI-R07. Regenerate .github/wiki/Core-Services.md from the
## compose service catalog. Set NSELF_SPORT_DIR to also refresh SPORT F08.
core-services:
	@bash scripts/sport/generate-core-services.sh

## wiki-commands — CLI-R08. Regenerate one wiki page per top-level command, plus
## the sidebar index and llms.txt. Human prose inside PROSE blocks is preserved;
## only the cobra-derived parts are rewritten. Use -report to list pages that
## still carry placeholder prose.
wiki-commands:
	@CGO_ENABLED=0 go run -mod=vendor ./tools/wikigen -report

## wiki-check — verify the wiki is current and every [[link]] resolves.
wiki-check:
	@CGO_ENABLED=0 go run -mod=vendor ./tools/wikigen -check
	@bash scripts/ci/wiki-link-audit.sh

## flag-drift-audit — checks every `nself <path> --flag` invocation in
## .github/wiki/ and scripts/ against the live cobra tree, so a documented or
## scripted flag that was never registered on the command it names fails the
## build instead of shipping silently (see scripts/ci/flag-drift-audit.sh).
flag-drift-audit:
	@bash scripts/ci/flag-drift-audit.sh

## mcp-docs — CLI-R15. Print the current MCP tool/resource/prompt list,
## generated from cmd/commands/mcp*.go — paste into cmd-mcp.md's PROSE blocks
## by hand after adding/removing a tool, resource, or prompt.
mcp-docs:
	@CGO_ENABLED=0 go run -mod=vendor ./tools/mcpdoc

## parity — CLI-R17. Regenerate the four-surface parity matrix (wiki page, MCP
## tool, env var docs, OpenAPI route) for every top-level command. Writes
## .github/surface-parity.md and .github/surface-parity.json. internal/repoqa
## asserts the committed copies match; run this after adding, renaming, or
## removing a command, an MCP tool, or an env var.
parity:
	@CGO_ENABLED=0 go run -mod=vendor ./tools/parity

## schemas — P7-REG-08. Regenerate schemas/ (JSON Schemas for the envelope, error
## object, command registry and every envelope command's data) from the Go types
## with tools/schemagen. Commit the result; internal/repoqa and `schemas-check`
## fail when the committed files drift.
schemas:
	@CGO_ENABLED=0 go run -mod=vendor ./tools/schemagen

## schemas-check — fail (exit 1) when any schemas/*.json differs from what
## tools/schemagen generates. Non-JSON files under schemas/ are ignored.
schemas-check:
	@CGO_ENABLED=0 go run -mod=vendor ./tools/schemagen -check

## contract-check — P7-REG-10. The one gate for every machine contract the CLI
## publishes: the committed schemas/, the registry and inventory, the compat
## marker table (cheap, so a drift fails in seconds), then the P7-REG Epic's go
## test lines run twice, with NSELF_V15 unset and =1 (ADR 0021). Nothing is skipped: a missing input, a hand-edited
## generated file or a Go type that drifted from its schema exits non-zero with
## a message naming the check and the fix. The registry check regenerates with
## `make cmd-inventory` into the tree, compares against what was committed (or
## hand-edited) before, then restores those files, so a failing run leaves the
## tree as it found it. The Linux smoke (scripts/ci/contract-smoke.sh) is
## separate: it drives the built binary.
CONTRACT_GOTEST := CGO_ENABLED=0 go test -mod=vendor -count=1
CONTRACT_PKGS := ./internal/errs/... ./internal/output/... ./internal/canon/... ./internal/cmdregistry/... \
	./internal/compat/... ./internal/portable/... ./internal/plugin/manifestv2/... ./tools/schemagen/...
CONTRACT_REGEN := .github/command-registry.json .github/command-inventory.json .github/wiki/Commands.md
contract-check:
	@set -e; \
	fail() { echo "contract-check: FAILED: $$1" >&2; echo "contract-check: fix: $$2" >&2; exit 1; }; \
	for f in $(CONTRACT_REGEN) schemas/index.json scripts/ci/compat-markers.sh; do \
	  [ -f "$$f" ] || fail "missing input $$f" "restore it; a gate input that is absent must fail, never pass"; \
	done; \
	echo "contract-check: schemas"; \
	CGO_ENABLED=0 go run -mod=vendor ./tools/schemagen -check \
	  || fail "schemas/ differs from tools/schemagen output" "run make schemas and commit; never hand-edit schemas/"; \
	echo "contract-check: registry and inventory"; \
	snap=$$(mktemp -d "$${TMPDIR:-/tmp}/contract-check.XXXXXX"); \
	restore() { for f in $(CONTRACT_REGEN); do cp "$$snap/$$(basename $$f)" "$$f"; done; rm -rf "$$snap"; }; \
	for f in $(CONTRACT_REGEN); do cp "$$f" "$$snap/$$(basename $$f)"; done; \
	trap restore EXIT; \
	$(MAKE) --no-print-directory cmd-inventory >/dev/null \
	  || fail "make cmd-inventory failed (the registry did not build)" "read the error above; a command missing from internal/canon/canon.yaml is the usual cause"; \
	for f in .github/command-registry.json .github/command-inventory.json; do \
	  cmp -s "$$snap/$$(basename $$f)" "$$f" || { \
	    diff -u "$$snap/$$(basename $$f)" "$$f" | head -40 >&2; \
	    fail "$$f is not what the generator produces (hand edit or stale)" "run make cmd-inventory and commit; never hand-edit generated files"; }; \
	done; \
	restore; trap - EXIT; \
	echo "contract-check: compat markers"; \
	bash scripts/ci/compat-markers.sh --check \
	  || fail "compat-markers.sh --check failed" "run bash scripts/ci/compat-markers.sh --write-wiki and commit"; \
	for mode in unset 1; do \
	  if [ "$$mode" = 1 ]; then NSELF_V15=1; export NSELF_V15; else unset NSELF_V15; fi; \
	  echo "contract-check: go tests (NSELF_V15=$$mode)"; \
	  $(CONTRACT_GOTEST) $(CONTRACT_PKGS) \
	    || fail "contract packages failed (NSELF_V15=$$mode)" "fix the code or regenerate the contract it names"; \
	  $(CONTRACT_GOTEST) ./internal/cmdregistry/... -run Golden \
	    || fail "registry golden failed (NSELF_V15=$$mode)" "run make cmd-inventory and review the diff"; \
	  $(CONTRACT_GOTEST) ./internal/repoqa/... \
	    || fail "repoqa drift tests failed (NSELF_V15=$$mode)" "regenerate the file the test names"; \
	  $(CONTRACT_GOTEST) ./cmd/nself/... \
	    || fail "cmd/nself exit-code tests failed (NSELF_V15=$$mode)" "fix errs.ExitCodeFor wiring"; \
	  $(CONTRACT_GOTEST) ./cmd/commands/ -run 'Registry|Canon|JSONContract|Invocation|Help|Pilot' \
	    || fail "command contract tests failed (NSELF_V15=$$mode)" "fix the command or regenerate the contract it names"; \
	done; \
	unset NSELF_V15; \
	echo "contract-check: OK"

## Q04 — SBOM generation (local dev target)
## Requires: syft (https://github.com/anchore/syft)
## Generates sbom.spdx.json (SPDX) and sbom.cdx.json (CycloneDX 1.5) from the source tree.
## Run `make sbom` before a release to verify SBOM generation works locally.
sbom:
	@echo "Checking for syft..."
	@if ! command -v syft >/dev/null 2>&1; then \
		echo "syft not found. Install via:"; \
		echo "  curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin"; \
		exit 1; \
	fi
	@echo "Generating SPDX SBOM..."
	@syft packages . --output spdx-json=sbom.spdx.json
	@echo "Generating CycloneDX 1.5 SBOM (Q04)..."
	@syft packages . --output cyclonedx-json=sbom.cdx.json
	@echo ""
	@echo "SBOMs written:"
	@echo "  sbom.spdx.json  (SPDX 2.3)        $$(wc -c < sbom.spdx.json) bytes"
	@echo "  sbom.cdx.json   (CycloneDX 1.5)   $$(wc -c < sbom.cdx.json) bytes"
	@echo ""
	@echo "Verify a release SBOM signature:"
	@echo "  bash tools/sbom/verify.sh v$(VERSION)"
	@echo ""
	@echo "Query SBOM for a package:"
	@echo "  bash tools/sbom/query.sh --local sbom.cdx.json --pkg cobra"

## man — generate man pages for all nself commands into ./man/
man: build
	@mkdir -p man
	@./$(BINARY) man --output man
	@echo "Man pages written to man/"

build:
	CGO_ENABLED=0 go build $(BUILDFLAGS) -mod=vendor -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/nself/

install: build
	cp $(BINARY) /usr/local/bin/$(BINARY)

clean:
	rm -f $(BINARY)
	rm -rf $(DIST_DIR)

test:
	CGO_ENABLED=0 go test -mod=vendor ./...

vet:
	CGO_ENABLED=0 go vet -mod=vendor ./...

## fmt — format all first-party Go source (cmd/ internal/ tools/).
fmt:
	gofmt -w cmd internal tools

## fmt-check — CLI-R01 gate. Same script CI runs; fails on any unformatted file.
fmt-check:
	@bash scripts/ci/gofmt-check.sh

cross: cross-linux cross-darwin

cross-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(BUILDFLAGS) -mod=vendor -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-amd64 ./cmd/nself/
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(BUILDFLAGS) -mod=vendor -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-arm64 ./cmd/nself/

cross-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(BUILDFLAGS) -mod=vendor -ldflags="$(LDFLAGS)" -o $(BINARY)-darwin-amd64 ./cmd/nself/
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(BUILDFLAGS) -mod=vendor -ldflags="$(LDFLAGS)" -o $(BINARY)-darwin-arm64 ./cmd/nself/

dist:
	@mkdir -p $(DIST_DIR)
	@for platform in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		name=$(BINARY)-$(VERSION)-$$os-$$arch; \
		echo "Building $$os/$$arch..."; \
		mkdir -p $(DIST_DIR)/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build $(BUILDFLAGS) -mod=vendor -ldflags="$(LDFLAGS)" -o $(DIST_DIR)/$$name/$(BINARY) ./cmd/nself/; \
		cp README.md LICENSE $(DIST_DIR)/$$name/; \
		tar -czf $(DIST_DIR)/$$name.tar.gz -C $(DIST_DIR) $$name; \
		rm -rf $(DIST_DIR)/$$name; \
	done
	@for arch in amd64 arm64; do \
		name=$(BINARY)-$(VERSION)-windows-$$arch; \
		echo "Building windows/$$arch..."; \
		mkdir -p $(DIST_DIR)/$$name; \
		CGO_ENABLED=0 GOOS=windows GOARCH=$$arch go build $(BUILDFLAGS) -mod=vendor -ldflags="$(LDFLAGS)" -o $(DIST_DIR)/$$name/$(BINARY).exe ./cmd/nself/; \
		cp README.md LICENSE $(DIST_DIR)/$$name/; \
		cd $(DIST_DIR) && zip -qr $$name.zip $$name && cd ..; \
		rm -rf $(DIST_DIR)/$$name; \
	done
	@cd $(DIST_DIR) && if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum *.tar.gz *.zip > checksums.txt; \
	else \
		shasum -a 256 *.tar.gz *.zip > checksums.txt; \
	fi
	@echo "dist/ ready."
