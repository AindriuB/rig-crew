# Architecture

The shape of the system: what the pieces are, what talks to what, and where the
boundaries are. `explorer` and `architect` read this before inferring
structure from the filesystem, so keeping it honest saves every session a
search.

**Status: draft, in design.**

## What rig-crew is

One repository holding a common pool of agent prompts and skills, one thin
folder per human role (product owner, scrum master, developer, tester), rig
workflow files, and a Go installer that renders a chosen role for a chosen AI
harness at a chosen scope:

```
rig-crew install --role developer --harness claude --workflow team --project .
rig-crew install --role tester    --harness claude --workflow solo --global
```

It depends on `rig` as an external tool on `PATH`. It does not import rig's
code and does not change it.

## Content: data, not code

```
pool/prompts/<agent>.md               an agent's prompt, shared by every harness
pool/skills/<name>/SKILL.md           procedures and slash commands alike
roles/<role>/role.json                agents, skills, actor, rig allow/deny, other read-only commands
roles/<role>/RULES.md, README.md
workflows/<name>.json                 rig workflow files (team, solo)
harnesses/<h>/agents/<agent>.*        native agent definition for that harness
harnesses/<h>/models.json             fast/strong -> that harness's model IDs
schema/                               role.schema.json
```

**Neutral only where rig-crew owns the concept.** A role's choice of agents
and skills, its actor, its rig allow/deny, the other read-only commands its
agents may run (forge search, for example) and its workflows are neutral, in
`role.json`, because rig-crew defines them. An agent's definition is not:
tools, permissions, hooks, delegation and model fields are written natively
per harness, beside a shared Markdown prompt. kiro-kit tried a neutral agent
model with a compiler and abandoned it for this split. A drift test keeps each
agent's name, description, prompt and skills the same across harnesses.

**Everything user-invocable is a skill.** Claude Code has folded custom
commands into skills, Codex has deprecated custom prompts in their favour, and
all three harnesses read `SKILL.md` folders. There is no `commands/` folder.

Roles, workflows, prompts and skills are found by listing folders. Adding one is a
folder, not code. `--from DIR` overlays a local folder with the same layout,
so a team adds its own without forking. A harness is a folder plus one Go
adapter registered by name; that is the only extension that needs code.

## Components

| Package | Owns |
|---|---|
| `crew` (repo root, `embed.go`) | the `embed.FS` over `pool/ roles/ workflows/ harnesses/ schema/`. It sits at the root because `go:embed` cannot reach a parent directory. |
| `cmd/rig-crew` | commands: flag parsing, policy, text and `--json` output, exit codes |
| `internal/catalog` | loads the content from an `fs.FS` (embedded, overlaid by `--from`), strictly: unknown fields, and a role naming an agent or skill that does not exist, are load errors |
| `internal/model` | neutral types: `Role`, `Skill`, `Prompt`, `Workflow` |
| `internal/plan` | the `Plan` type a harness returns and install applies |
| `internal/harness` | port: `Harness` turns (catalog, role, scope) into a `Plan`; it reads its own `harnesses/<h>/` folder |
| `internal/harness/claude` | adapter, v1 |
| `internal/harness/fake` | adapter for tests |
| `internal/install` | applies a `Plan` to disk: manifest, merges, conflicts, dry-run, uninstall |
| `internal/rigcli` | runs `rig version` and `rig workflow validate` as subprocesses |

## How they talk

```
catalog ──> harness.Render(role, scope) ──> Plan ──> install.Apply
                                                       │
                                     --dry-run prints it and stops
```

- **Render is pure.** It reads the catalog and returns a `Plan`: files to own,
  entries to merge into shared files, environment, and *reports* (anything the
  harness cannot express, such as a rig deny it cannot enforce). It never
  touches the disk. Golden tests run on it per role × harness.
- **Install is the only writer.** It compares the plan with the manifest and
  the disk, stops on conflict, then writes.

