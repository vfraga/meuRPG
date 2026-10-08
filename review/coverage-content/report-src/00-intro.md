# MeuRPG content coverage: what the app runs, what it only lists, what is missing

Inventory of the SRD 5.1 content in `backend/internal/rules/srd51/data/*.json` against what the code does with it.
Scope: spells, equipment, magic items, monsters, races, backgrounds and feats. Class features and the rules chapters are
mapped elsewhere. **No code was changed.** Base: `origin/main` of `vfraga/meuRPG`.

How to read it:

- **State**: `built` (the app computes what the text asks for), `partial` (it computes something real and misses a named
  part), `reminder` (the content is listed or shown as text, or a resource is spent, and nothing is computed) and
  `absent` (it cannot be used at all, or is not in the data).
- **Evidence** is `file:line` on `origin/main`. Every state was checked in code; docs are cited only in the
  "Deliberate?" column, and a doc is never the evidence for a state.
- **Deliberate?** quotes where a doc says the gap is on purpose. "no doc" means none was found.
- **Table impact** is a judgement of how often a table would hit the gap in a first session: `high`, `medium`, `low`.
- Counts come from the scripts in `review/coverage-content/scripts/` and the overlay tests in
  `review/coverage-content/overlay/` (re-runnable: see the README at the end). Raw lists are in the same folder.

The five engine facts that most rows depend on, and the shared causes, are in section 7.
