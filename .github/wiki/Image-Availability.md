# Image availability

`nself doctor images` checks every image in the generated image lock. It asks Docker to inspect the locked index digest and each platform digest at the upstream registry and, where configured, the mirror. The command needs network access. Each row shows the upstream and mirror result, a pass, warn, or fail status, and a suggested action.

| Result | Meaning |
|---|---|
| Pass | Both registries have the locked digests. |
| Warn | One registry is missing the digest, the mirror is not configured, or a registry cannot be reached. Network failures never count as a pass. |
| Fail | Neither registry has the required digest. |

Doctor uses its usual exit rule: 0 when all rows pass; 2 for warnings only or 1 for failures in v1.4 mode; 12 or 10 respectively in v1.5 mode.

With `IMAGE_PINNING=lock`, `nself start` checks the image required by every locked service on every start, including after the lock changes. It first looks for the exact upstream digest locally, then pulls that digest from upstream. If that fails, it tries the *same digest* at the mirror. A successful mirror pull writes `.nself/state/image-overrides.yml`, a generated Compose fragment applied last so subsequent Compose calls use the mirror reference. When upstream works again, start removes that override. It never retags the mirror image as the upstream image.

In legacy tag mode, the first-run `docker compose pull` still runs, but a pull failure now stops start and reports E465 rather than being ignored. The diagnosis distinguishes a missing repository or tag, required authentication, rate limiting, and network failures. Run `nself doctor images` and check the reported registry access or digest before retrying.