## Scopes

Borrowed from kiro-kit's installer: exactly one of `--global` or
`--project DIR`, on `install` and `uninstall` alike.

- The two scopes are independent as to files. Installing or removing one never
  touches the other. Where a harness lets a project definition shadow a global
  one of the same name, that is the harness's rule, not ours. Settings are not
  independent at run time: Claude Code combines `permissions.deny` from user
  and project settings and checks deny before allow at any scope, so a role's
  `rig.deny` from a `--global` install applies in every project, while one
  from a `--project` install applies only inside that project. `env` keys
  such as `RIG_ACTOR` come from the most specific scope.
- Both scopes use the same relative layout inside the harness's directory,
  where the harness allows it.

Where a role lands (surveyed 2026-11-25 from each harness's current docs;
recheck when an adapter is planned, as these formats move quickly):

| Piece | Claude Code (v1) | Kiro (next) | Codex (later) |
|---|---|---|---|
| Agents | `{~,.}/.claude/agents/<n>.md`, YAML front matter; project shadows global | `{~,.}/.kiro/agents/<n>.json`; project shadows global | `{~,.}/.codex/agents/<n>.toml` |
| Rules | `{~,.}/.claude/rules/rig-crew-<role>.md`, owned whole | `{~,.}/.kiro/steering/rig-crew-<role>.md`, owned whole | `AGENTS.md`, one delimited block |
| Skills and commands | `{~,.}/.claude/skills/<n>/SKILL.md`; global shadows project | `{~,.}/.kiro/skills/<n>/SKILL.md` | `~/.agents/skills/`, `.agents/skills/` |
| rig allow/deny | `permissions.allow/deny` as `Bash(rig task move *)`; deny wins across scopes; enforced | `permissions.rules` inside each agent file | `.codex/rules/*.rules`, `prefix_rule`; experimental |
| `RIG_WORKFLOW`, `RIG_ACTOR` | settings `env` | none documented: print `export` lines | `config.toml` `shell_environment_policy.set` |

Claude Code settings go to `~/.claude/settings.json` at `--global` and to
`.claude/settings.local.json` at `--project`: `RIG_WORKFLOW` is an absolute
path on one machine, and a role belongs to one person, so neither belongs in a
committed file. Kiro custom agents see no steering or skills unless their
`resources` list them, so the Kiro adapter adds those entries. Kiro has two
agent formats (V2 and V3); V3 skips a V2 agent until it is upgraded, so the
adapter targets V3 and records the engine in the manifest.

## Ownership and safety

- **Manifest.** Each scope's harness directory holds a rig-crew manifest
  listing every file it owns with its content hash, and every entry it merged
  into a shared file.
- **Owned files** (agents, skills, rules files) are written whole and hashed
  with SHA-256. An owned file whose hash no longer matches, or an unowned file
  at that path that differs from what we would write, stops the operation
  with the full list of paths. An unowned file that is already byte-identical
  is adopted. There is no `--force` in v1. Customisations belong in separately
  named files the installer never owns.
- **Shared files** are merged, never replaced. Settings (JSON, and later
  TOML): each added allow/deny entry and env key is recorded and removed on
  uninstall, nothing else. A rules file only a harness's single instructions
  file can hold (Codex `AGENTS.md`) gets one delimited block per role, hashed
  like an owned file.
- **Writes are guarded.** An install takes an exclusive lock file in the
  scope, refuses symlinks and junctions on every path it touches, re-checks the
  manifest and each target under the lock, writes atomically, and rolls back
  everything it wrote if any write fails.
- **Idempotent.** A second identical run changes nothing and says so.
- **No tokens.** rig-crew never reads or writes credentials. `rig login` and
  the tracker's own token setup stay the user's job.

## rig integration

