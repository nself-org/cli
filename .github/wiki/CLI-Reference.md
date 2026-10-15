# CLI Reference

See [[Commands]] for the command index and [[Command-Registry]] for the machine-readable contract.

`nself start` resolves `AI_AUTO_INSTALL` through the project environment cascade
before considering local AI installation. A project value of `false` suppresses
installation even when the AI plugin is installed.

`nself ops deploy` checks image architectures only in the compose file it ships.
Architecture probes honor an inventory SSH key reference first, then the normal
deploy key settings (`NSELF_DEPLOY_KEY_PATH`, followed by `NSELF_DEPLOY_SSH_KEY`).

Doctor's vector-search audit conservatively includes inferred callers.
Changing the desired Hasura environment alone cannot
establish that a running container has stopped inferring permissions.

Machine transports refuse requests whose selected flag overrides disable the
JSON envelope. Installed plugin startup failures preserve the forwarded
`--json` intent so the CLI can return its error envelope.
