# SDK Releasing — Trusted Publisher Setup

This page documents how trusted-publisher (OIDC keyless) publishing is configured for each
SDK registry. Use this when creating a new SDK release or troubleshooting publish failures.

---

## TypeScript SDK (`@nself/plugin-sdk` on npm)

**Workflow:** `sdk/ts/.github/workflows/sdk-ts-publish.yml`

**OIDC permission set:** `id-token: write` — enables npm provenance via OIDC.

### Setup (npm Trusted Publisher)

1. Sign in to [npmjs.com](https://npmjs.com) with the `@nself` org owner account.
2. Navigate to the package settings for `@nself/plugin-sdk`.
3. Under **Automation tokens**, generate a token scoped to `Publish` only and add it as the
   `NPM_TOKEN` repository secret in `nself-org/cli` → Settings → Secrets → Actions.
4. Alternatively, npm Provenance (no secret needed) works when the workflow has
   `id-token: write` and `npm publish --provenance` is called. Current workflow uses both
   paths: provenance is attached automatically when `NPM_TOKEN` is set.

**Trigger:** push a tag matching `sdk-ts/v*` (e.g. `sdk-ts/v2.1.0`).

---

## Python SDK (`nself-plugin-sdk` on PyPI)

**Workflow:** `sdk/py/.github/workflows/sdk-py-publish.yml`

**OIDC permission set:** `id-token: write` — enables PyPI Trusted Publisher (no API token needed).

### Setup (PyPI Trusted Publisher)

1. Sign in to [pypi.org](https://pypi.org) using the nSelf publisher account.
2. Go to the project page for `nself-plugin-sdk` → **Publishing** tab.
3. Click **Add a new publisher** and fill in:
   - Owner: `nself-org`
   - Repository: `cli`
   - Workflow filename: `sdk-py-publish.yml`
   - Environment name: (leave blank)
4. No secret token is needed once the trusted publisher is configured. The
   `pypa/gh-action-pypi-publish` action uses the OIDC token directly.

**Trigger:** push a tag matching `sdk-py/v*` (e.g. `sdk-py/v2.1.0`).

---

## Summary Table

| SDK | Registry | Method | Secret needed |
|-----|----------|--------|---------------|
| TypeScript | npm | OIDC provenance + NPM_TOKEN | `NPM_TOKEN` |
| Python | PyPI | OIDC Trusted Publisher | none (after setup) |

---

[[Home]] | [[Release-Process]]
