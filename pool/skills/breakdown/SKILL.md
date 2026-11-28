---
name: breakdown
description: Use to split one item into child items and hand the agreed list to push-tree.
---
# Break an item into children

Propose children for one item, agree the list with the user, then create them.

## Actor

Every `rig` write passes `--actor <role>`, where the role is `product-owner`
or `tech-lead`. Use `RIG_ACTOR` when it is set; otherwise ask the user once per
session which role they are acting as.

## Steps

1. Read `rig-crew-planning.json` (defaults when absent). Run
   `rig task show <task> --json` to see the item's state. If `breakdown` is
   `after` and the item is not yet planned, say this skill is skipped for now
   and stop.
2. Read the item's fields. Use the next level down in `levels` as the child
   level, and a `kind` from `kinds` for each child.
3. Propose the children as a list. For each give a title and the product
   fields it needs: `problem`, `audience`, `outcome`, `scope`, `non-goals`,
   `acceptance`, `priority`. Keep each child small enough to plan alone, and
   say which children depend on which.
4. Revise the list with the user until they agree. Create nothing yet.
5. Hand the agreed list, with its parent, to the `push-tree` skill, which
   creates the children. Do not run `rig task new` here.
