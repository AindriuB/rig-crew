---
name: tech-shape
description: Use to fill the technical fields of an item: approach, NFRs, risks, dependencies and size.
---
# Shape an item technically

Fill the tech fields of one item. Readiness is guidance: report what is
missing, never refuse to proceed.

## Steps

1. Read `rig-crew-planning.json` at the project root (use the defaults when it
   is absent). Note `estimate`.
2. Run `rig task show <task> --json` and read the item. Read its product
   fields as given; do not change them.
3. Set `approach`: a short note on how it will be built, or a link to a
   decision record (use `decision-record` when the choice is not obvious).
   `rig task set <task> --field approach='<text>' --actor tech-lead`
4. Walk the NFR checklist, asking whether each applies: security, performance,
   reliability, operability, accessibility, data and privacy, compatibility.
   Set `nfrs` to the list of those that apply, each with its target. It is a list field, so pass a JSON array; a plain
   value is split on commas and breaks an entry that contains one:
   `rig task set <task> --field nfrs='["performance: p95 under 200 ms","security: no PII in logs"]' --actor tech-lead`
5. Set `risks`: every risk or unknown gets a closing entry, either the slug of
   a spike task that will resolve it or `accepted`. Offer to create a spike
   (kind `spike`) for each open one. Do not leave a risk without one. It is a list field, so pass a JSON array,
   each entry whole:
   `rig task set <task> --field risks='["cold cache on deploy: accepted","unknown rate limit: spike-rate-limit"]' --actor tech-lead`
6. Set `depends` to the tasks this one depends on, as a comma list of slugs:
   `rig task set <task> --field depends=<slug-a>,<slug-b> --actor tech-lead`
7. Set `size` in the unit named by `estimate`. When `estimate` is `none`, leave
   `size` empty.
8. List the tech fields still empty and say so. Stop there.

Every `rig` write passes `--actor tech-lead`. Do not move the task to another
state.
