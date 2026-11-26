---
name: systematic-debugging
description: Use when a test, build or behaviour fails and the cause is not yet known, before changing any code.
---
# Systematic debugging

Find the cause before you change anything. A guess that happens to pass hides
the real fault.

## Steps

1. Read the whole error, including the first line and the line it points to.
   Note what it says, not what you expect it to say.
2. Reproduce it with the narrowest command that fails, and make it fail every time.
3. Check what changed: the last commits, the inputs, the environment.
4. Trace backwards from the symptom to where the bad value or state first
   appears. Add a temporary print or assertion at each hop, then remove it.
5. Compare with a working case nearby and list every difference.
6. State one hypothesis in a sentence: "X fails because Y". Test it by changing
   one thing. If it is wrong, drop it and form another; do not stack fixes.
7. Write a test that fails for the cause, fix the cause, and run the narrow check.

## Stop signs

- Three fixes tried and none worked: stop. The model of the problem is wrong, so go back to step 1.
- The fix needs a change outside your task's file set: stop and report it as blocked.
- You cannot reproduce it: say so and record what you tried, rather than guessing.

Record a conclusion that later readers need with
`rig note add <task> --kind decision --text '<what and why>' --actor developer`.
