## Appendix: how this was made and how to repeat it

Everything lives in `review/coverage-content/`. No file of the application was changed.

| What | Where |
| --- | --- |
| Overlay tests (Go, run with `go test -overlay`, kept outside the packages) | `overlay/zz_*_dump_test.go`; `overlay/run.sh <name> <TestName> [package]` writes `<name>.json` |
| Spells: what a cast reads for each of the 319 | `spells-machine.json` (`overlay/zz_spells_dump_test.go`, run with `overlay/run-spells.sh`) |
| Spells: tags from the description (haiku sweep, 8 batches of 40, two agents at a time) | prompt `scripts/spell-tag-prompt.md`, inputs `raw/in/`, outputs `raw/spell-tags-b*.jsonl` |
| Spells: state, impact, groups | `scripts/spell-state.py` (rules and hand overrides in `scripts/spell-overrides.json`), `scripts/spell-groups.py`; results `spells-states.json`, `spells-states.csv`, `spells-groups.md` |
| Spells: sample of the content side of a cast | `spells-cast-sample.txt` (`overlay/zz_sample_dump_test.go`) |
| Equipment | `equip.json` (`overlay/zz_equip_dump_test.go`: 37 weapons and 13 armour pieces through `Derive`) |
| Races | `races.json` (`overlay/zz_races_dump_test.go`) |
| Magic items | `scripts/magic-counts.py`, `magic-counts.json`, `magic-items-by-effect.json` |
| Monsters | `scripts/monster-counts.py`, `monsters-counts.json`, `monsters-features.json`, `npc-sheets.json` (`overlay/zz_npc_dump_test.go`: `npcSheetFromCreature` over all 334) |
| Report | `report-src/*.md` assembled by `scripts/build-report.py` into `../coverage-content.md` |

To repeat: `cd backend && ../review/coverage-content/overlay/run-spells.sh`, then `run.sh races TestCoverageDumpRaces`, `run.sh equip TestCoverageDumpEquip`,
`run.sh sample TestCoverageSampleSpells`, `run.sh npc TestCoverageDumpNpcSheets characters`, then the Python scripts, then `build-report.py`
(the `run.sh` files write to `review/coverage-content/<name>.json`; the sample output was renamed to `spells-cast-sample.txt`).

**Limits to know.**

- The play tests that cast through the RPC (`play/combat_spells_test.go` and others) need Postgres, which this session did not have, so the
  engine side of a cast was verified by reading the code at the lines cited, and the content side by the overlay tests above. The sample file shows what the
  cast would roll for 22 spell and slot combinations.
- The haiku tags give the *text* of each spell; the *state* of every spell was decided by me from the machine view plus the rules written in
  `scripts/spell-state.py` and fixed by 16 hand overrides. The impact column of the spell table is a rule of thumb (level, kind, a list of staple spells in the script),
  not a measurement.
- Magic item rows 3.1 that count "text mentions" use keywords over the SRD text and are approximate.
- "Deliberate?" quotes a doc line only when the doc says the gap is on purpose or out of scope; a doc was never used as evidence of a state.
