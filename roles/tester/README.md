# tester

## What the role is

The tester checks a pull request. It installs three agents (`tester`,
`exploratory-tester`, `explorer`) and two skills (`pr-review`,
`verification-before-completion`), plus a rules file and the `rig` permissions it
needs. It works under the `solo` workflow and acts as `tester`. It works from the
pull request, not from a task's stored state.

## Install

```
rig-crew install --role tester --harness <harness> --workflow solo --project <dir>
rig-crew install --role tester --harness <harness> --workflow solo --global
```

Use exactly one of `--project` and `--global`. Add `--dry-run` first: it lists
the files, settings and workflow the install would write, and writes nothing. It
needs rig 0.5.0 or later.

## A day with rig

1. `rig pr view` and `rig pr checks` show the head and what the forge reports.
2. `tester` runs the build and suites for that head and reports the minimal failing detail.
3. `exploratory-tester` uses the change as a person would and reports each defect with steps to reproduce.
4. In solo, `pr-review` records each finding with `rig finding add`.
5. `explorer` answers questions about the code. It only reads.

## States and gates

The tester moves no task. It only reads the pull request and records findings.
Its open `P1`, `P2`, `STYLE` and `SPEC` findings block the developer's move to
`reviewed` and the move from `ci_green` to `ready`, which in solo the person
makes. `P3` and `NOTE` do not block.

## Solo mode

In `solo` the tester has no review to post: `pr-review` adds each finding itself
with `rig finding add`, marked `by=tester`. The tester never moves a task, pushes
or opens a pull request.
