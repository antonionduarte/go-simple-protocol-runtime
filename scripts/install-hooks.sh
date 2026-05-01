#!/usr/bin/env bash
#
# scripts/install-hooks.sh — install git hooks from scripts/hooks/ into
# .git/hooks/. Idempotent: safe to re-run.
#
# Currently installs only pre-push. There is deliberately no pre-commit
# hook — commits are unblocked so iteration stays fast; the local
# correctness gate is pre-push. CI is authoritative for lint.

set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

HOOKS_DIR=".git/hooks"
SRC_DIR="scripts/hooks"

if [[ ! -d "$HOOKS_DIR" ]]; then
  echo "error: $HOOKS_DIR not found (is this a git checkout?)" >&2
  exit 1
fi

# Install each hook file present in scripts/hooks/ except the README.
for src in "$SRC_DIR"/*; do
  name=$(basename "$src")
  [[ "$name" == "README"* ]] && continue
  dest="$HOOKS_DIR/$name"
  cp "$src" "$dest"
  chmod +x "$dest"
  echo "installed $dest"
done

# Remove any stale pre-commit left from a previous iteration of this
# project's hook story — we deliberately do not install one.
if [[ -f "$HOOKS_DIR/pre-commit" && ! -e "$SRC_DIR/pre-commit" ]]; then
  rm -f "$HOOKS_DIR/pre-commit"
  echo "removed stale $HOOKS_DIR/pre-commit (no pre-commit hook is provided)"
fi

echo
echo "git hooks installed. Bypass with --no-verify; CI is authoritative."
