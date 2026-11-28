# packs

One folder per pack, holding a `pack.json`, the steering files it names and
optional pack-local skills under `skills/<name>/SKILL.md`. A pack lists the
agents, skills and workflows it uses and the `rig` commands its agents may run.
A role pack carries an `actor`; a capability pack carries none. `requires` names
capability packs a pack builds on.
