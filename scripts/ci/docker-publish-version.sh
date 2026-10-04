#!/usr/bin/env bash
# Purpose: decide whether and what docker-publish.yml publishes. The only source
#          of the `proceed`, `version` and `latest` step outputs (P7-HYG-06).
# Inputs (env):
#   EVENT_NAME       workflow_run | workflow_dispatch   (github.event_name)
#   HAVE_SECRETS     1 when DOCKERHUB_USERNAME and DOCKERHUB_TOKEN are both set, else 0
#   INPUT_VERSION    dispatch only: version to publish, with or without a leading v;
#                    blank means "GitHub's latest release"
#   RUN_HEAD_BRANCH  workflow_run only: head_branch of the Release run (the tag name)
#   RUN_CONCLUSION   workflow_run only: conclusion of the Release run
#   REPO             owner/name, e.g. nself-org/cli
#   GH_TOKEN         read access to the repo's releases (gh reads it)
#   GITHUB_OUTPUT    file to append outputs to (defaults to stdout)
# Outputs: tag, version (tag without v), latest (true|false), proceed=true.
#          Exit 1 with an ::error:: line on anything else; there is no skip path,
#          so a run that cannot publish is red, never green with nothing pushed.
# Constraints: bash + gh only; bash 3.2 compatible; no registry credential is read.
set -euo pipefail

OUT="${GITHUB_OUTPUT:-/dev/stdout}"
SEMVER_RE='^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$'

fail() {
  echo "::error::docker-publish: $*" >&2
  exit 1
}

: "${EVENT_NAME:?EVENT_NAME is required}"
: "${REPO:?REPO is required}"

# Secrets first: with them absent nothing else matters, and the failure must be
# loud on every event (nself/nself went unpublished for months behind a skip).
if [ "${HAVE_SECRETS:-0}" != "1" ]; then
  fail "DOCKERHUB_USERNAME or DOCKERHUB_TOKEN is not available to this workflow (nself-org org secrets)."
fi

case "${EVENT_NAME}" in
  workflow_run)
    [ "${RUN_CONCLUSION:-}" = "success" ] \
      || fail "the Release run concluded '${RUN_CONCLUSION:-}', not success; no image for this tag."
    TAG="${RUN_HEAD_BRANCH:-}"
    ;;
  workflow_dispatch)
    if [ -n "${INPUT_VERSION:-}" ]; then
      TAG="v${INPUT_VERSION#v}"
    else
      TAG=$(gh release view -R "${REPO}" --json tagName -q .tagName) \
        || fail "no input version and GitHub reports no latest release for ${REPO}."
    fi
    ;;
  *)
    fail "unsupported event '${EVENT_NAME}'."
    ;;
esac

if ! [[ "${TAG}" =~ ${SEMVER_RE} ]]; then
  fail "'${TAG}' is not a release tag (want vMAJOR.MINOR.PATCH[-prerelease])."
fi

# A tag with no published GitHub release is never published as an image.
FLAGS=$(gh release view "${TAG}" -R "${REPO}" --json tagName,isPrerelease,isDraft \
  -q '"\(.isPrerelease) \(.isDraft)"') \
  || fail "tag ${TAG} has no GitHub release in ${REPO}."
IS_PRERELEASE="${FLAGS%% *}"
IS_DRAFT="${FLAGS##* }"
[ "${IS_DRAFT}" = "false" ] || fail "the GitHub release for ${TAG} is a draft."

# :latest only for a non-prerelease that is also GitHub's latest release, so a
# backfill of an older tag never moves :latest backwards.
LATEST=false
if [ "${IS_PRERELEASE}" = "false" ]; then
  LATEST_TAG=$(gh release view -R "${REPO}" --json tagName -q .tagName) \
    || fail "cannot read ${REPO}'s latest release."
  if [ "${LATEST_TAG}" = "${TAG}" ]; then
    LATEST=true
  fi
fi

{
  echo "tag=${TAG}"
  echo "version=${TAG#v}"
  echo "latest=${LATEST}"
  echo "proceed=true"
} >> "${OUT}"
