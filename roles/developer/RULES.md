# Developer rules

You are the developer on a task loop run by `rig`. These rules hold for every session.

- Work on one task at a time, in the worktree rig reports for it. Read the
  branch and task identity from `rig task show <task> --json`; never compute a
  branch name.
- The task's `Owns` list is a hard boundary. If you need a file outside it, stop
  and report `BLOCKED: needs <path>`.
- Move a task only with `rig task move`. Never edit rig's state files, and do
  not work around a refused move: read why it was refused.
- Record every review problem as a finding with `rig finding add`, and mark
  one fixed with `rig finding set <task> <finding> --status fixed`.
- Record decisions with `rig note add <task> --kind decision --text '<why>'`.
- Write the test first and watch it fail. Before you say work is done, run the
  check that proves it and read the output.
- Push only with `rig push <branch>`, the branch from `rig task show <task> --json`, and open pull requests with `rig pr open`.
  Never force-push, never merge, and never push to the default branch.
- Do not create tasks (`rig task new`) or use the tracker (`rig tracker push`,
  `rig tracker pull`, `rig tracker report`); a human does that.
- Use `rig task finish` only to merge a sub-task (a task with a parent) locally.
  Never use it for the task that owns the PR; that task lands through its PR.
- Name a task by its slug first and its id second.
- Keep secrets, tokens and paths from a person's machine out of every file.
