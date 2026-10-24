#!/usr/bin/env bash
# Tests for next-version.sh. Run: bash .github/scripts/next-version.test.sh
set -uo pipefail

script="$(dirname "$0")/next-version.sh"
failures=0

check() {
  local name="$1" last="$2" subjects="$3" expected="$4" actual
  actual=$(printf '%s' "$subjects" | bash "$script" "$last")
  if [[ "$actual" == "$expected" ]]; then
    echo "ok   $name"
  else
    echo "FAIL $name: expected '$expected', got '$actual'"
    failures=$((failures + 1))
  fi
}

check "fix bumps patch" v1.0.18 $'🐛 FIX: crash on empty file\n' 1.0.19
check "improve bumps patch" v1.0.18 $'👌 IMPROVE: faster OCR\n' 1.0.19
check "new bumps minor and resets patch" v1.0.18 $'🐛 FIX: a\n📦 NEW: export\n' 1.1.0
check "breaking bumps major" v1.6.2 $'‼️ BREAKING: drop Windows 10\n📦 NEW: b\n' 2.0.0
check "docs, tests and releases only: no release" v1.0.18 $'📖 DOC: readme\n🤖 TEST: more cases\n🦄 RELEASE: v1.0.18\n' ""
check "docs plus a fix: patch" v1.0.18 $'📖 DOC: readme\n🐛 FIX: a\n' 1.0.19
check "unprefixed commit bumps patch" v1.6.0 $'Bugfix\n' 1.6.1
check "colon without known prefix bumps patch" v1.6.0 $'feat: new thing\n' 1.6.1
check "skip marker on the newest commit: no release" v1.0.18 $'👌 IMPROVE: wip [skip release]\n📦 NEW: export\n' ""
check "skip marker on an older commit is ignored" v1.0.18 $'🐛 FIX: a\n👌 IMPROVE: wip [skip release]\n' 1.0.19
check "no commits: no release" v1.0.18 "" ""
check "no previous tag starts from 0.0.0" v0.0.0 $'📦 NEW: first\n' 0.1.0
check "last line without newline is read" v1.0.18 '📦 NEW: export' 1.1.0

if ((failures > 0)); then
  echo "$failures failing"
  exit 1
fi
echo "all passing"
