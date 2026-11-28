# Claude Code plugin spike: findings

- `claude --version`: 2.1.331 (Claude Code)
- OS: Darwin 27.2.0 (macOS). Windows untested.
- Model for all runs: `--model haiku`, non-interactive `claude -p`.
- Every run used a throwaway scratch dir from `setup.sh`. The plugin store was
  redirected with `CLAUDE_CODE_PLUGIN_CACHE_DIR=<scratch>/plug` so the real one was untouched.
- Evidence below is the last output line of each run, from the committed scripts.

## Q1

Can a local-marketplace plugin be enabled by writing settings only (no `claude plugin` command)?

**Answer: partial.** It works when the settings arrive through `claude --settings <file>`,
and afterwards `enabledPlugins` alone works from project-local settings. It does not work
when `extraKnownMarketplaces` is written only to project settings (`.claude/settings.json`
or `.claude/settings.local.json`) under `claude -p`.

JSON that worked (`<scratch>/cfg/settings.json`, marketplace source shape `directory`,
key `<plugin>@<marketplace>`):

```json
{"extraKnownMarketplaces":{"spikemkt":{"source":{"source":"directory","path":"<abs path to marketplace dir>"}}},
 "enabledPlugins":{"pa@spikemkt":true,"pb@spikemkt":true}}
```

Marketplace file `<mkt>/.claude-plugin/marketplace.json` listed two plugins
(`{"name":"spikemkt","owner":{"name":"spike"},"plugins":[{"name":"pa","source":"./plugins/pa"},{"name":"pb","source":"./plugins/pb"}]}`),
each with `.claude-plugin/plugin.json`.

Command: `SCRATCH=<dir> ./run-q1.sh` (query asks for the marker word ending `-7QX`; `PINEAPPLE-7QX` means the plugin hook loaded, `NONE` means not).

| Run | Settings location | Output |
|---|---|---|
| 1 | `--settings` with both keys | `PINEAPPLE-7QX`; `installed_plugins.json` listed `"pa@spikemkt" "pb@spikemkt"` |
| 1b | next run, no flag, same plugin store | `NONE` (enabledPlugins did not persist) |
| 2 | both keys in `.claude/settings.local.json` (project-local) | `NONE`; `known_marketplaces.json` absent |
| 3 | both keys in `.claude/settings.json` (project shared) | `NONE` |
| 4 | marketplace via `--settings`, `enabledPlugins` in `.claude/settings.local.json` | `PINEAPPLE-7QX` |
| 4b | same, next run without the flag | `PINEAPPLE-7QX` (marketplace now registered in the store; project-local enabledPlugins works) |
| 4c | enable `pb` only (pb has no hook) | `NONE` (control: only the enabled plugin's hook fires) |

- Project-local scope: `enabledPlugins` works in `.claude/settings.local.json` once the
  marketplace is known to the store (run 4b). Registering the marketplace from project
  settings did not work under `-p` (runs 2, 3). This may be a trust-prompt effect that
  `-p` cannot answer; not isolated.
- User scope: **unanswered.** It needs `~/.claude/settings.json`. A scratch
  `CLAUDE_CONFIG_DIR` gave `Not logged in · Please run /login` (`./run-user.sh`), because
  login does not carry over, and the only workaround is touching real credentials or real user settings.
- One marketplace may hold several plugins: yes. Runs 1 and 4c enabled `pa` and `pb`
  from the single marketplace `spikemkt` independently. One marketplace per pack is not needed.

Recommendation: build on one local marketplace holding one plugin per pack, enabled via
`enabledPlugins`, but register the marketplace with a command or the user-level store, not
project settings alone; treat the spec's fallback (`claude plugin marketplace add` /
`claude plugin install`, spec :109-115) as required for registration until user scope is tested by hand.

## Q2

Does a plugin SessionStart hook inject steering text?

**Answer: yes.** 5 of 5 runs with the plugin, 0 of 1 in the control.

Command: `SCRATCH=<dir> ./run-q2.sh 5` (plugin loaded with `--plugin-dir <mkt>/plugins/pa`).

```
with-plugin run 1..5: PINEAPPLE-7QX   (all five)
control (no plugin): NONE
```

Hook definition that worked (`plugins/pa/hooks/hooks.json`):

