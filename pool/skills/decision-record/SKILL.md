---
name: decision-record
description: Use to write a decision record when a technical choice is not obvious.
---
# Write a decision record

Record one decision, once, where the team keeps them.

## Steps

1. Read `rig-crew-planning.json` at the project root (defaults when absent).
   Take `decisions.dir`, `decisions.format` and `decisions.when`.
   Defaults when a key is absent: `decisions.format` `adr`, `decisions.dir`
   `docs/decisions`, `decisions.when` "the choice is hard to reverse or crosses
   a component boundary".
2. Check `decisions.when` against the choice. Write a record only when it
   holds or the user asks. Otherwise say why not and stop.
3. Write one file in `decisions.dir`, numbered after the last record there
   (`0007-short-title.md`). With format `adr`, use these sections:
   - Title
   - Status (proposed, accepted, superseded by a named record)
   - Context: the forces at play, stated neutrally
   - Decision: what we will do, in full sentences, "We will ..."
   - Consequences: what becomes easier, what becomes harder
   Keep it to a page or two. For any other format, follow that format's
   sections.
4. Link it from the item: set `approach` to a one-line summary and the record's
   path.
   `rig task set <task> --field approach='<summary> (see <path>)' --actor tech-lead`
5. Tell the user the path. Do not commit it unless asked.

Every `rig` write passes `--actor tech-lead`. Do not move the task to another
state.
