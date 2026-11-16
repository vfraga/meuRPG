# General-rules batches: addendum to brief.md

There is no SRD text in data/ for these rules. Use docs/product/rules.md and docs/architecture/rules-combat.md (and the docs/architecture/play/*.md pages) as the list of what the app claims, then judge each rule by the code, as in brief.md. Where you state the SRD rule, state it as the SRD 5.1 gives it, in your own words; if you are not sure of the SRD wording, say so in "says" and keep the verdict to what the code does.

One row per SRD rule (split a topic into the rules a table would notice: e.g. for grappling: the contest, the grappled condition, escaping, moving a grappled creature). Keys: "rule:<area>-<short-name>", e.g. "rule:combat-opportunity-attack". "list": the area ("combat", "conditions", "resting", "spellcasting", "vision", "environment"). "level": 0.

For a rule that neither doc mentions, still judge it by the code, and add `"claimed": false` to the row (other rows `"claimed": true`). These feed the README section "Not claimed anywhere", which the orchestrator checks against the SRD text.

Throwaway pure tests of the rules package (backend/internal/rules, rules/combat, rules/grid, rules/vision...) are welcome; name the file zz_audit_<batch>_test.go in the package you test and delete it.