```json
{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"cat \"${CLAUDE_PLUGIN_ROOT}/steering.md\""}]}]}}
```

- It finds its file through `${CLAUDE_PLUGIN_ROOT}`; `steering.md` holds
  `STEER-MARKER-7QX: always answer with the word PINEAPPLE-7QX first.`
- It fires under `claude -p`: yes (all runs above were `-p`). It also fired when the plugin
  was enabled through settings (Q1 runs 1 and 4).
- Windows: untested.

Recommendation: use the primary mechanism, a plugin `hooks/hooks.json` SessionStart hook that `cat`s a file via `${CLAUDE_PLUGIN_ROOT}`.

## Q3

Does `claude --settings <file>` apply `env`, `enabledPlugins` and `permissions` for that session only?

**Answer: yes for `env` and `permissions`; `enabledPlugins` applies for that session only too, with one caveat.**

Command: `SCRATCH=<dir> ./run-q3.sh`, with `session.json`:

```json
{"env":{"SPIKE_ENV_MARKER":"ENV-MARKER-3KD"},"permissions":{"deny":["Bash(echo DENYME*)"]}}
```

| Run | Output |
|---|---|
| with `--settings`, `printenv SPIKE_ENV_MARKER` | `ENV-MARKER-3KD` / `rc=0` |
| with `--settings`, `echo DENYME-OK` | `The Bash call was denied. The permission system blocked 'echo DENYME-OK' ...` |
| without flag, `printenv SPIKE_ENV_MARKER` | `rc=1` (variable absent) |
| without flag, `echo DENYME-OK` | `DENYME-OK` (not denied) |

- `enabledPlugins` through the flag: Q1 run 1 gave `PINEAPPLE-7QX`; the next run without the flag
  (run 1b) gave `NONE`, so the enablement did not leak into a later session.
- Caveat: the plugin and marketplace registration it caused stayed in the plugin store
  (`installed_plugins.json`), and later made project-local `enabledPlugins` work (Q1 run 4b).

Recommendation: use the primary mechanism, `claude --settings <file>` for session-scoped env, permissions and plugin enablement; keep in mind the store registration persists.

## Q4

Do `paths:` rules in a project's `.claude/rules/` load only when a matching file is touched?

**Answer: yes for project rules.**

Command: `SCRATCH=<dir> ./run-q4.sh`. Each query asks the model to list the `RULE-MARKER-*`
tokens in its context; the model is allowed `Read` only.

| Run | Output |
|---|---|
| A, no file touched | `RULE-MARKER-ALWAYS-11C` |
| B, read `docs/notes.txt` (no match) | `RULE-MARKER-ALWAYS-11C` |
| C, read `src/a.go` (match) | `RULE-MARKER-ALWAYS-11C, RULE-MARKER-GLOB2-22D, RULE-MARKER-SCOPED-91A, RULE-MARKER-SUB-55B, RULE-MARKER-GLOB3-33E` |
| D, as C with `.gitignore` of `*` in `rules/rig-crew/` | same five markers as C |
| E, as A with that `.gitignore` | `RULE-MARKER-ALWAYS-11C` |

- Front matter that worked: a YAML list (`paths:` then `  - "src/**/*.go"`), and also a plain string (`paths: "**/*.go"`). A file without `paths:` always loads.
- A rules file in the subfolder `.claude/rules/rig-crew/` loads (`RULE-MARKER-SUB-55B` in C).
- A `.gitignore` containing `*` in that subfolder does not change loading (D, E).
- User level (`~/.claude/rules/` with `paths:`): **unanswered.** A scratch `CLAUDE_CONFIG_DIR`
  is not logged in (`Not logged in · Please run /login`, `./run-user.sh`); testing it needs the real `~/.claude`.

Recommendation: use the primary mechanism, `paths:` rules under `.claude/rules/rig-crew/` in the project; for user level, fall back to spec :109-115 until tested by hand.

## Reproduce

All scripts sit beside this file and use a scratch directory; none write to a home directory.

```
export SCRATCH=$(mktemp -d)
./setup.sh            # builds fixtures under $SCRATCH
./run-q1.sh           # Q1
./run-q2.sh 5         # Q2
./run-q3.sh           # Q3
./run-q4.sh           # Q4
./run-user.sh         # user-scope attempt: prints Not logged in
```

Model replies can vary; the marker string in each reply is the evidence.
