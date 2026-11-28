---
name: discover
description: "Use to frame the problem behind an item: who it is for, what they need and where the need came from."
---
# Discover the problem

Fill `problem`, `source` and `audience` on one item.

## Steps

1. Run `rig task show <task> --json`. Read what is already set. Never
   overwrite a field the user has not asked you to change.
2. Ask who it is for and what they need. Do not guess: one question at a
   time, and wait for the answer.
3. Offer prompts when the user is stuck.
   Brainstorming:
   - What is the smallest group that feels this most?
   - What do they do today instead, and what does that cost them?
   - What would they stop doing if this were solved?

   Interview, to put to a real person:
   - Tell me about the last time this came up.
   - What did you try? What went wrong?
   - What would you have wanted to happen instead?
   - Who else is affected?
4. Draft the three fields and show them to the user:
   - `problem`: the problem or need, in the user's words, without a solution.
   - `source`: where the need came from: a link, a person or an issue key.
   - `audience`: who it is for.
5. After the user agrees, write them:

   ```
   rig task set <task> --field problem='<text>' --field source='<text>' --field audience='<text>' --actor product-owner
   ```
6. Read `needs` from the item. If it is `tech` or `both`, say that the tech
   lead enriches the item next, and leave the technical fields alone.

This skill sets only `problem`, `source` and `audience`. It does not change
the item's state.
