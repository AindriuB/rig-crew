---
name: ingest-review
description: Use when a pull request review is ready to turn each of its lines into a rig finding without duplicates.
---
# Ingest a pull request review

Read a review written in the line format below and add each line as a finding.
Running it twice adds nothing new.

## Line format

One finding per line: `- [<SEV>] <path>:<line> <summary>`

`<SEV>` is one of `P1 P2 P3 STYLE SPEC NOTE`. `<path>:<line>` may be `-` when
there is no location. Lines that do not match are ignored; mention how many in your report.

## Steps

1. Run `rig task show <task> --json` and read the pull request number `<n>`.
   Read the pull request head SHA the review covered (`rig pr view <n>`) and
   keep its first 12 characters as `<sha12>`. Get the review text from the file
   or text you were given.
   Run `rig finding list <task> --json` and note every key, whatever the status.
2. For the matching line number `<i>` (counting matching lines from 1), the key
   is `pr-<n>-<sha12>-<i>`. If that key is already in the list from step 1,
   skip the line. Otherwise run:
   `rig finding add <task> --severity <SEV> --summary '<summary>' --path <path:line> --key pr-<n>-<sha12>-<i> --field by=tester`
   Omit `--path` when the location is `-`.
3. The key names the review's head, so a later review never collides with an
   earlier one, and checking every status first means a review that is ingested
   again adds nothing, even if its findings were fixed. Check
   `rig finding list <task>` afterwards.
4. Report how many lines were added and how many already existed.
5. If blocking findings are now open, run `rig task move <task> changes`.
   Otherwise run `rig task move <task> ready`.

Quote a summary containing an apostrophe by writing it as `'\''`.
