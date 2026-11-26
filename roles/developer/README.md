# developer

## What the role is

The developer implements tasks, reviews them and ships the pull request. It
installs three agents (`implementer`, `code-reviewer`, `explorer`) and seven
skills (`pick`, `loop-task`, `ship-pr`, `ingest-review`,
`test-driven-development`, `systematic-debugging`,
`verification-before-completion`), plus a rules file and the `rig` permissions
it needs. It works under the `solo` workflow and acts as `developer`.

## Install

```
rig-crew install developer --harness <harness>
```

Add `--dry-run` first to see every path the install would touch. It needs rig
0.5.0 or later.

## A day with rig

1. `pick` lists what is ready with `rig next` and starts a task in its own worktree.
2. `loop-task` implements it test first, hands it to `code-reviewer`, fixes the
   findings and moves the task to `verified`.
3. `ship-pr` pushes the branch, opens the pull request and waits for checks.
4. When a reviewer has written a review, `ingest-review` turns its lines into findings.
5. `explorer` answers questions about the code, and about the forge when the
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
| ci_green | ready | no open P1, P2, STYLE or SPEC finding |
| any review state | changes | counts a round, at most four |

Merging is a human step.

## Solo mode

In `solo` one agent plays every part: it implements, reviews through the
`code-reviewer` agent, and ships. A human refines the task before it is
planned and merges after it is ready. The developer cannot create tasks or push
to the tracker.
