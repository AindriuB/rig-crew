# Conventions

The rules a reviewer checks a diff against. Each one is checkable in a diff; a
preference that is not belongs in a conversation, not here. **Status: draft,
in design.**

## Go

- Go version from `go.mod`, the same line as rig. Standard library only; a new
  dependency needs a decision recorded in `docs/architecture.md` first.
- `gofmt` clean, `go vet ./...` clean.
- Package layout and boundaries as in `docs/architecture.md`. Only
  `internal/install` writes to disk; only an adapter names a harness.
- Subprocesses (`rig`) run from an argument vector with a deadline. Never a
  shell string.
- Errors are wrapped with `%w` and say what was being done and to which path
  or name. No `panic` outside `main` and tests. No silently ignored error; a
  deliberately ignored one has a one-line comment saying why.
- No package-level mutable state. Pass the `fs.FS`, the clock and the runner
  in, so tests can replace them.

## The command line

- Exit codes as in rig: `0` success, including an identical rerun that
  changes nothing (it says "nothing to change"), `1` refused (a conflict, a
  missing role), `2` usage or internal error.
- Text on stdout by default; `--json` gives a stable shape for scripts. Errors
  and reports go to stderr.
- Every command that writes accepts `--dry-run`, and its output names every
  path it would touch.
- A harness limitation is reported, never dropped silently.

## Content (`pool/`, `roles/`, `workflows/`)

- A file's `name` equals its file or folder name, in kebab-case.
- JSON is two-space indented, no trailing commas. Every role file validates
  against `schema/`.
- Prose lives in Markdown, never in a JSON string.
- Nothing in `pool/`, `roles/` or `workflows/` names a harness or a model ID.
  Harness-specific content lives under `harnesses/<h>/`.
- An agent added for one harness is added for every supported harness in the
  same change, or the drift test fails.
- No path in anyone's home directory, anywhere in content.
- Content is written fresh. A piece that follows an upstream closely cites it
  and its licence in `NOTICE`.

## Tests

- `go test ./...` passes on Linux, macOS and Windows.
- Golden files for every role × harness render live in `testdata/`. Change
  them with `go test ./... -update` and commit the diff with the change that
  caused it.
- Install tests run against a temporary directory with a fake home. Every
  install test also runs the install a second time and asserts no change.
- No network, no real `~`, no sleeps. A test that needs `rig` uses a fake
  runner, except the CI job that runs `rig workflow validate` on `workflows/`.
- A test name says the behaviour: `TestInstall_ModifiedOwnedFileStops`.

## Commits and branches

- Branch `task/<slug>`. Never commit to `main` directly once the first task
  lands.
- Commit subject `<task-id>: <imperative summary>`, under 72 characters. A
  body only when the why is not obvious from the diff.

## Documentation

- Prose in complete sentences. Tables only for tabular content.
- Each role's `README.md` has the five sections the design names: what the
  role is; install; a day with rig; the states and gates it owns; `solo` mode.
- Code comments are short. Only a trap or a decision that would otherwise be
  re-argued earns more than two lines.

## What never goes in a file

Credentials, tokens, keys, real personal data, pasted log dumps, and paths
from a maintainer's machine.
