# developer

## What the role is

The developer implements tasks, reviews them and ships the pull request. It
installs three agents (`implementer`, `code-reviewer`, `explorer`) and six
skills (`pick`, `loop-task`, `ship-pr`, `test-driven-development`,
`systematic-debugging`, `verification-before-completion`), plus a rules file and the `rig` permissions
it needs. It works under the `solo` workflow and acts as `developer`.

## Install

```
rig-crew install --role developer --harness <harness> --workflow solo --project <dir>
rig-crew install --role developer --harness <harness> --workflow solo --global
```

Use exactly one of `--project <dir>` and `--global`. Add `--dry-run` first: it
lists the files, settings and workflow the install would write, and writes
nothing. It needs rig 0.5.0 or later.

## A day with rig

1. `pick` lists what is ready with `rig next` and starts a task in its own worktree.
2. `loop-task` implements it test first, hands it to `code-reviewer`, fixes the
   findings and moves the task to `verified`.
3. `ship-pr` pushes the branch, opens the pull request and waits for checks.
4. `explorer` answers questions about the code, and about the forge when the
   checkout is not enough. It only reads.

## States and gates

The developer moves a task through these states of the `solo` workflow.

| From | To | Gate |
|---|---|---|
| planned | implementing | dependencies merged |
| implementing | reviewed | no open P1, P2, STYLE or SPEC finding |
| reviewed | verified | the given SHA is the branch head |
| verified | pr_open | a pull request number |
| pr_open | ci_green | the SHA is the verified SHA |
| ci_green | ready | not the developer: the person in solo mode, or the reviewer role in a team, moves it once there is no open P1, P2, STYLE or SPEC finding |
| any review state | changes | counts a round, at most four |

Merging is a human step.

## Solo mode

In `solo` one agent plays every part: it implements, reviews through the
`code-reviewer` agent, and ships. A human refines the task before it is
planned and merges after it is ready. The developer never merges, not even a sub-task, and does not run
`rig task finish`. It cannot create tasks, or push to, pull from or report to the tracker.
