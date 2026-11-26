---
name: verification-before-completion
description: Use before claiming any work is done, fixed or passing, to check the claim against fresh evidence.
---
# Verification before completion

Do not say work is done, fixed or passing until you have run the check that
proves it, in this turn, and read its output.

## The gate

1. Name the claim: "the tests pass", "the bug is fixed", "criterion 3 is met".
2. Name the command that would prove it. Run it in full, now.
3. Read the output and the exit status. Count passes and failures.
4. Say what you saw. If it does not support the claim, say that instead.

## Rules

- An earlier run, another agent's report or "it should work" is not evidence.
- Check each acceptance criterion by itself, not the task as a whole.
- A check that ran zero tests proves nothing. Confirm the test you meant to run was collected.
- After a fix, rerun the check that failed, not a nearby one.
- Check that your changes are committed and only the files you own were touched.
- If you could not run a check, say which one and why. Never report it as passed.

Words such as "should", "probably" and "seems to" mean the claim is not yet verified.
