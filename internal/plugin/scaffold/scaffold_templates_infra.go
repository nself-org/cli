package scaffold

// scaffold_templates_infra.go — infrastructure and DevOps template strings.
//
// Purpose: Hold all Go const template strings for plugin.json, database
//          migrations, Hasura metadata, Docker Compose, .dockerignore,
//          .air.toml, README, and CI workflow. Separated from scaffold.go
//          so that logic functions and template data do not co-reside in one
//          file.
// Inputs:  none (package-level consts, referenced by buildFiles and
//          addTenancyFiles in scaffold.go).
// Outputs: Exported const strings consumed by render/renderAny at run time.
// Constraints: All consts here are Go text/template strings. Use backtick
//              literals only. No logic allowed in this file.
// SPORT:   cli/internal/plugin/scaffold — decomposed from scaffold.go (T-E2-06).

// tmplPluginJSON is the plugin.json template. It requires a struct with all
// Params fields plus MultiAppSupported (bool) and IsolationColumn (string).
const tmplPluginJSON = `{
  "name": "{{.Name}}",
  "version": "0.1.0",
  "description": "{{.Description}}",
  "author": "{{.Author}}",
  "license": {{if eq .Tier "pro"}}"Source-Available"{{else}}"MIT"{{end}},
  "isCommercial": {{if eq .Tier "pro"}}true{{else}}false{{end}},
  {{- if eq .Tier "pro"}}
  "licenseType": "pro",
  "requiredEntitlements": ["pro"],
  "requires_license": true,
  {{- end}}
  "homepage": "https://nself.org/plugins",
  "minNselfVersion": "{{.MinCLI}}",
  "minSdkVersion": "{{.MinSDK}}",
  "category": "{{.Category}}",
  {{- if .Bundle}}
  "bundle": "{{.Bundle}}",
  {{- end}}
  "multiApp": {
    "supported": {{.MultiAppSupported}},
    "isolationColumn": "{{.IsolationColumn}}"
  },
  "tags": ["{{.Name}}"]
}
`

// tmplMigrationNone is emitted when the plugin stores no per-user Postgres data.
const tmplMigrationNone = `-- {{.Name}} initial migration
-- No multi-tenancy columns required for this plugin.
-- Add your schema here.

CREATE TABLE IF NOT EXISTS np_{{.Name}}_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
`

// tmplMigrationAppIsolation is emitted for multi-app isolation within one nSelf deploy.
// Uses source_account_id per the Multi-Tenant Convention Wall.
const tmplMigrationAppIsolation = `-- {{.Name}} initial migration
-- Multi-app isolation: source_account_id separates independent consumer apps
-- within one nSelf deploy. See: docs/architecture/multi-tenant-conventions.md

CREATE TABLE IF NOT EXISTS np_{{.Name}}_items (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id  TEXT NOT NULL DEFAULT 'primary',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Row-level security: each app sees only its own rows.
ALTER TABLE np_{{.Name}}_items ENABLE ROW LEVEL SECURITY;

CREATE POLICY np_{{.Name}}_items_isolation ON np_{{.Name}}_items
    USING (source_account_id = current_setting('app.source_account_id', true));
`

// tmplMigrationCloudTenant is emitted for Cloud SaaS tenancy.
// Uses tenant_id UUID per the Multi-Tenant Convention Wall.
const tmplMigrationCloudTenant = `-- {{.Name}} initial migration
-- Cloud multi-tenancy: tenant_id separates paying customers in nSelf Cloud SaaS.
-- See: docs/architecture/multi-tenant-conventions.md
-- NEVER use tenant_id for per-app isolation within one deploy — use the app-isolation column instead.

CREATE TABLE IF NOT EXISTS np_{{.Name}}_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Row-level security: each tenant sees only its own rows.
ALTER TABLE np_{{.Name}}_items ENABLE ROW LEVEL SECURITY;

CREATE POLICY np_{{.Name}}_items_tenant ON np_{{.Name}}_items
    USING (tenant_id = current_setting('app.tenant_id', true)::UUID);
`

