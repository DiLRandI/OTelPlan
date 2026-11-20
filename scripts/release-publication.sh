#!/bin/sh
set -eu

fail() {
  printf 'release publication: %s\n' "$*" >&2
  exit 1
}

if [ "$#" -ne 5 ]; then
  fail 'usage: release-publication.sh verify|check|publish RUN_ID VERSION SOURCE_SHA NEW_DIRECTORY'
fi
mode=$1
run_id=$2
version=$3
source_sha=$4
directory=$5
repository=DiLRandI/OTelPlan
api="repos/$repository"
case "$mode" in verify|check|publish) ;; *) fail 'unknown publication mode' ;; esac
case "$run_id" in ''|*[!0-9]*) fail 'candidate run ID must be numeric' ;; esac
if [ "${#version}" -gt 64 ] || ! printf '%s\n' "$version" |
    LC_ALL=C grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.(0|[1-9][0-9]*))?$'; then
  fail 'expected vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.N'
fi
printf '%s\n' "$source_sha" | LC_ALL=C grep -Eq '^[0-9a-f]{40}$' || fail 'expected a full source SHA'

check_tag() {
  tag_refs=$(git ls-remote --exit-code "https://github.com/$repository.git" \
    "refs/tags/$version" "refs/tags/$version^{}") || fail 'the maintainer must create the reviewed tag first'
  tag_sha=$(printf '%s\n' "$tag_refs" | awk '
    NR == 1 { tag = $1 }
    $2 ~ /\^\{\}$/ { peeled = $1 }
    END { print peeled ? peeled : tag }
  ')
  [ "$tag_sha" = "$source_sha" ] || fail 'release tag points to a different commit'
}

if [ "$mode" != verify ]; then
  branch=$(gh api "$api/branches/main")
  printf '%s\n' "$branch" | jq -e --arg sha "$source_sha" \
    '.protected == true and .commit.sha == $sha' >/dev/null ||
    fail 'main must be protected and still point to the candidate source'

  environment=$(gh api "$api/environments/release") ||
    fail 'configure the protected release environment before requesting publication'
  printf '%s\n' "$environment" | jq -e '
    .deployment_branch_policy.custom_branch_policies == true
    and any(.protection_rules[]; .type == "required_reviewers"
      and .prevent_self_review == true and (.reviewers | length) > 0)
  ' >/dev/null || fail 'release environment must require independent review and restrict deployment branches'
  policies=$(gh api "$api/environments/release/deployment-branch-policies")
  printf '%s\n' "$policies" | jq -e '
    .total_count == 1 and (.branch_policies | length) == 1
    and .branch_policies[0].name == "main" and .branch_policies[0].type == "branch"
  ' >/dev/null || fail 'release environment must allow only the main branch'

  check_tag
fi

run=$(gh api "$api/actions/runs/$run_id")
attempt=$(printf '%s\n' "$run" | jq -er --arg sha "$source_sha" --arg repo "$repository" '
  select(.conclusion == "success" and .status == "completed"
    and .event == "workflow_dispatch" and .path == ".github/workflows/release-validation.yml"
    and .head_branch == "main" and .head_sha == $sha
    and .repository.full_name == $repo and .head_repository.full_name == $repo)
  | .run_attempt | numbers
') || fail 'candidate needs a successful manual release-validation run on the same main commit'

jobs=$(gh api "$api/actions/runs/$run_id/attempts/$attempt/jobs?per_page=100")
printf '%s\n' "$jobs" | jq -e '
  any(.jobs[]; .name == "release-validation" and .conclusion == "success")
  and any(.jobs[]; .name == "prepare-linux-candidate" and .conclusion == "success")
' >/dev/null || fail 'candidate run did not pass validation and signed packaging'

mkdir -- "$directory"
gh run download "$run_id" --repo "$repository" --name "otelplan-linux-candidate-$attempt" --dir "$directory"
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
sh "$script_dir/verify-release-provenance.sh" "$directory" "$version" "$source_sha" "$directory/provenance.jsonl"
if [ "$mode" != publish ]; then
  printf 'verified staged candidate %s at %s from run %s attempt %s\n' "$version" "$source_sha" "$run_id" "$attempt"
  exit 0
fi

stem="$directory/otelplan_${version}_linux_amd64"
set --
case "$version" in *-rc.*) set -- --prerelease --latest=false ;; esac
gh release create "$version" --repo "$repository" --verify-tag --draft \
  --title "$version" --notes-from-tag "$@" \
  "$stem.tar.gz" "$stem.spdx.json" "$directory/SHA256SUMS" "$directory/provenance.jsonl"
release_id=$(gh release view "$version" --repo "$repository" --json databaseId --jq '.databaseId')
draft=$(gh api "$api/releases/$release_id")
printf '%s\n' "$draft" | jq -e --arg tag "$version" '.draft == true and .tag_name == $tag' >/dev/null ||
  fail 'expected the unpublished draft for the reviewed tag'
expected_assets=$(
  cd "$directory"
  sha256sum -- "otelplan_${version}_linux_amd64.tar.gz" "otelplan_${version}_linux_amd64.spdx.json" SHA256SUMS provenance.jsonl |
    jq -Rn '[inputs | capture("^(?<sha>[a-f0-9]{64})  (?<name>.*)$")
      | {name, digest: ("sha256:" + .sha)}] | sort_by(.name)'
)
printf '%s\n' "$draft" | jq -e --argjson expected "$expected_assets" '
  ([.assets[] | {name, digest}] | sort_by(.name)) == $expected
' >/dev/null || fail 'draft assets disagree with the verified candidate; draft remains unpublished'
check_tag
gh release edit "$version" --repo "$repository" --draft=false
