---
name: enrich
description: Use to add technical detail and sub-tasks to an item another role drafted.
---
# Enrich a drafted item

Add technical detail to an item a product role drafted. Product fields are
theirs: read them, never change them.

## Steps

1. Read `rig-crew-planning.json` at the project root (defaults when absent).
   Note `breakdown`.
2. Run `rig task show <task> --json`. Read `problem`, `outcome`, `scope`,
   `non-goals`, `acceptance`, `priority` and `rationale` as given.
3. Fill only the tech fields that are empty: `approach`, `nfrs`, `risks`,
   `depends` and `size`, following `tech-shape`. If a tech field already has a
   value, show it and ask before changing it. Pass `--actor tech-lead` on every
   write.
4. If something in a product field looks wrong or untestable, say so to the
   user as a note for the product role. Do not edit it.
5. When `breakdown` is `before`, or the item is already planned, propose
   sub-tasks: a title and the fields of each, with its own `acceptance`. When
   `breakdown` is `after` and the item is not yet planned, say this step is
   skipped.
6. Hand the agreed list to `push-tree`. Create nothing until the user agrees
   to the list.

Do not move the task to another state.
