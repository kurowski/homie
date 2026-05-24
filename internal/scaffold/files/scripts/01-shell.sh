#!/usr/bin/env bash
# Example script: report the user's current login shell.
# Edit or delete this and add your own steps. Scripts run in lexical
# order. Convention: make each script idempotent (guard with
# `command -v X >/dev/null && exit 0` or similar).
set -euo pipefail

current=$(getent passwd "$USER" | cut -d: -f7 || true)
if [ -z "$current" ]; then
  echo "could not determine login shell for $USER"
  exit 0
fi
echo "login shell: $current"
