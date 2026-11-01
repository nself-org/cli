# Image Mirror Runbook

This document describes the image mirroring tool used to pull images from their original upstream locations and mirror them into the `nself` Docker Hub namespace. This process helps ensure that `nself` has a resilient supply chain, guarantees digest immutability for shipped containers, and meets AGPL source disclosure requirements for redistributed images.

See [ADR 0030: Image Supply Chain](../../architecture/adr/0030-image-supply-chain.md) for context.

## Mirror Tool (`mirror-images.sh`)

The CLI ships with a dedicated bash script to copy images based on the generated lock file (`internal/compose/images.lock.json`):

```bash
# Dry-run (default). Plans copies and validates that lock digests exist.
scripts/images/mirror-images.sh --plan

# Applies the planned copies to the configured mirror namespace.
scripts/images/mirror-images.sh --apply

# Exits 1 if any pinned index digest is missing from its configured mirror.
scripts/images/mirror-images.sh --check
```

By default, the tool copies images into `docker.io/nself/<name>`.

Checks resolve each lock entry's `mirror@index_digest`, so a moved version tag
cannot hide a missing pin or invalidate an available digest. Copies use the
upstream index digest and verify that the destination preserves it and all
locked platform manifests.

The daily default-image workflow runs this check for every mirrored lock entry,
without an opt-in variable. After lock changes merge, the image-pin-bump workflow
copies missing digests when the owner's `IMAGE_MIRROR_APPLY` switch is enabled.
If the check reports missing mirrors, an authorized maintainer must enable that
switch with registry credentials configured or run `--apply` to repair them.

### Crane Image

To avoid adding Go module dependencies to the CLI and host-level binary requirements, the mirror tool runs `crane` directly from a digest-pinned Google Container Registry image: `gcr.io/go-containerregistry/crane`.

> [!WARNING]
> Bumping the pinned `crane` image digest in the mirror script requires a PR and a review. Always verify the new digest against the upstream repository.

## AGPL Sources

We distribute several AGPL-licensed images (e.g., Elasticsearch, MinIO, Grafana components). As a distributor, we must provide the corresponding source code or a link to it.

`scripts/images/mirror-sources.yaml` contains the upstream source URLs. When you run the mirror tool, it prints a reminder to set the repository description in Docker Hub to include the AGPL source link:

```text
mirror-images: source: minio https://github.com/pgsty/minio (AGPL — set as the pgsty/minio repository description)
```

**Maintainer task:** Whenever a new AGPL image is added or an existing one is updated, ensure that its Docker Hub repository description clearly links to the upstream source URL listed in `mirror-sources.yaml`.

## Handling Offline Archives

For legacy support or specific scenarios (e.g., restoring the legacy MinIO data volume before the migration to SeaweedFS), you may need to mirror from a `docker save` archive:

```bash
scripts/images/mirror-images.sh --archive <tarball> --as <repo:tag>
```

This command pushes the provided tarball to the specified repository and tag.

## Weekly Pin Bump

The weekly `IMAGE_PINNING` dependency bump PR uses `--list-tags` to fetch tags of a repository and `--plan` to verify the state of the mirror:

```bash
scripts/images/mirror-images.sh --list-tags docker.io/pgsty/minio
```
