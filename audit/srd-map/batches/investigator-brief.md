# SRD audit: shared brief for investigators (read-only)

Repository clone: /home/user/meuRPG-p3, branch audit/srd-map (identical to origin/main code). Go code under backend/, Angular under web/.

## Goal
For each entry in your batch, judge what the MeuRPG app really does with that SRD rule. You fix nothing and you file nothing.

## References
- The rule is the SRD text in the repository: backend/internal/rules/srd51/data/*.json (`desc`, `higher_level`) and backend/internal/rules/srd51/effects/corrections.json. Never your memory of the game.
- Table rulings: docs/product/rules.md (sections RN-xx). Where a ruling deliberately differs or leaves the thing to the master, the verdict is `by-design` with the RN id.
- Effects per class: backend/internal/rules/srd51/effects/<class>.json, races.json, spells.json, spell_targets.json, combat_effects.json, choices.json, features_pt.json, traits_pt.json, spells_pt.json (Portuguese texts shown to players).
- Judge by what the code does, never by a field's or a handler's name. Follow an effect to where it is read: backend/internal/rules/ (effect types, handlers, casting), backend/internal/characters/combattraits.go, backend/internal/play/. A feature with no effect row may still be handled in code by its key: grep the key (e.g. `arcane-recovery`, `feature:arcane-recovery`) across backend/ and web/src.
- What a screen shows: grep web/src for the key, the handler or the note mechanism. A `{"type":"note"}` row with a `text_pt` shows that text; check how notes reach the sheet/card/log (find where note effects are rendered) before claiming a reminder is seen. A bare `{"type":"note"}` with no text shows nothing beyond the feature's own description.
- For Go references use `gopls references <file>:<line>:<col>` before grep where helpful (PATH needs /root/go/bin).

## Verdicts (exactly one per row)
- `runs`: the app applies the whole rule by itself, with the numbers the text gives.
- `runs-part`: it applies a part, and a reminder on the sheet, the card or the log tells the table the rest.
- `by-hand-said`: it applies nothing, and a text the player or the master sees says what to do.
- `by-hand-silent`: it applies nothing and nothing on a screen says so beyond the feature's own description, or the description is missing. (Also use this when a part is applied but the rest has no reminder at all; say so in "does".)
- `wrong`: it applies something the text does not say (a wrong number, die, save, target, duration, timing, or an effect that fires when it should not). In "does" say whether the table can correct it by hand (the master's undo or an adjustment) or not.
- `by-design`: a ruling in docs/product/rules.md settles it (put the RN id in "says" or "does").
- `step`: a row that is only a level-up step (ability score improvement, a "subclass feature" marker, a spell-slot row) and the level-up handles it; confirm the level-up does at that level (cite the code).
- `unsettled`: you could not settle it; say in "does" what you would need. Never guess.

## Throwaway tests
When a doubt can be settled by a pure test of the `rules` package (no database), write a throwaway test file named `zz_audit_<your batch id>_test.go` in the package, with test function names prefixed `TestZZAudit<BatchId>`, run only it (`cd backend && go test ./internal/rules/ -run '^TestZZAudit<BatchId>' -count=1`), then DELETE the file. Mark that row "checked": "test". Never run a whole suite, never touch the database. Other investigators work in the same clone at the same time: touch only your own zz file, never edit any other file, never run git commands that change the tree (no checkout, stash, reset, commit).

## Privacy
Both repositories are public. If you find that something hidden from players (master notes, hidden monster stats, map secrets) can reach a player, do NOT write the details into your output file; write a row with verdict `unsettled`, "does": "PRIVACY — see report", and describe it only in your final reply.

## Output
Write a JSON array to /home/user/srd-audit/out/<your batch id>.json, one object per entry, with exactly these keys:
{"key", "name", "class" (for features: e.g. "wizard" or "wizard/evocation") or "list" (for traits: the race key; for spells: the class lists, e.g. "wizard,druid"), "level" (number; traits: 0), "verdict", "says" (the rule in one line, your own words), "does" (what the engine does, one line), "seen" (what a screen tells the table, or "nothing"), "code" (file and function you read, e.g. "backend/internal/rules/casting.go castSpell"), "fix" (the smallest change that would make it `runs` or at least `by-hand-said`: an effect row with its type, a handler, a reminder text; no size estimate; "" for runs/step/by-design), "checked" ("test" if a throwaway test settled it, else "")}
English only, one line per field, SRD names only. Validate the file with `python3 -m json.tool` before you finish.

Your final reply: the count of rows by verdict, every `wrong` row's key with one line of evidence (file:line), anything privacy-related, and nothing else. Keep it under 40 lines.
