---
name: pick
description: Use to choose the next task for the agent to work on and start it.
---
# Pick the next task

Choose one task, read it, start it.

## Steps

1. Run `rig next --who agent --json` to list what an agent can act on now. If
   the user named a task, skip to step 3.
2. Choose by this order: a task in `changes` (it has open findings), then one
   in `planned`. Among equals, take the first rig lists, since it already
   orders by dependency. If nothing is listed, run `rig status` and report why,
   rather than inventing work.
3. Run `rig task show <task> --json`. Read the state, the `Owns` list, the
   acceptance criteria and the dependencies. If a dependency is not merged,
   pick another task or report the block.
4. Run `rig context <task>` for the brief and read the task file it names.
5. Run `rig task start <slug>`. Read the branch and worktree from
   `rig task show <task> --json` and work in that worktree, never in the main checkout.

Name a task by slug first and id second when you speak to the user. Never
compute a branch name; rig reports the one in use.

If `rig task start` exits non-zero, stop and report it. Do not fall back to the main checkout.
