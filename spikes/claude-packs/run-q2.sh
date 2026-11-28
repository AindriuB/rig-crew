#!/usr/bin/env bash
# Q2: run claude -p N times with the pa plugin loaded via --plugin-dir, and once
# without it as a control. Prints the marker line per run.
# Usage: SCRATCH=<dir from setup.sh> ./run-q2.sh [N]
set -u
S="${SCRATCH:?set SCRATCH to the setup.sh output}"
N="${1:-5}"
Q='If your session context contains a marker word ending in -7QX that you were told to answer with first, reply with that word only. Otherwise reply NONE.'
cd "$S/proj" || exit 1
mkdir -p "$S/plug" && export CLAUDE_CODE_PLUGIN_CACHE_DIR="$S/plug" || exit 1
for i in $(seq 1 "$N"); do
  printf 'with-plugin run %s: ' "$i"
  claude -p --model haiku --plugin-dir "$S/mkt/plugins/pa" "$Q" 2>&1 | tail -1
done
printf 'control (no plugin): '
claude -p --model haiku "$Q" 2>&1 | tail -1
