#!/usr/bin/env bash
# Copyright 2026 CloudBlue LLC
# SPDX-License-Identifier: Apache-2.0
#
# Generate PR-level release notes scoped to a single module.
#
# Usage: release-notes.sh <current_tag> <tag_glob> <module_key>
#   current_tag  e.g. sdk/v0.3.0
#   tag_glob     e.g. 'sdk/v*'  (finds the previous tag of the SAME module)
#   module_key   one of: core | sdk | contrib | admin
#
# Emits a "## What's Changed" list containing only the PRs whose changed files
# touched the given module's directory, followed by a filtered
# "## New Contributors" section and the module-scoped Full Changelog link.
# Pure path-based filtering: a PR that edits a root-level file (e.g. Makefile)
# counts as a "core" change even if its intent was another module.
#
# Requires: gh (authenticated via GH_TOKEN), git (full history + tags), jq.
set -euo pipefail

cur="$1"; glob="$2"; key="$3"
repo="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY not set}"

# Previous tag of the same module (newest tag matching the glob that isn't cur).
prev="$(git tag --list "$glob" --sort=-version:refname | grep -vFx "$cur" | head -n1 || true)"

# Raw GitHub-generated notes for the module's commit range.
if [ -n "$prev" ]; then
  raw="$(gh api "repos/$repo/releases/generate-notes" \
    -f tag_name="$cur" -f previous_tag_name="$prev" --jq '.body')"
else
  raw="$(gh api "repos/$repo/releases/generate-notes" -f tag_name="$cur" --jq '.body')"
fi

# Returns 0 if the file list on stdin contains a file belonging to $key.
module_match() {
  case "$key" in
    sdk)     grep -qE '^sdk/' ;;
    admin)   grep -qE '^admin/' ;;
    contrib) grep -E '^plugins/contrib/' | grep -qvE '^plugins/contrib/microsoft/keyvault/' ;;
    # Exclude only the sibling modules (separate go.mod). plugins/reference has
    # no go.mod, so it belongs to the root module and must count as core.
    core)    grep -qvE '^(sdk|plugins/contrib|admin|\.github|docs|\.ai)/' ;;
    *) echo "unknown module key: $key" >&2; exit 2 ;;
  esac
}

# Decide which PRs to keep (space-separated list of surviving PR numbers).
keepers=""
for pr in $(printf '%s\n' "$raw" | grep -oE 'pull/[0-9]+' | sed 's#pull/##' | sort -un); do
  if gh api "repos/$repo/pulls/$pr/files" --paginate --jq '.[].filename' | module_match; then
    keepers="$keepers $pr"
  fi
done

# Re-emit the raw notes, dropping bullet lines whose PR wasn't kept, and
# dropping the "New Contributors" header if nothing survives under it.
printf '%s\n' "$raw" | awk -v keepers="$keepers" '
  BEGIN { split(keepers, a, " "); for (i in a) ok[a[i]] = 1 }
  # Capture a section header; buffer until we know it has content.
  /^## / { flush(); header = $0; have = 0; next }
  # A bullet referencing a PR: keep only if that PR survived.
  /pull\/[0-9]+/ {
    n = $0; sub(/.*pull\//, "", n); sub(/[^0-9].*/, "", n)
    if (ok[n]) { buffer = buffer (buffer ? "\n" : "") $0; have = 1 }
    next
  }
  # Full Changelog (and any other prose) — pass through, flushing first.
  { flush(); print }
  END { flush() }
  function flush() {
    if (header != "") {
      if (have) { print header; print buffer }
      header = ""; buffer = ""; have = 0
    } else if (buffer != "") { print buffer; buffer = "" }
  }
' | cat -s
