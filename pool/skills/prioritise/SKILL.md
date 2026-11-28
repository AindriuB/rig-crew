---
name: prioritise
description: Use to order a set of items and record each one's priority and the rationale, naming value and risk.
---
# Prioritise items

Fill `priority` and `rationale` on each item in a set.

## Steps

1. Run `rig task list --json` and choose the set with the user: all open items
   or a named group. Run `rig task show <task> --json` for each to read
   `problem`, `audience`, `outcome` and `risks` where set.
2. For each item, weigh value (who gains, how much, how soon) against risk
   (what could go wrong, what is unknown, what it blocks). Ask the user for
   anything you cannot read from the items.
3. Propose an order as a short table: item, priority, one-line rationale. Use
   the priority labels the team already uses; if there are none, use `high`,
   `medium` and `low`.
4. Each `rationale` must name both value and risk, for example: "Value: removes
   the daily manual export for the finance team. Risk: low, the data is
   already available."
5. After the user agrees to the order, write each item:

   ```
   rig task set <task> --field priority='<label>' --field rationale='<text>' --actor product-owner
   ```
6. For an item whose `needs` is `tech` or `both`, say that the tech lead
   enriches it, and leave the technical fields alone.

This skill sets only `priority` and `rationale`. It does not change any
item's state, and it does not reorder or link tasks.
