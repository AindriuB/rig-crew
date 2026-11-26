---
name: test-driven-development
description: Use when implementing any behaviour so that each acceptance criterion starts as a failing test.
---
# Test-driven development

Write the test, watch it fail, make it pass, then tidy. A test you never saw
fail proves nothing.

## The loop

1. Take one acceptance criterion. Name a test after the behaviour it checks.
2. Write the smallest test that fails for the right reason. Run only that test
   and read the failure. A test that errors on a typo has not failed yet.
3. Write the least code that makes it pass. Do not build ahead of the test.
4. Run the narrow check again. Then tidy duplication while it stays green.
5. Commit, then take the next criterion.

## Rules

- No production code without a failing test first. If you wrote code first,
  delete it and start from the test.
- One behaviour per test. If the name needs "and", split it.
- Run the narrowest check that answers your question, not the whole suite.
- A bug fix starts with a test that reproduces the bug.
- Do not weaken or delete a test to make it pass. If the test is wrong, say why.

## When a test cannot fail

Some criteria are about content or configuration. Write the check that reads
the file and asserts the fact, and run it. If no automated check is possible,
say so in your report and name how you verified it by hand.
