#!/usr/bin/env bash
# Next release version, computed from the commit subjects since the last tag.
# Usage: git log --no-merges --format=%s v1.2.3..HEAD | next-version.sh v1.2.3
#
# Subjects arrive newest first. The highest bump wins:
#   ‼️ BREAKING -> major, 📦 NEW -> minor, 📖 DOC / 🤖 TEST / 🦄 RELEASE -> none,
#   anything else (👌 IMPROVE, 🐛 FIX, unprefixed) -> patch.
# "[skip release]" in the newest subject postpones the release to the next push.
# Prints nothing when no release is due.
set -euo pipefail

last="${1:-v0.0.0}"
IFS=. read -r major minor patch <<<"${last#v}"

bump=0 # 0 none, 1 patch, 2 minor, 3 major
newest=1
while IFS= read -r subject || [[ -n "$subject" ]]; do
  if [[ $newest == 1 && "$subject" == *"[skip release]"* ]]; then
    exit 0
  fi
  newest=0

  level=1
  if [[ "$subject" == *:* ]]; then
    case "${subject%%:*}" in
      *BREAKING) level=3 ;;
      *NEW) level=2 ;;
      *DOC | *TEST | *RELEASE) level=0 ;;
    esac
  fi
  if ((level > bump)); then
    bump=$level
  fi
done

case $bump in
  0) exit 0 ;;
  1) patch=$((patch + 1)) ;;
  2) minor=$((minor + 1)) && patch=0 ;;
  3) major=$((major + 1)) && minor=0 && patch=0 ;;
esac
echo "$major.$minor.$patch"
