# rig-crew

Role setups for [rig](https://github.com/AindriuB/rig), installed into an AI
coding harness.

## What rig-crew is

rig-crew holds a pool of agent prompts and skills, and the `rig-crew` command
that installs a role from that pool into a harness. A role is a set of agents,
skills, rules, a rig actor and a rig allow/deny list. The installer renders it
into the files the harness reads natively and sets `RIG_WORKFLOW` and
`RIG_ACTOR` so that rig knows which workflow and actor the harness runs as.

## Install

Install the binary (needs Go):

```
go install github.com/AindriuB/rig-crew/cmd/rig-crew@latest
```

rig-crew needs rig 0.5.0 or later on your `PATH`. Install a role into one
project, or for your whole user account:

```
rig-crew install --role developer --harness claude --workflow solo --project ~/code/my-app
rig-crew install --role developer --harness claude --workflow solo --global
```

Use exactly one of `--project DIR` and `--global`. Add `--dry-run` to see the
plan without writing anything, and `rig-crew uninstall` with the same role,
harness and scope to remove it again. Installing twice changes nothing the
second time.

## Roles

- `developer`: implements tasks one at a time. See [packs/developer/README.md](packs/developer/README.md).
- `tester`: runs builds and suites and reports results. See [packs/tester/README.md](packs/tester/README.md).

The product owner and scrum master roles, and the `team` workflow they need,
arrive with rig 0.6.0.

## Scopes

`--project DIR` writes under `DIR`, which must be an existing directory;
`--global` writes under your home directory. For Claude Code the files land in:

| File | Project | Global |
|---|---|---|
| Agents | `.claude/agents/` | `~/.claude/agents/` |
| Rules | `.claude/rules/rig-crew-<role>.md` | `~/.claude/rules/rig-crew-<role>.md` |
| Skills | `.claude/skills/<name>/SKILL.md` | `~/.claude/skills/<name>/SKILL.md` |
| Settings | `.claude/settings.local.json` | `~/.claude/settings.json` |

Project settings go to `.claude/settings.local.json`, which is not meant to be
committed, because it holds an absolute path. The workflow is copied to
`~/.rig/workflows/<name>.json` at either scope, and `RIG_WORKFLOW` (that path)
and `RIG_ACTOR` are set in the settings `env`.

An existing file identical to what rig-crew would write is adopted. Settings
files are merged and your own entries are kept. A settings file stops the
install if it is not valid JSON (including duplicate keys), if
`permissions.allow` or `permissions.deny` is not an array, if `env` or another
key on the path is not an object, or if `env.RIG_WORKFLOW` or `env.RIG_ACTOR`
is set to a different value that rig-crew did not write for this role. Any
other file rig-crew did not write, or one it wrote and that has changed since,
also stops the install. A stop exits with code 1 and writes nothing. There is no
`--force`: move or delete the file yourself and run the install again.

## What each harness enforces

| Harness | Rig allow/deny | Notes |
|---|---|---|
| Claude Code | Enforced through settings permissions | Agents, rules and skills are native files. |
| Kiro | Not yet supported | |
| Codex | Not yet supported | |

Anything a harness cannot enforce is advice only. The installer reports each
such limitation when you install.

## Keeping the board fresh

rig reads the board from a local cache that `rig tracker refresh --all`
refreshes for every task already linked to an issue. rig-crew does not
schedule it. Cron's default PATH is only `/usr/bin:/bin`. With a GitHub
tracker rig runs `gh` from PATH, and with a GitLab tracker it runs `glab`, so
cron needs rig and that tool on PATH; an absolute path to rig alone is not
enough. Cron does not expand `~` in variables, so give `RIG_WORKFLOW` as an
absolute path: without it rig does not find the workflow the installer set up
in your settings. To refresh every 15 minutes, add these lines with
`crontab -e`, replacing the placeholder paths with your own:

```
PATH=/path/to/rig-dir:/path/to/gh-or-glab-dir:/usr/bin:/bin
RIG_WORKFLOW=/absolute/path/to/home/.rig/workflows/<name>.json
*/15 * * * * cd /path/to/project && rig tracker refresh --all
```

## Licence

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
