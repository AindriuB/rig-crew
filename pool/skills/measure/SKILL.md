---
name: measure
description: Use to set an item's outcome: the success metric and how it is observed.
---
# Define the outcome

Fill `outcome` on one item.

## Steps

1. Run `rig task show <task> --json`. Read `problem`, `audience` and
   `acceptance`.
2. Ask what would be different if this worked, for the audience. Turn the
   answer into one metric.
3. Draft `outcome` with three parts:
   - the metric: what is measured, and the target;
   - how it is observed: the source, the person or the check that shows it,
     and when;
   - the baseline, when one is known. If it is not known, say so in the text
     rather than inventing a number.
4. Check the metric against the problem: if it could move without the problem
   being solved, pick a different one.
5. Show the draft. After the user agrees, write it:

   ```
   rig task set <task> --field outcome='<text>' --actor product-owner
   ```
6. Read `needs` from the item. If it is `tech` or `both`, say that the tech
   lead enriches the item next, and leave the technical fields alone.

This skill sets only `outcome`. It does not change the item's state.