// tmplMigrationBoth emits both columns for developers who are unsure which
// convention they need. Remove the unused column before going to production.
const tmplMigrationBoth = `-- {{.Name}} initial migration
-- Both multi-tenancy columns included. Remove the one you do not need before
-- going to production. See: docs/architecture/multi-tenant-conventions.md
--
--   source_account_id: per-app isolation within one nSelf deploy
--   tenant_id:         Cloud SaaS paying-customer isolation

CREATE TABLE IF NOT EXISTS np_{{.Name}}_items (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_account_id  TEXT NOT NULL DEFAULT 'primary',
    tenant_id          UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE np_{{.Name}}_items ENABLE ROW LEVEL SECURITY;

-- Choose ONE of the two policies below and delete the other.
CREATE POLICY np_{{.Name}}_items_isolation ON np_{{.Name}}_items
    USING (source_account_id = current_setting('app.source_account_id', true));

-- CREATE POLICY np_{{.Name}}_items_tenant ON np_{{.Name}}_items
--     USING (tenant_id = current_setting('app.tenant_id', true)::UUID);
`

// tmplHasuraNoFilter is the Hasura metadata stub for plugins that use
// source_account_id (app isolation) or no tenancy at all. No tenant row filter
// is required because isolation is handled at the Postgres RLS layer.
const tmplHasuraNoFilter = `{
  "table": {
    "schema": "public",
    "name": "np_{{.Name}}_items"
  },
  "select_permissions": [
    {
      "role": "user",
      "permission": {
        "columns": "*",
        "filter": {}
      }
    }
  ]
}
`

// tmplHasuraCloudFilter is the Hasura metadata stub for Cloud multi-tenant
// plugins. The row filter enforces that each tenant only sees its own rows via
// the X-Hasura-Tenant-Id session variable.
const tmplHasuraCloudFilter = `{
  "table": {
    "schema": "public",
    "name": "np_{{.Name}}_items"
  },
  "select_permissions": [
    {
      "role": "user",
      "permission": {
        "columns": "*",
        "filter": {
          "tenant_id": {
            "_eq": "X-Hasura-Tenant-Id"
          }
        }
      }
    }
  ]
}
`

const tmplCompose = `# docker-compose.plugin.yml for {{.Name}}
# Merged into the generated stack by ` + "`nself build`" + `. Do not hand-edit.
services:
  {{.Name}}:
    image: nself/{{.Name}}:${{"{"}}{{.EnvPrefix}}_VERSION:-latest}
    container_name: ${PROJECT_NAME:-nself}_{{.Name}}
    restart: unless-stopped
    environment:
      LOG_LEVEL: ${LOG_LEVEL:-info}
      DATABASE_URL: ${DATABASE_URL}
      {{.EnvPrefix}}_LISTEN_ADDR: ":{{.Port}}"
    ports:
      - "127.0.0.1:{{.Port}}:{{.Port}}"
    networks:
      - nself_net
networks:
  nself_net:
    external: true
`

const tmplDockerignore = `.git
.gitignore
README.md
Dockerfile
docker-compose*.yml
.air.toml
tmp/
*.test
coverage.out
`

const tmplAirToml = `# air.toml — hot-reload for {{.Name}} dev (pair with nself plugin dev)
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/{{.Name}} ./cmd"
  bin = "tmp/{{.Name}}"
  delay = 500
  include_ext = ["go", "yaml", "yml"]
  exclude_dir = ["tmp", "vendor", ".git"]

[log]
  time = true

[color]
  app = "magenta"
`

const tmplReadme = `# {{.PascalName}} Plugin

{{.Description}}

Tier: ` + "`{{.Tier}}`" + `{{if .Bundle}}  ·  Bundle: ` + "`{{.Bundle}}`" + `{{end}}  ·  Category: ` + "`{{.Category}}`" + `

## Local development

` + "```bash" + `
go mod tidy
go test ./...
go run ./cmd        # runs on :{{.Port}}
` + "```" + `

With hot-reload (install [air](https://github.com/air-verse/air)):

` + "```bash" + `
nself plugin dev {{.Name}}
` + "```" + `

## Endpoints

- ` + "`GET /healthz`" + ` — liveness
- ` + "`GET /readyz`" + ` — readiness
- ` + "`GET /metrics`" + ` — Prometheus metrics
- ` + "`GET /version`" + ` — plugin version
- ` + "`GET /v1/hello`" + ` — starter handler

## License

{{if eq .Tier "pro"}}Source-Available (Licensed plugin in a Bundle). Requires an active nSelf license key.{{else}}MIT.{{end}}
`

const tmplCI = `name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
      - name: Test
        run: go test ./...
      - name: Build Docker image
        run: docker build -t {{.Name}}:test .
      - name: Health check
        run: |
          docker run -d -p {{.Port}}:{{.Port}} --name test {{.Name}}:test
          sleep 2
          curl -f http://localhost:{{.Port}}/healthz
          docker stop test
`
