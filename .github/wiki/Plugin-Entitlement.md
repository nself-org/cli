# Plugin entitlement

In v1.5 mode (`NSELF_V15=1`), build, start, and `nself license set` refresh the entitlement for installed licensed plugins. The CLI writes the cached, verified ping licence JWT to `.nself/state/entitlement.jwt` atomically with mode `0600`. It mounts that file read-only at `/run/nself/entitlement.jwt` and sets `NSELF_ENTITLEMENT_FILE` to that container path. Free plugins receive neither the mount nor the variable. The JWT and licence key never appear in generated compose or environment files.

If no verified JWT is obtainable, the CLI writes an empty file and reports E125 (exit 3). Default builds always retain the mount, including when `LICENSE_PUBLIC_KEY_OVERRIDE` is set. Only a development build tagged `nself_devkeys` with that override set can skip the mount, with a warning. v1.4 mode leaves generated compose unchanged.

The `github.com/nself-org/cli/sdk/go/v2/license` package verifies compact EdDSA JWTs against committed ping public keys and the header's `kid`. It fails closed for unknown keys, invalid signatures, unsupported algorithms, wrong issuer (`ping.nself.org`), wrong audience (`nself-cli`), or a plugin slug absent from `plugins` (unless `"*"` is present). Runtime environment overrides and key files cannot change the production verifier's trusted keys.

The grace period ends thirty days after the signed `exp`, including the endpoint. Local file timestamps and refresh times do not extend grace. Doctor reports E126 (exit class 1) when the verified file's `exp` is more than seven days old. Unattended refresh scheduling is a separate feature.

The HTTP guard re-reads the file every twenty-four hours, so a refreshed valid token restores service without restarting the plugin. Its refresh worker has an owned lifetime and must be closed with the plugin's server. When `NSELF_ENTITLEMENT_FILE` is set but the file is missing, unreadable, forged, for another plugin, or beyond grace, every route except `/health` returns HTTP 503 with `{"error":"entitlement_expired"}`.

Until v1.6.0, an **unset** `NSELF_ENTITLEMENT_FILE` permits serving and logs one warning. This absence rule supports released v1.4.x CLIs, which never set the variable. It does not permit serving when a configured file is missing or invalid. The compatibility exception sunsets at v1.6.0.

Runtime entitlement is best effort on self-hosted code: an operator can modify the guard, and a copied valid token grants at most its remaining signed grace. Licence-gated downloads provide the primary control. Free plugins and security capabilities do not perform entitlement checks.
