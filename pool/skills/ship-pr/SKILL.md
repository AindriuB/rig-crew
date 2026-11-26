---
name: ship-pr
description: Use when a task is verified to push its branch, open the pull request and wait for checks.
---
# Ship a pull request

Only for a task in `verified`. Anything else goes back through `loop-task`.

## Steps

1. Run `rig task show <task> --json`. Confirm the state is `verified` and read
   the branch and verified SHA from it. Confirm the branch head is that SHA;
   if it is not, the task has new commits and must be reviewed again.
2. Run `rig push <task>` in a Bash call of its own. If rig refuses (exit 2),
   report it and stop; do not push another way.
3. Run `rig pr open --head <branch> --title '<title>' --body '<body>'`, with
   the branch from step 1 and a title that starts with the task id. The body says
   what changed and which acceptance criteria are met. Note the pull request number.
4. Run `rig task move <task> pr_open --pr <n>`.
5. Wait for checks: `rig pr wait <n>`, run in the background so it wakes you
   when it ends. Do not poll with sleep loops. Read the result with `rig pr checks <n>`.
6. If checks are green at the verified SHA, run
   `rig task move <task> ci_green --sha <sha>`.
7. If a check fails, read the log, record it with `rig finding add <task>
   --severity P2 --summary '<what failed>' --field by=developer`, and move the
   task to `changes` with `rig task move <task> changes`.

Merging is a human step. Never merge, and never force-push.
