#!/usr/bin/env bash
# Purpose: promote nself/nself:latest to a published versioned tag, but only while
#          that tag is still GitHub's latest release. Runs inside the shared
#          `docker-publish-latest` concurrency group, so promotions are serial and
#          the eligibility read below is current at the moment of the write (D-0204).
#          Without the recheck, an older release's run that resolved latest=true
#          and finished after a newer release would move :latest backwards.
# Inputs (env):
#   TAG        release tag being promoted, v<ver> (steps.version.outputs.tag)
#   REPO       owner/name, e.g. nself-org/cli
#   GH_TOKEN   read access to the repo's releases (gh reads it)
#   IMAGE      image repository (default nself/nself)
# Outputs: exit 0 after promoting, or after skipping because TAG is no longer
#          GitHub's latest (a ::notice:: line says which); exit 1 on a failed read
#          or a failed `imagetools create`.
# Constraints: bash + gh + docker buildx only; bash 3.2 compatible.
set -euo pipefail

: "${TAG:?TAG is required}"
: "${REPO:?REPO is required}"
IMAGE="${IMAGE:-nself/nself}"

LATEST_TAG=$(gh release view -R "${REPO}" --json tagName -q .tagName) || {
  echo "::error::docker-publish: cannot read ${REPO}'s latest release." >&2
  exit 1
}

if [ "${LATEST_TAG}" != "${TAG}" ]; then
  echo "::notice::docker-publish: ${TAG} is no longer the latest release (${LATEST_TAG} is); leaving ${IMAGE}:latest alone."
  exit 0
fi

docker buildx imagetools create -t "${IMAGE}:latest" "${IMAGE}:${TAG#v}"
