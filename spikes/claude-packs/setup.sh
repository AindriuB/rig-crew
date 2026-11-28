#!/usr/bin/env bash
# Build the throwaway fixtures under $SCRATCH (default: a fresh mktemp dir).
# Usage: SCRATCH=/some/dir ./setup.sh   -> prints the scratch dir.
set -eu
S="${SCRATCH:-$(mktemp -d)}"
mkdir -p "$S/mkt/.claude-plugin" "$S/mkt/plugins/pa/.claude-plugin" "$S/mkt/plugins/pa/hooks" \
  "$S/mkt/plugins/pb/.claude-plugin" "$S/cfg" "$S/proj/.claude/rules/rig-crew" "$S/proj/src" "$S/proj/docs"

# One marketplace holding two plugins.
cat > "$S/mkt/.claude-plugin/marketplace.json" <<E
{"name":"spikemkt","owner":{"name":"spike"},"plugins":[
 {"name":"pa","source":"./plugins/pa","description":"pack a"},
 {"name":"pb","source":"./plugins/pb","description":"pack b"}]}
E
echo '{"name":"pa","version":"0.0.1","description":"pack a"}' > "$S/mkt/plugins/pa/.claude-plugin/plugin.json"
echo '{"name":"pb","version":"0.0.1","description":"pack b"}' > "$S/mkt/plugins/pb/.claude-plugin/plugin.json"

# Plugin pa: SessionStart hook that cats a file found via ${CLAUDE_PLUGIN_ROOT}.
echo 'STEER-MARKER-7QX: always answer with the word PINEAPPLE-7QX first.' > "$S/mkt/plugins/pa/steering.md"
cat > "$S/mkt/plugins/pa/hooks/hooks.json" <<'E'
{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"cat \"${CLAUDE_PLUGIN_ROOT}/steering.md\""}]}]}}
E

# Settings that enable the plugins via settings only.
cat > "$S/cfg/settings.json" <<E
{"extraKnownMarketplaces":{"spikemkt":{"source":{"source":"directory","path":"$S/mkt"}}},
 "enabledPlugins":{"pa@spikemkt":true,"pb@spikemkt":true}}
E

# Session settings file for Q3.
cat > "$S/session.json" <<'E'
{"env":{"SPIKE_ENV_MARKER":"ENV-MARKER-3KD"},
 "permissions":{"deny":["Bash(echo DENYME*)"]}}
E

# Q4 rules.
cat > "$S/proj/.claude/rules/scoped.md" <<'E'
---
paths:
  - "src/**/*.go"
---
RULE-MARKER-SCOPED-91A: when this rule is loaded, say RULE-MARKER-SCOPED-91A.
E
cat > "$S/proj/.claude/rules/rig-crew/sub.md" <<'E'
---
paths:
  - "src/**/*.go"
---
RULE-MARKER-SUB-55B: when this rule is loaded, say RULE-MARKER-SUB-55B.
E
echo 'RULE-MARKER-ALWAYS-11C: unconditional.' > "$S/proj/.claude/rules/always.md"
printf -- '---\npaths: "**/*.go"\n---\nRULE-MARKER-GLOB2-22D: string form.\n' > "$S/proj/.claude/rules/glob2.md"
printf -- '---\npaths:\n  - "**/*.go"\n---\nRULE-MARKER-GLOB3-33E: list form.\n' > "$S/proj/.claude/rules/glob3.md"
git init -q "$S/proj"
echo 'package src' > "$S/proj/src/a.go"
echo 'plain notes' > "$S/proj/docs/notes.txt"
echo "$S"
