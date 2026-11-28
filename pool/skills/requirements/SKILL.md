---
name: requirements
description: Use to set an item's scope, non-goals and testable acceptance criteria.
---
# Write the requirements

Fill `scope`, `non-goals` and `acceptance` on one item.

## Steps

1. Run `rig task show <task> --json`. Read `problem`, `audience` and anything
   already set. If `problem` is empty, suggest the `discover` skill first, but
   carry on if the user wants to.
2. Draft `scope`: what is in. Draft `non-goals`: what is out, including the
   nearby things someone might expect.
3. Draft `acceptance`: one criterion per statement. Each must be something an
   outsider can check without asking the author: it names an observable
   result, and a reader can say yes or no.
4. Test each criterion. If it is vague ("fast", "easy", "works well"), rewrite
   it into a checkable statement and show both versions.
   - Not checkable: "Search is fast."
   - Checkable: "A search over 10,000 items returns in under one second."
5. Show the three fields. After the user agrees, write them:

   ```
   rig task set <task> --field scope='<text>' --field non-goals='<text>' --field acceptance='<criterion>' --field acceptance='<criterion>' --actor product-owner
   ```
   Pass one `--field acceptance=` per criterion.
6. Read `needs` from the item. If it is `tech` or `both`, say that the tech
   lead enriches the item next, and leave the technical fields alone.

This skill sets only `scope`, `non-goals` and `acceptance`. It does not change
the item's state.
