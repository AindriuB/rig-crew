#!/usr/bin/env bash
# User-scope Q1 and Q4 under CLAUDE_CONFIG_DIR=<scratch>, never the real config dir.
# Usage: SCRATCH=<dir from setup.sh> ./run-user.sh
set -u
S="${SCRATCH:?set SCRATCH to the setup.sh output}"
export CLAUDE_CONFIG_DIR="$S/userhome"
rm -rf "$CLAUDE_CONFIG_DIR"; mkdir -p "$CLAUDE_CONFIG_DIR/rules"
cd "$S/proj"
rm -rf .claude/rules; mkdir -p .claude
echo "== auth check under scratch config dir"
claude -p --model haiku -- 'Reply with the word OK.' < /dev/null 2>&1 | tail -1
echo "== U1 user settings.json with extraKnownMarketplaces + enabledPlugins"
cp "$S/cfg/settings.json" "$CLAUDE_CONFIG_DIR/settings.json"
claude -p --model haiku -- 'If your session context contains a marker word ending in -7QX that you were told to answer with first, reply with that word only. Otherwise reply NONE.' < /dev/null 2>&1 | tail -1
echo "== U2 user-level paths: rule"
printf -- '---\npaths:\n  - "src/**/*.go"\n---\nRULE-MARKER-USER-77F: user-level scoped rule.\n' > "$CLAUDE_CONFIG_DIR/rules/u.md"
ASK='List every token of the form RULE-MARKER-* that appears in your context, comma separated, or NONE. Reply with only that list on one line.'
echo "-- no file touched";   claude -p --model haiku --allowedTools Read -- "$ASK" < /dev/null 2>&1 | tail -1
echo "-- touch docs/notes.txt"; claude -p --model haiku --allowedTools Read -- "Read docs/notes.txt first, then: $ASK" < /dev/null 2>&1 | tail -1
echo "-- touch src/a.go";    claude -p --model haiku --allowedTools Read -- "Read src/a.go first, then: $ASK" < /dev/null 2>&1 | tail -1
