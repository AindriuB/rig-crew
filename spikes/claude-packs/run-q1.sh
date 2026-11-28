#!/usr/bin/env bash
# Q1: can a local-marketplace plugin be enabled by writing settings only?
# CLAUDE_CODE_PLUGIN_CACHE_DIR points the plugin store (known_marketplaces.json,
# installed_plugins.json, cache) at a scratch dir so the real one is untouched.
# Usage: SCRATCH=<dir from setup.sh> ./run-q1.sh
set -u
S="${SCRATCH:?set SCRATCH to the setup.sh output}"
cd "$S/proj"
Q='If your session context contains a marker word ending in -7QX that you were told to answer with first, reply with that word only. Otherwise reply NONE.'
cat > "$S/mk-only.json" <<E
{"extraKnownMarketplaces":{"spikemkt":{"source":{"source":"directory","path":"$S/mkt"}}}}
E
cat > "$S/enable-pa.json" <<'E'
{"enabledPlugins":{"pa@spikemkt":true}}
E
run() { claude -p --model haiku "$@" -- "$Q" < /dev/null 2>&1 | rg -v '^(Permission|Warning)' | tail -1; }
fresh() { rm -rf "$S/plug"; mkdir -p "$S/plug"; export CLAUDE_CODE_PLUGIN_CACHE_DIR="$S/plug"; rm -f .claude/settings.json .claude/settings.local.json; }

fresh; echo "== 1 --settings with marketplace + enabledPlugins (flag scope)"
run --settings "$S/cfg/settings.json"
echo "   installed: $(rg -o '"p[ab]@spikemkt"' "$S/plug/installed_plugins.json" | tr '\n' ' ')"
echo "== 1b next run, no flag, same plugin store"; run

fresh; echo "== 2 both keys in .claude/settings.local.json (project-local)"
cp "$S/cfg/settings.json" .claude/settings.local.json; run
echo "   known_marketplaces.json present: $(test -f "$S/plug/known_marketplaces.json" && echo yes || echo no)"

fresh; echo "== 3 both keys in .claude/settings.json (project shared)"
cp "$S/cfg/settings.json" .claude/settings.json; run

fresh; echo "== 4 marketplace via --settings, enabledPlugins in .claude/settings.local.json"
cp "$S/enable-pa.json" .claude/settings.local.json; run --settings "$S/mk-only.json"
echo "== 4b next run, no flag (marketplace now registered in the store)"; run
echo "== 4c enable pb only (pb has no hook)"; echo '{"enabledPlugins":{"pb@spikemkt":true}}' > .claude/settings.local.json; run

rm -f .claude/settings.json .claude/settings.local.json
unset CLAUDE_CODE_PLUGIN_CACHE_DIR
