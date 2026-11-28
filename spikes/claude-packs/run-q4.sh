#!/usr/bin/env bash
# Q4: do paths: rules load only when a matching file is touched?
# Usage: SCRATCH=<dir from setup.sh> ./run-q4.sh
set -u
S="${SCRATCH:?set SCRATCH to the setup.sh output}"
cd "$S/proj"
ASK='List every token of the form RULE-MARKER-* that appears in your context from project rules, comma separated, or NONE. Reply with only that list on one line, no other text.'
run() { claude -p --model haiku --allowedTools Read -- "$1" < /dev/null 2>&1 | rg -v '^(Permission|Warning)' | tail -1; }
echo "== A no file touched";            run "$ASK"
echo "== B touch docs/notes.txt (no match)"; run "Read docs/notes.txt first, then: $ASK"
echo "== C touch src/a.go (match)";     run "Read src/a.go first, then: $ASK"
echo '*' > .claude/rules/rig-crew/.gitignore
echo "== D as C, with .gitignore '*' in rules/rig-crew/"; run "Read src/a.go first, then: $ASK"
echo "== E as A, with .gitignore"; run "$ASK"
rm -f .claude/rules/rig-crew/.gitignore