- **Workflow file.** The chosen workflow is copied to
  `~/.rig/workflows/<name>.json` (spec D1) whatever the scope, and
  `RIG_WORKFLOW` is set to that absolute path in the harness's environment at
  the chosen scope, with `RIG_ACTOR` set to the role's actor. A harness with no
  environment setting gets printed `export` lines instead (spec O4).
- **Shared workflow file.** Several scopes may use one workflow file. The
  workflow manifest in `~/.rig/workflows/` records which scopes reference it;
  it is overwritten only when unmodified and removed when the last one
  uninstalls.
- **Minimum rig.** `install` checks `rig version`: 0.5.0 for `solo` and the
  developer and tester roles; 0.6.0 for `team` and the product-owner and
  scrum-master roles.
- **Hooks may call rig.** rig releases `store.lock` before it runs a move's
  `enter:` hooks, so `team.json` hooks can run `rig tracker report`.

## Models

An agent carries a hint, `fast` or `strong`, never a model ID. Each harness
folder maps the hints to its current IDs (`harnesses/<h>/models.json`). A
user-owned `rig-crew-models.json` in the scope's harness directory (for Claude
Code, `.claude/rig-crew-models.json`), which the installer never writes, can pin a model per agent; `null`
means the harness's own default. Pins are resolved into the native field at
install time, so changing them needs a reinstall.

## Boundaries that must not be crossed

- Only `internal/install` writes to disk, and only to paths the plan names.
- Only an adapter knows a harness's file formats. `cmd/` and `catalog` name no
  harness.
- Nothing in `pool/`, `roles/` or `workflows/` names a harness. Everything
  harness-specific lives under `harnesses/<h>/`.
- Role allow entries are plain words: lowercase letters, digits and hyphens,
  single spaces between words. `*` and `|` are refused at load in v1; wildcards
  are revisited when the scrum-master role is planned.
- No role's deny entry may overlap, by word prefix, an allow entry of another
  role, because a deny from a global install, or from a project install in the
  same project, overrides another role's allow. A role's own prohibitions
  are expressed by leaving the command out of its allow list, so it prompts,
  and in its `RULES.md`. Task 09's test `TestDrift_DenyDoesNotOverlapAllow`
  (AC12) enforces it.
- Re-applying a role converges on its plan: settings entries, keys, files and
  workflow references the role recorded earlier but no longer plans are
  removed, exactly as dropped owned files are.
- rig-crew never edits `~/.rig/config.json`; `rig config set` refuses the
  `workflow` key on purpose.

## Decisions worth knowing

- 2026-11-25 — One public repository, Apache-2.0, written fresh with the
  agent kit as a reference only (spec D3–D5).
- 2026-11-25 — Agent definitions native per harness, prompts and skills
  shared; rejected: one neutral agent schema with `x-<harness>` blocks (spec
  D7), which kiro-kit tried and abandoned.
- 2026-11-25 — Skills only, no separate commands; rejected: `pool/commands/`
  (spec D6), now deprecated or folded into skills by every target harness.
- 2026-11-25 — `--global | --project DIR` scopes, as in kiro-kit; rejected: a
  project-only default with user level opt-in (spec D9).
- 2026-11-25 — Workflow files under `~/.rig/workflows/` (spec D1); rejected: a
  project-local copy, which would leave rig run from a terminal on a different
  workflow.
- 2026-11-25 — Go, standard library only, like rig. Front matter is a flat
  `key: value` subset parsed in-house; rejected: a YAML dependency.
- 2026-11-25 — O2: no scheduled tracker sync; the README documents a cron
  line running `rig tracker refresh --all`, with `PATH` set in the crontab for
  `rig` and `gh` (task 10 Amendment A). O3: revisit `rig sync` when rig phase 4 ships. O5: Codex is checked
  when its adapter is planned. O6: a pool skill that follows an upstream
  closely gets its licence checked and a `NOTICE` line. O7: GitHub issues for
  the worked examples. O8: a separate binary, not a rig subcommand, in v1.
- 2026-11-25 — The loop docs and plan history stay untracked, as in rig.
