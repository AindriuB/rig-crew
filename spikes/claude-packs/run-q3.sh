#!/usr/bin/env bash
# Q3: does --settings apply env and permissions for that session only?
# Usage: SCRATCH=<dir from setup.sh> ./run-q3.sh
set -u
S="${SCRATCH:?set SCRATCH to the setup.sh output}"
cd "$S/proj"
ENVQ='Run the Bash command: printenv SPIKE_ENV_MARKER ; echo "rc=$?". Reply with the raw command output only.'
DENYQ='Run the Bash command: echo DENYME-OK . Reply with the raw output, or the word DENIED if you were not allowed to run it.'
for flag in with without; do
  if [ "$flag" = with ]; then F="--settings $S/session.json"; else F=""; fi
  echo "== $flag --settings: env"
  claude -p --model haiku --allowedTools "Bash(printenv:*)" $F -- "$ENVQ" 2>&1 | rg -v "^Permission (allow|deny) rule" | tail -3
  echo "== $flag --settings: permissions.deny"
  claude -p --model haiku --allowedTools "Bash(echo:*)" $F -- "$DENYQ" 2>&1 | rg -v "^Permission (allow|deny) rule" | tail -3
done
