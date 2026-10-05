# Errors

← [[Home]]

---

Every error the ɳSelf CLI prints with a code has a stable `[Exxx]` code, a plain-language cause and a fix. The full catalogue, with the exit status of each code, is on [[error-codes]]. How exit statuses are chosen is on [[Exit-Codes]].

The older `ERR-INSTALL-*` catalogue that used to live on this page was removed together with the `internal/errors` package that printed those codes. Nothing in the CLI prints an `ERR-INSTALL-*` code any more. Use these instead:

| Old code | Now |
|----------|-----|
| ERR-INSTALL-001 (Docker not found) | `E001` in [[error-codes]] |
| ERR-INSTALL-002 (Docker daemon not running) | `E002` in [[error-codes]] |
| Any other ERR-INSTALL code | Run `nself doctor` for the check that failed, then search [[error-codes]] by the bracketed code in the message |

Need help? See [[Support]].

---

← [[Home]] | [[Support]]
