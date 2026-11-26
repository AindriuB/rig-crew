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

- `developer`: implements tasks one at a time. See [roles/developer/README.md](roles/developer/README.md).
- `tester`: runs builds and suites and reports results. See [roles/tester/README.md](roles/tester/README.md).

The product owner and scrum master roles, and the `team` workflow they need,
arrive with rig 0.6.0.

## Scopes

`--project DIR` writes under `DIR`; `--global` writes under your home
directory. For Claude Code the files land in:

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

rig-crew only overwrites files it wrote and has not seen modified. A file you
changed, or one it did not create, stops the install with exit code 1 and
nothing is written. There is no `--force`: move or delete the file yourself and
run the install again.

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
schedule it. Cron's default PATH is only `/usr/bin:/bin`, so it will not find
rig unless the line calls it by absolute path. To refresh every 15 minutes, add
this line with `crontab -e`, replacing both placeholder paths with your own:

```
*/15 * * * * cd /path/to/project && /path/to/rig tracker refresh --all
```

## Licence

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
