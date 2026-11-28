---
name: tech-item
description: Use to create a technical item, such as refactoring, debt or platform work, that needs no product input.
---
# Create a technical item

Create one item that needs only technical input.

## Steps

1. Read `rig-crew-planning.json` at the project root (defaults when absent).
   Note `kinds` and `estimate`.
2. Ask what the work is and why it matters. Choose a `kind` from `kinds` that
   fits (for example `chore` or `enabler`); ask if none does.
3. Create it, with a short slug, a title and a body that states the problem:
   `rig task new --slug <slug> --title '<title>' --body '<body>' --field needs=tech --field kind=<kind> --actor tech-lead`
4. Set `acceptance`: testable criteria that show the work is done. It is a
   list field, so pass a JSON array; a plain value is split on commas:
   `rig task set <task> --field acceptance='["criterion one","criterion two"]' --actor tech-lead`
5. Fill the tech fields following `tech-shape`: `approach`, `nfrs`, `risks`,
   `depends` and `size` (empty when `estimate` is `none`). Each is set with
   `rig task set <task> --field <name>=<value> --actor tech-lead`. `nfrs` and
   `risks` are list fields, so pass a JSON array; a plain value is split on
   commas and breaks an entry that contains one. `depends` is a plain comma
   list of slugs.
6. If it belongs under another item, link it with
   `rig task parent <task> <parent> --actor tech-lead`.
7. List the fields still empty and stop.

Do not move the task to another state.
