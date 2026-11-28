---
name: push-tree
description: Use to create an agreed tree of items locally and push it to the tracker after a dry run and the user's yes.
---
# Create a tree and push it

Create the items locally, then push the tree to the tracker. Push sends only
title, body, type, parent and a marker label, so everything in the contract
must be in the body first.

## Actor

Every `rig` write passes `--actor <role>`, where the role is `product-owner`
or `tech-lead`. Use `RIG_ACTOR` when it is set; otherwise ask the user once per
session which role they are acting as.

## Steps

1. Take the agreed list: the root (an existing item or a new one) and its
   children, each with a title and fields.
2. For each new item run
   `rig task new --slug <slug> --title '<title>' --body '<body>' --actor <role>`,
   then set its fields with
   `rig task set <task> --field <name>=<value> --actor <role>` (`needs`,
   `kind`, then the rest of the contract). Pass `acceptance` as a JSON array,
   `--field acceptance='["criterion one","criterion two"]'`, because a plain
   value is split on commas.
3. Link each child with `rig task parent <child> <parent> --actor <role>`.
4. Write the contract into each item's body as Markdown sections, one
   `## <field>` heading per filled field (`problem`, `source`, `audience`,
   `outcome`, `scope`, `non-goals`, `acceptance`, `priority`, `rationale`,
   `approach`, `nfrs`, `risks`, `depends`, `size`), before any push. Write
   the body without an editor: `rig task new --body-file <path>` for a new
   item, `rig task set <task> --field body=<text> --actor <role>` for an
   existing one. Never use `rig task edit`, which opens an interactive editor.
5. Run `rig tracker push <root> --tree --dry-run` and show the user the output.
6. Ask whether to push. Push only after the user says yes:
   `rig tracker push <root> --tree --actor <role>`. On anything but a clear
   yes, stop; the local tree stays.
7. Report what was created and pushed, by slug.
