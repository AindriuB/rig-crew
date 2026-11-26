---
name: ingest-review
description: Use when a pull request review is ready to turn each of its lines into a rig finding without duplicates.
---
# Ingest a pull request review

Read a review that starts with a header line, then add each finding line as a
finding. Running it twice adds nothing new.

## Line format

The first line is the header:
`## <owner/repo>#<n> · <verdict> · head <sha12>`

`<verdict>` is one of `Blocking`, `Changes suggested`, `Looks good`. `<sha12>`
is exactly 12 lowercase hex characters: the head the review covered.

Then one finding per line: `- [<SEV>] <path>:<line> <summary>`

`<SEV>` is one of `P1 P2 P3 STYLE SPEC NOTE`. `<path>:<line>` may be `-` when
there is no location. Lines after the header that do not match are ignored;
mention how many in your report.

## Steps

Do these in order. A refusal prints one line, adds nothing and moves no task.

1. Get the review text from the file or text you were given. If its first line
   is not the header above (missing or malformed), refuse.
2. Run `rig task show <task> --json` and read the task's pull request number.
   If the header's `<n>` is not that number, refuse.
3. Run `rig pr view <n>` and read the current head SHA. If `<sha12>` is not a
   prefix of it, the review is stale: refuse, print both SHAs, and tell the user
   to re-run pr-review.
4. Run
   `rig finding list <task> --json --status open --status fixed --status wontfix --status deferred`
   and note every key. (The list shows open findings only by default.)
5. For each finding line, number it `<i>` counting finding lines from 1. The key
   is `pr-<n>-<sha12>-<i>`, built from the header's SHA, never from `rig pr view`.
   If that key is already in the list from step 4, skip the line. Otherwise run:
   `rig finding add <task> --severity <SEV> --summary '<summary>' --path <path:line> --key pr-<n>-<sha12>-<i> --field by=tester`
   Omit `--path` when the location is `-`.
6. The key names the review's head, so a later review never collides with an
   earlier one, and checking every status first means a review that is ingested
   again adds nothing, even if its findings were fixed. Check
   `rig finding list <task>` afterwards.
7. Report how many lines were added and how many already existed.
8. If blocking findings are now open, run `rig task move <task> changes`.
   Otherwise run `rig task move <task> ready`.

Quote a summary containing an apostrophe by writing it as `'\''`.
