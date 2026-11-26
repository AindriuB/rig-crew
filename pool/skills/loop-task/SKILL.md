---
name: loop-task
description: Use to carry one task from planned or changes to verified by implementing, reviewing and recording findings.
---
# Carry a task to verified

Take one task through implement, review, fix and verify, moving its state with
rig. Use `pick` first if you have no task yet.

## Steps

1. Run `rig task show <task> --json` for the state, branch and worktree. If
   the state is `planned` or `changes`, run `rig task move <task> implementing`.
2. Implement. Hand the work to the `implementer` agent, or do it yourself with
   the `test-driven-development` skill. Work only inside `Owns`, in the
   worktree rig reports.
3. Review. Hand the branch to the `code-reviewer` agent. It records findings
   with `rig finding add`. Read them with `rig finding list <task> --blocking`.
4. If blocking findings are open, run `rig task move <task> changes`, fix
   exactly those findings, mark each with
   `rig finding set <task> <finding> --status fixed --actor developer`, then go
   back to step 1. The workflow allows only a few rounds; if the move is
   refused for that reason, stop and ask the human.
5. When no blocking finding is open, run `rig task move <task> reviewed`. If
   rig refuses, read why, fix the cause and do not force it.
6. Verify with `verification-before-completion`: run the task's checks and
   confirm each acceptance criterion. Get the head commit SHA from git, then
   run `rig task move <task> verified --sha <sha>`.
7. Stop at `verified`. Opening the pull request is `ship-pr`.

## Rules

- A new commit after review sends the task back to `implementing`; review again.
- Record a decision with `rig note add <task> --kind decision --text '<why>' --actor developer`.
- If you cannot continue, run `rig task move <task> stopped --reason '<why>'`
  and tell the human. Do not work around a refusal.
