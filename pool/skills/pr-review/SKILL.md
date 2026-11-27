---
name: pr-review
description: Use to review one pull request by running the testers and writing a single review in a fixed format, or, in solo mode, to record findings directly.
---
# Review a pull request

Work from the pull request, not from a task's stored state.

## Steps

1. Run `rig pr view <n>` and note the state and the head SHA. If the pull request
   is not open, output `NO REVIEW: <reason>` and stop.
2. Run `rig pr checks <n>` and read what the forge reports.
3. Run the `tester` agent, and the `exploratory-tester` agent when the change is
   user-facing, against that head.
4. Turn every problem into one finding line, held in your notes. Do not add a
   finding to rig yet. Never write a summary paragraph in place of a line.
5. Run `rig pr view <n>` again. If the head changed, the PR is no longer open
   (closed or merged), or you could not finish the
   review, output `NO REVIEW: <reason>` and no header. Never output a partial review.
6. Otherwise, in solo mode (see Solo mode), do not write or post a review: go to
   the Solo mode section, which is the only place a finding is added, and only
   after step 5. In any other mode write the review below and return its text. Post it
   to the forge, with the forge's own command line tool or its web page, only when
   the caller asked for it to be posted.

## Review format

The review begins with this header, exactly:

`## <owner/repo>#<n> · <verdict> · head <sha12>`

`<sha12>` is the first 12 lowercase hex characters of the head from step 1.
`<verdict>` follows the counts: `Blocking` if there is any `P1`, else
`Changes suggested` if there is any `P2`, else `Looks good`.

Then one finding per line:

`- [<SEV>] <path>:<line> <summary>`

`<SEV>` is one of `P1 P2 P3 STYLE SPEC NOTE`. `<path>:<line>` may be `-` for none.

Examples:

```
## acme/widgets#42 · Blocking · head 3f9a1c0d7b2e
- [P1] src/parse.go:88 Input with an empty key panics; the suite never covers it.
- [P2] src/parse.go:120 The documented flag --strict is ignored.
- [P3] README.md:14 The install example names a flag that does not exist.
- [STYLE] src/parse.go:5 Exported function has no doc comment.
- [NOTE] - Build and all suites pass at this head.
```

## Solo mode

You are in solo mode only when the caller's instruction says `solo`. If it does
not, you are not in solo mode.

In solo mode there is no review to post. Once step 5 has confirmed the head is
unchanged and the pull request is still open, add each finding directly:

`rig finding add <task> --severity <S> --summary '<text>' --path <path:line> --field by=tester`

Leave out `--path <path:line>` when the finding has no location (`-`).

First resolve `<task>` from the pull request: take the head branch name from
`rig pr view <n>` and run `rig task list --format json`; `<task>` is the task
whose `branch` field equals it. If no task matches, output `NO REVIEW: no task for branch <branch>`
and stop. Run `rig finding list <task>` next so you do not repeat an open finding. No
header is needed. `P1`, `P2`, `STYLE` and `SPEC` block; `P3` and `NOTE` do not.
