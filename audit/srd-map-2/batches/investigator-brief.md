# SRD audit, second pass: shared brief for investigators (read-only)

Repository clone: /home/user/meuRPG-audit2 (branch audit/srd-map-2, identical to origin/main code). Go under backend/, Angular under web/src/, protos under proto/.

## Goal
For each entry in your batch, judge what the MeuRPG app really does with that SRD rule. You fix nothing, file nothing, and edit no file under backend/, web/, proto/ or docs/.

## References
- The rule is the SRD text in the repository: backend/internal/rules/srd51/data/*.json and backend/internal/rules/srd51/effects/corrections.json. Never your memory of the game. If the repository holds no SRD text for an entry the batch names, say so in "says" ("no SRD text in the repository") and judge what the app does anyway; verdict `unsettled` only if you truly cannot tell what the app does.
- Hand-written effects: backend/internal/rules/srd51/effects/*.json (e.g. ammunition.json, consumables.json, potions.json, item_charges.json, magic_item_values.json, magic_items_pt.json, standard_actions.json, combat_effects.json, damage_resistances.json, lights.json, traps.json, treasure.json, feats.json, backgrounds.json).
- Table rulings: docs/product/rules.md (RN-xx; Portuguese twin docs/pt-BR/produto/regras.md). Where a ruling deliberately differs or leaves the thing to the master, the verdict is `by-design` with the RN id.
- Judge by what the code does on this checkout, never by a field's or handler's name. Follow data to where it is read: backend/internal/rules/, backend/internal/characters/, backend/internal/play/, web/src/app. Grep the key (e.g. `weapon-property:finesse`, `finesse`) across backend/ and web/src.
- What a screen shows: grep web/src for the key or the text mechanism; a reminder counts only if you find where it is rendered.
- For Go references use `gopls references <file>:<line>:<col>` (PATH needs /root/go/bin) before grep where helpful.

## Verdicts (exactly one per row)
- `runs`: the app applies the whole rule by itself, with the numbers the text gives.
- `runs-part`: it applies a part, and a reminder on the sheet, the card or the log tells the table the rest.
- `by-hand-said`: it applies nothing, and a text the player or the master sees says what to do.
- `by-hand-silent`: it applies nothing (or a part) and nothing on a screen says the rest beyond the entry's own description, or the description is missing.
- `wrong`: it applies something the text does not say (a wrong number, die, type, range, property, timing, or an effect that fires when it should not). In "does" say whether the table can correct it by hand (the master's undo or an adjustment) or not.
- `by-design`: a ruling in docs/product/rules.md settles it (put the RN id in "says" or "does").
- `unsettled`: you could not settle it; say in "does" what you would need. Never guess.

## Throwaway tests
When a doubt can be settled by a pure test of the `rules` package (no database), write a throwaway test file `zz_audit2_<batch id>_test.go` in backend/internal/rules/, test names prefixed `TestZZAudit2<BatchId>`, run only it (`cd /home/user/meuRPG-audit2/backend && go test ./internal/rules/ -run '^TestZZAudit2<BatchId>' -count=1`), then DELETE the file. Mark the row "checked": "test". Never run a whole suite, never touch a database. Other investigators share the clone: touch only your own zz file, never edit another file, never run git commands that change the tree.

## Privacy
Both repositories are public. If you find that something hidden from players (master notes, hidden monster stats or names, map secrets) can reach a player, do NOT write the details into your output file; write the row with verdict `unsettled`, "does": "PRIVACY — see report", and describe it only in your final reply.

## Output
Write a JSON array to /home/user/srd-audit2/out/<batch id>.json, one object per row, with exactly these keys (plus any extra keys your batch prompt names):
{"key", "name", "list" (the group: e.g. "weapon-property", "weapon", "armor", "tool", "gear", "rule", "skill", "check", "background", "feat", "language", "proficiency"), "verdict", "says" (the rule in one line, your own words), "does" (what the app does, one line), "seen" (what a screen tells the table, or "nothing"), "code" (file and function you read), "fix" (the smallest change that would make it `runs` or at least `by-hand-said`; "" for runs/by-design), "checked" ("test" or "")}
English only, one line per field, SRD names only. Validate the file with `python3 -m json.tool` before you finish.

Final reply: the count of rows by verdict, every `wrong` row's key with one line of evidence (file:line), anything privacy-related, the rules your batch found that neither the code nor docs/product/rules.md mentions ("Not claimed anywhere"), and nothing else. Under 40 lines.
