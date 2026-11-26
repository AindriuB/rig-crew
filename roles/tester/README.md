# tester

## What the role is

The tester checks a pull request. It installs three agents (`tester`,
`exploratory-tester`, `explorer`) and two skills (`pr-review`,
`verification-before-completion`), plus a rules file and the `rig` permissions it
needs. It works under the `solo` workflow and acts as `tester`. It works from the
pull request, not from a task's stored state.

## Install

```
rig-crew install tester --harness <harness>
```

Add `--dry-run` first to see every path the install would touch. It needs rig
0.5.0 or later.

## A day with rig

1. `rig pr view` and `rig pr checks` show the head and what the forge reports.
2. `tester` runs the build and suites for that head and reports the minimal failing detail.
3. `exploratory-tester` uses the change as a person would and reports each defect with steps to reproduce.
4. `pr-review` writes one review with a header and one line per finding.
5. `explorer` answers questions about the code. It only reads.

## States and gates

The tester moves no task and is not a gate in the `solo` workflow. It only reads
the pull request and records findings. Open `P1`, `P2`, `STYLE` and `SPEC`
findings block the developer's move to `reviewed` and to `ready`; `P3` and `NOTE`
do not.

## Solo mode

In `solo` the tester has no review to post: `pr-review` adds each finding itself
with `rig finding add`, marked `by=tester`. The tester never moves a task, pushes
or opens a pull request.
