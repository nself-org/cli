# Config System

ɳSelf uses environment variable files for all configuration. A layered cascade lets you separate team defaults from personal overrides from production secrets, keeping the right things committed to git and the right things off of it.

---

## The Env Cascade

When ɳSelf loads configuration, it reads env files in order, with later files overriding earlier ones:

```
.env.dev        ← Team defaults (committed to git)
    ↓
.env.staging    ← Staging overrides (committed, only loaded if ENV=staging)
    ↓
.env.prod       ← Production overrides (committed, only loaded if ENV=prod)
    ↓
.env.secrets    ← Production secrets (GITIGNORED — never commit this)
    ↓
.env.local      ← Local personal overrides (GITIGNORED)
    ↓
.env            ← Highest priority overrides (GITIGNORED)
```

The cascade is additive, each layer only needs to contain the values it wants to override. A variable set in `.env` beats the same variable in `.env.dev`. Most teams only need two files in practice: `.env.dev` for shared team defaults and `.env.secrets` for passwords and API keys.

Environment-specific files (`.env.staging`, `.env.prod`) are skipped unless the `ENV` variable matches. Running `nself start` in a dev environment will never load `.env.prod`.

---

## What Goes Where

| File | Contents | Committed? |
|------|----------|------------|
| `.env.dev` | `POSTGRES_VERSION`, `HASURA_VERSION`, service toggles, non-sensitive defaults | Yes |
| `.env.staging` | Staging-specific URLs, feature flags | Yes |
| `.env.prod` | Production URLs, CORS domains | Yes |
| `.env.secrets` | `POSTGRES_PASSWORD`, `HASURA_GRAPHQL_ADMIN_SECRET`, API keys, tokens | **No , gitignored** |
| `.env.local` | Your personal port overrides, debugging flags | No , gitignored |
| `.env` | Emergency overrides, highest priority | No , gitignored |

---

## .env.secrets Is Gitignored

This is the most important rule in the config system: **never put secrets in `.env.dev` or `.env.prod`.**

`.env.secrets` is gitignored by default and is where all sensitive values belong:

- Database passwords (`POSTGRES_PASSWORD`)
- Hasura admin secret (`HASURA_GRAPHQL_ADMIN_SECRET`)
- Third-party API keys and tokens
- Auth provider client secrets
- Any credential that must not appear in version control

If `.env.secrets` does not yet exist on a server or local machine, ɳSelf will tell you which required secrets are missing when you run `nself doctor` or `nself build`. Populate the file manually or with your team's secret management tool.

---

## .env.computed, Never Hand-Edit

After `nself build` runs, it writes `.env.computed` with derived values that other services need. This file is generated automatically, **do not edit it by hand.**

Values written to `.env.computed` include:

- `DATABASE_URL`, constructed from the individual `POSTGRES_*` variables
- `DOCKER_NETWORK`, the project-scoped Docker network name
- `AUTH_ALLOWED_REDIRECT_URLS`, computed from `BASE_DOMAIN` plus the configured frontend routes

Every time you run `nself build`, `.env.computed` is regenerated from scratch. Any manual changes will be overwritten. If a derived value is not what you expect, trace it back to the source variables and fix those instead.

---

## Smart Defaults

You do not need to set every variable. ɳSelf ships with sensible defaults so minimal projects need minimal configuration. Key defaults:

| Variable | Default |
|----------|---------|
| `ENV` | `dev` |
| `BASE_DOMAIN` | `local.nself.org` |
| `POSTGRES_VERSION` | `16-alpine` |
| `HASURA_VERSION` | `v2.44.0` |

Password handling varies by environment:

- **Dev**, if no password is set, ɳSelf auto-generates a weak local password and logs it. This is intentional: low friction for local development.
- **Staging / Prod**, strong passwords are required. ɳSelf enforces a minimum of **16 characters** for `POSTGRES_PASSWORD` and **32 characters** for `HASURA_GRAPHQL_ADMIN_SECRET`. The build will refuse to proceed if these requirements are not met.

---

## ENV Normalization

The `ENV` variable accepts common aliases and normalizes them automatically:

| You write | ɳSelf reads as |
|-----------|----------------|
| `development`, `develop`, `devel` | `dev` |
| `production`, `prod` | `prod` |
| `staging`, `stage` | `staging` |

This means `ENV=production` and `ENV=prod` are equivalent, use whichever matches your team's convention.

---

## nself doctor

Running `nself doctor` validates your configuration before you deploy:

- Checks that all required variables are set for the current environment
- Validates password strength (dev: warning, staging/prod: hard failure)
- Confirms no wildcard CORS domains are configured in production
- Reports misconfigured or conflicting ports
- Verifies that service toggles are internally consistent, for example, flagging if a licensed plugin that requires Redis is enabled while Redis is disabled

Run `nself doctor` after setting up a new environment and after any significant change to your env files. It is also run automatically as part of `nself build`.

---

**See also:** [[Architecture]] | [[Service-Graph]] | [[Compose-Generation]] | [[Home]]
