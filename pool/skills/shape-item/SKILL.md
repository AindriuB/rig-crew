---
name: shape-item
description: Use to fill the item contract for one work item, so it carries what its role needs before it is planned.
---
# Shape one item

Fill the contract fields of one item. Readiness is guidance: never refuse an
item and never gate on it. Report what is missing and let the user decide.

## Actor

Every `rig` write below passes `--actor <role>`, where the role is
`product-owner` or `tech-lead`. Use the value of `RIG_ACTOR` when it is set.
Otherwise ask the user once per session which role they are acting as, and
reuse the answer.

## Steps

1. Read `rig-crew-planning.json` at the project root. Every key is optional;
   when the file or a key is absent use the defaults: `levels`
   `["feature","task"]`, `kinds` `["feature","bug","spike","enabler","chore"]`,
   `estimate` `none`, `breakdown` `before`, `decisions` format `adr`, dir
   `docs/decisions`.
2. Name the item. Run `rig task show <task> --json` for an existing one, or
   create it with `rig task new --slug <slug> --title '<title>' --actor <role>`.
3. Set `needs` first, then `kind`, with
   `rig task set <task> --field needs=<product|tech|both> --actor <role>` and
   `rig task set <task> --field kind=<kind> --actor <role>`. `kind` must be one
   of the team's `kinds`. If the item has a parent, link it with
   `rig task parent <task> <parent> --actor <role>`.
4. Fill the fields your role owns, asking the user for anything you cannot
   find. Set each with `rig task set <task> --field <name>=<value> --actor <role>`.
   - Product: `problem`, `source`, `audience`, `outcome` (the success measure
     and how it is observed), `scope`, `non-goals`, `acceptance` (testable
     criteria), `priority`, `rationale` (value and risk).
   - Tech: `approach` (a note, or a link to a decision record when the
     `decisions` threshold is met), `nfrs`, `risks` (each closed by a spike's
     slug or `accepted`), `depends`, `size` (in the team's `estimate` unit;
     leave empty when it is `none`).

   `acceptance`, `nfrs` and `risks` are list fields, and a plain value is split
   on commas, which breaks an entry like "Given X, when Y, then Z" or "cold
   cache on deploy, accepted". Pass each as a JSON array:
   `--field acceptance='["criterion one","criterion two"]'`, and the same form
   for `nfrs` and `risks`.

   Do not overwrite a field the other role has filled unless the user asks.
5. List the fields still empty for the inputs the item needs: product fields
   when `needs` is `product` or `both`, tech fields when `tech` or `both`. Show
   the list; do not block on it.
6. Move the item to planned only when its needed inputs are in and the user
   says so: `rig task move <task> planned --actor <role>`. Either role may do
   this. If inputs are missing and the user still says so, say what is
   missing, then follow the user.
