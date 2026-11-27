#!/usr/bin/env bash
# Creates the release tag <tag> at commit <sha> in $GITHUB_REPOSITORY with gh, as the release workflow does when it
# is started by hand:
#   scripts/create-release-tag.sh <tag> <sha>
# Refuses a tag that is not vX.Y.Z, optionally with a prerelease part such as v1.3.0-rc.1, and a tag that exists.
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: scripts/create-release-tag.sh <tag> <sha>" >&2
  exit 2
fi
tag=$1
sha=$2

if ! [[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "The version $tag is not vX.Y.Z, such as v1.2.3 or v1.3.0-rc.1" >&2
  exit 1
fi

# matching-refs lists every tag starting with $tag, so only an exact match counts.
existing=$(gh api "repos/$GITHUB_REPOSITORY/git/matching-refs/tags/$tag" \
  --jq "map(select(.ref == \"refs/tags/$tag\")) | length")
if [ "$existing" != 0 ]; then
  echo "The tag $tag already exists" >&2
  exit 1
fi

gh api --silent "repos/$GITHUB_REPOSITORY/git/refs" -f "ref=refs/tags/$tag" -f "sha=$sha"
echo "Created the tag $tag at $sha"
