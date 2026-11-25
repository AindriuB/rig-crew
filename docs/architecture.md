# Architecture

The shape of the system: what the pieces are, what talks to what, and where the
boundaries are. `explorer` and `architect` read this before inferring
structure from the filesystem, so keeping it honest saves every session a
search.

**Status: not yet designed.** The sections below are filled in during the
design phase. Until then, the design input is the shaped spec in the rig
repository, `docs/plan/specs/2026-11-25-role-workflows.md` (backlog item
`role-workflows`). Where this file and that spec disagree, this file wins once
it is written.

## What rig-crew is

One repository holding a common pool of agents, skills and commands, one thin
folder per human role (product owner, scrum master, developer, tester), rig
workflow files, and a Go installer that renders a chosen role for a chosen AI
harness:

```
rig-crew install --role developer --harness claude --workflow team
```

It depends on `rig` as an external tool on `PATH`. It does not import rig's
code and does not change it.

## Components

<!-- Filled in at design. -->

## How they talk

<!-- Filled in at design. -->

## Boundaries that must not be crossed

<!-- Filled in at design. -->

## Decisions worth knowing

<!-- One line each, with the date and the alternative that was rejected. -->
