---
name: research
description: Use to answer a code, domain or market question for an item and record the cited summary on it.
---
# Research a question

Answer one question and keep the answer with the item.

## Actor

Every `rig` write passes `--actor <role>`, where the role is `product-owner`
or `tech-lead`. Use `RIG_ACTOR` when it is set; otherwise ask the user once per
session which role they are acting as.

## Steps

1. State the question and the item (`<task>`) it serves. Split a mixed
   question into a code part and a domain part.
2. Send a code question (what exists, what calls it, what a change touches) to
   the `explorer` agent. Send a domain or market question (users, competitors,
   standards, prior art) to the `researcher` agent. Run the two in parallel when
   both are needed.
3. Require each answer to cite its sources: `path:line` for code, links for
   the web. Drop a claim you cannot cite, or mark it unverified.
4. Write a short summary that keeps the citations and states what it means
   for the item.
5. Record it:
   `rig note add <task> --kind research --text '<summary>' --actor <role>`.
   Write an apostrophe inside the text as `'\''`.
6. Tell the user what was found and where it was recorded.
