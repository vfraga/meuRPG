# Spell zones

The areas that Spirit Guardians, Moonbeam, Cloudkill, Web, Grease and Entangle keep on the map (RN-37).
The code is `zones_*.go` in `backend/internal/play`; the rules are in [Rules](../../product/rules.md#rn-37-areas-that-spells-keep-on-the-map).

## The model

A zone is a row of `spell_zones` ([data](../../data/combat.md#spell_zones)): the combat, the caster (`owner_id`), the spell, the squares it covers now, and what a trigger needs (the save and the damage as the cast made them).
It is not a lasting effect (RN-22).
A lasting effect belongs to one target and has its own clock; a zone has no single target and carries geometry.
Its life has three points, each a single call: the cast makes it (`createZone`), the end of a concentration deletes it (`endZonesOf`, called by `endConcentrationEffects`, so every way a concentration ends reaches it, `stopConcentrating` and `endConcentrationAtZero` included), and the start of the caster's turn deletes it when its round is out (`endZonesOutOfTime`).
The foreign keys delete it with the combat or the caster.

`zoneDefs` (`zones_catalog.go`) is the table of the six spells: kind (damage, restrain or prone), shape and size, how the cast places it (`AreaPlacement`), how it moves, the save, when it catches (`onEnter`, `onStart`, `onCast`, `onTurnEnd`), the effect a restraint puts on a creature (`restrainKey`), whether it spares the caster's side, and whether the cast still hits the creatures inside.
Another spell with a lingering area is another row, plus its entry in the catalogue of lasting effects if it also puts an effect on its targets.
`placementOf`, `areaOf` and the area fields of the turn options read the row first, so the preview, the cast and the options lay the area out as the zone does.
A cube is centred on its point (`Area.FromPoint`, an even side puts the extra square on the larger side).

## The table's own spell

A table spell with `TableSpell.zone` (`rules/v1/table_content.proto`: a trigger and `follows_caster`) keeps its area through the same code. `rules` checks it in `checkSpellZone` (`overlay_spell.go`: refusals at `zone`, `zone.trigger` and `zone.follows_caster`, one sentence each) and keeps it as `Content.SpellZone`; `characters` fills `link.Spell.Zone`; `zoneDefOfSpell` returns the SRD row or `zoneDefOfRule`, one `zoneDef` value either way. A table zone's definition is kept in the row (`spell_zones.def`, JSON, migration 00268) so that it outlives an edit of the spell; an SRD zone keeps none, its row of `zoneDefs` is the truth. The trigger is `onEnter` and `onStart` of the definition, read by `zone.asks`. A table zone has the shape, size, save, damage and time of the spell; with no save the damage is the offered roll (`catchWithoutSave`).

## The cast

A zone is laid only when the request chooses a point (`origin`): `placeArea` then sets `Spell.ZoneLaid`, which makes `placementOf` and `areaOf` read the zone's own layout (a cube at a distance, Spirit Guardians around the caster, whose point only says the cast lays the zone). Without a point the cast is the spell with its targets and effects, and no zone. Moonbeam and Cloudkill are placed at a point by their own data, so without one they are refused. `PreviewSpellArea` and the drawing of the cast read the same flag.

`castFlow.affect` hands a placed cast of a zone spell to `affectZone`.
Moonbeam, Spirit Guardians and Cloudkill hit nobody at the cast.
Grease and Entangle catch the creatures standing in the area once the zone exists (`catchWhoIsInside`, reason `zone_cast`).
Web runs `hitTarget` and `applyLastingSpell`, so the creatures inside get the web's effect.
`createZone` keeps `plan.origin` and `plan.squares` (what `planArea` worked out, walls included), the caster's DC, the damage at the cast's slot level and the end round (`SpellEffectRounds`).
The cast's event records every creature a catch at the cast caught, whatever its save: `made.Hits` holds one `castHit` for each, with the save of an NPC, and the conditions a player's character had before for Grease.
The restraint of an NPC is in the cast's `Lasting.Effects`.
So the hidden-creature reveal and the cast's line treat a creature that passed like one that failed.
The answer of a player's character (`settleZoneSave`) puts the fall or the restraint on the creature after the cast's event is written.
The undo of the cast puts it all back: the conditions from the recorded `castHit`, the restraint from the effects of the cast's group (`zoneEffectsBack`), and it closes the questions the zone opened (`closeCastZoneWindows`).
The `spell_zone_triggered` lines of the cast and the `lasting_effect_saved` line of an answer to it never close the undo chain (`chainCastZoneCatch`).
The cast's event keeps its place (`placed`, `area_col`, `area_row`).
The cast's `cast_id` is on the zone.
The undo of the cast deletes the zone with that id (`castEffectsBack`).
Without a grid (`isTheatre`) a spell is not placed, so there is no zone.

## When a zone looks

| Where | What it does |
| --- | --- |
| `zonesAfterSteps` (`zones_hooks.go`) | Called with the creatures that changed square: `moveInTx` (the mover and the creature it dragged; this also covers the master's move and a placement), `castFlow.teleport` (Misty Step's landing, `combat_teleport.go`), `pusher.enterZones` (a push by a cast or by the Open Hand technique) and the shove outcome (`shovePushes`). A step has where the creature stood and the squares it went through (the straight line for a walk, only the end for a jump, a push or a forced move). A zone catches a step that comes into it: one square inside whose predecessor is outside |
| `zonesAtTurnStart` | Called at the end of `startOfTurnEffects`: ends the zones whose round is out, moves the zones that drift, then catches every creature that starts its turn inside |
| `zonesAtTurnEnd` | Called when a member's part of the turn ends (`closeTurnPart`), right before the saves of the effects open: both questions are then open together, in no order. It catches the creature if it stands in a zone that asks at the end of the turn (Grease). Its hit mark is `"<round>:<leader>:end"`, so the entry and the end of one turn each ask once |
| `followCaster` | Part of `zonesAfterSteps`: when the creature that moved owns a zone centred on it, the zone is placed on its square and the creatures the new area covers and the old one did not are caught as if they had come in |

The undo of a move, of a push or of a shove puts the creature back and re-evaluates nothing.

## Catching a creature

`catchZone` (`zones_trigger.go`) is the whole of it, in this order:

1. The checks: the creature is on the map and standing; it is not spared (Spirit Guardians leaves the caster and the caster's `side` alone); the zone has not caught it on this turn (`hit_marks`, below). A restraining zone also skips a creature the spell holds or asks a save of already (`heldByZone`). The cast's catch has its own mark (`"<mark>:cast"`), so a creature caught at the cast is asked again if it leaves and comes back on that turn.
2. The turn is written first: `hit_marks[creature] = "<round>:<first combatant of the turn>"`. At a turn start the mark is built from the turn that starts; in the middle of a change it is built from the encounter's current turn.
3. The saving throw: `zoneSave` calls `spellSave` with a `link.Spell` made of the zone's save, so it is the cast's save (modes, auto-fail, Lucky, effect dice).
4. The effect: `dealZoneDamage` rolls the dice and halves them on a pass, applies resistance, and inserts an effect pending damage (`InsertEffectPendingDamage`, `effect_source_key` the spell), landed by `landDamage` (an NPC or creature takes it, a character's waits for the master); `afterDamage` then opens the concentration save as for any damage. A restraining zone instead adds its `restrainKey` effect on a failure (`restrainInZone`: `effect:web-restrained`, `effect:entangle-restrained`; both are broken by a Strength check), and a prone zone gives `condition:prone` (`knockProne`), which lasts until the creature stands up.
5. One event, `spell_zone_triggered`, with a `lastingEvent` (change `saved` or `failed`, reason `zone_enter`, `zone_start`, `zone_cast` or `zone_end`, the roll, the DC and the damage).

The events a zone leaves (`spell_zone_triggered`, `spell_zone_moved`, and the `lasting_effect_ended` of a zone that timed out) are held on the change (`combatTx.zoneOut`) and written by `record` right after the change's own event, so the log reads the move and then what it brought.
They are registered as effect lines in `logHandlers` and `effectLogKinds`; `COMBAT_LOG_EFFECT_CHANGE_ZONE_MOVED` is the one new change.
None of the three is undoable (`undoableKinds`), so a move that caught someone cannot be undone as a move: the damage is a pending damage, discarded and undone as one.

## Moving a zone

- Spirit Guardians follows its caster (`followCaster`).
- Cloudkill drifts at the start of its caster's turn (`driftZones`): `planPush` from the caster's square, `driftFt` (10) long, so it stops at a wall; a cloud on the caster's own square stays.
- Moonbeam is moved by `MoveSpellZone` (`zones_move.go`), a keyed `write` of kind `spell_zone_moved`. A player must be the caster's player, on the caster's turn (`mustActNow`), with the action free (it is spent), and within `moveRangeFt` (60) of the zone's origin (`TARGET_OUT_OF_REACH` with `missing_ft`); the square must be on the map and not a wall. The master moves any zone, with none of that. A zone the caller does not read is `not_found`, as one that does not exist. The creatures the new area reaches are caught by `relocate`, the same step that follows and drifts.

## The save question, the end and the other effects

- **The question.** A zone with a save that catches a player's character (`Kind == kindPlayer`) opens an `EFFECT_SAVE` reaction window with `windowTrigger.Zone` (`askZoneSave`, `zones_save.go`); the answer is `RollEffectSave`, which loads the zone in `loadZoneSave` and settles it in `settleZoneSave` (damage halved on a pass, or the web's restraint) through the same `applyZone` an NPC's catch uses. The prompt (`zoneSavePrompt`) carries `zone_id`, `zone_reason`, the damage type and the half, and the DC only for the master, the caster's player and, with the table rule `show_save_dc` on, the player whose creature rolls the save (`saveDCFor`). The windows of one creature on one turn share a group (`zoneWindowGroup`), so the existing order rule applies, and `openOne` lists a player the first only. A zone that ends closes its windows (`closeZoneWindows`).
- **No save.** A zone whose row has no save ability inserts a pending damage awaiting its roll with the caster as attacker (`catchWithoutSave`), rolled and applied by the paths of any damage.
- **Ending.** `endZonesOf` writes an `ended` line for each zone (reason `concentration` or `concentration_replaced`), the clock writes `duration`, `EndSpellZone` (master only) writes `ended_by_master`. They are written at once, not held, because a concentration also ends outside the changes that flush the zone lines.
- **Speed and terrain.** A zone with `slows` adds `effect:spirit-guardians-slowed` (speed 50%, until the end of the creature's turn) when it catches a creature. A zone with `difficult` is added to the Difficult layer of `terrainOf` (`withZoneTerrain`, a copy, so the map's own layer is untouched).

## The view and the privacy

`Encounter.zones` is built by `encounterData.zonesView` from the zones read with the combat (`loadEncounter`).
A player gets the zones of casters they `sees` (not hidden, not in the dark of a fog map); the master gets all, with `SPELL_ZONE_VISIBILITY_MASTER_ONLY` on the zones of hidden casters.
`rounds_left` is the round the zone ends at, minus the current round, 0 for a zone with no round count.
The hint of a zone that moved goes to the master alone when the caster is hidden (`publishZoneMoved`).
The event of a catch is `Secret` for a hidden caster or a hidden creature, and its actor is the caster and its target the creature, so the log's own rule hides it from a player who does not see either (RN-10); the DC is the master's, and the target's player's with `show_save_dc` on (`effectLogRoll`).
A player gets the numbers (`save_*`, damage, `notes`, `moves_per_turn_ft`, `rounds_left`) only for a zone of their own character; for another they get `details_hidden` and the name, shape, squares, concentration and the occupants they see. The squares of a visible caster's zone reach every player, also on a fog map where they have not explored them: the shape of a spell is not a secret the fog keeps.

## Tests

`zones_grease_test.go` (Grease: the shape, the cast, entering, the end of the turn, the end of the zone and the undo; Entangle: the shape, the cast only, the end with the concentration and the undo) and `zones_test.go` (Spirit Guardians, Moonbeam, Cloudkill, Web, the push, the forced move, the undo, the player's character, the permissions and the retry) and `zones_hidden_test.go` (the viewer matrix for a hidden caster, a revealed one and a hidden creature, and the stream hint); `internal/leaktest` classifies `MoveSpellZone`.
The grid cube is in `rules/grid/area_test.go`.

## The web

The zones ride on `Encounter.zones`: a change arrives as the usual `encounter_changed` hint (the master alone when the caster is hidden) and the combat is read again, so a refresh carries them too and nothing else is wired.
`core/combat/zones.ts` turns each `SpellZone` into a `ZoneView` (label, detail pieces, rounds left, concentration, occupants, the "Só você vê" flag, `canMove`, `canEnd`); a zone with `details_hidden` reads name, area, caster, concentration and occupants only, even if a field slipped through, and the client never fills what the server left out.
`zone-move.ts` holds the move range copy (`ZONE_MOVE_RANGE_FT`, the server's `moveRangeFt`) and `zoneMovePreview`, which draws the open-floor squares with `sphereSquares`; no wall cuts the preview, the server cuts the area on commit.
`MoveSpellZone` and `EndSpellZone` are in `CombatCastCalls`, one idempotency key per request kept across retries and renewed after success; `zone-errors.ts` words their refusals.
The log lines for `ZONE_MOVED`, the `zone_enter`, `zone_start`, `zone_cast` and `zone_end` catches and the end reasons are in `combat-log-effects.ts`.

### The map, the panel and the end

- **Map layer.** `shared/combat-map/zone-layer` (`app-zone-layer`) is projected into `app-combat-map` by `app-combat-map-card`, the way the area overlay is, so it follows the map's zoom and crop and sits under the tokens. It draws each `ZoneView.squares` with `cellsPath` and `outlinePath`; `core/combat/zone-pattern.ts` gives each `spell_key` a pattern (each of the six catalog spells a fixed one; a table spell a hash of the key) and `zoneLegend` the entries the card lists. The layer and its labels are `aria-hidden`; the panel is the accessible version. The move page's map does not draw zones.
- **Panel.** `pages/live-session/combat/zones-panel` (`app-zones-panel`) lists the zones: the master's column and the player's side column. Its live region (`zoneNotice`) compares the ids of the list with the last one it read. `details_hidden` cards render the name, the area and the caster only.
- **End.** The panel asks in place and emits `end`; `ZoneFlow` (`zone-flow.ts`, built in `combat-view-parts.ts`) calls `endSpellZone` under the screen's one-call-at-a-time rule (`calls.busy`) and words a refusal with `zoneEndError`.
- **Move.** "Mover" emits `move`; `ZoneFlow.move` (only for a zone whose view says `canMove`) sets `moving` and asks `CombatSheets.moveZone` to open `ZoneMoveSheet` (`pages/live-session/combat/zone-move`), a bottom sheet on a phone and a dialog from a tablet up (`openSheet`); a scene without a battle map opens nothing and says so in the panel. The sheet holds the picked square and wraps `app-zone-move-map` (`shared/area-picker`): `app-combat-map` in `pickSquares` mode (click, arrows from the zone's origin, Enter) with two `app-area-overlay` layers, the current footprint dimmed and the new one from `zoneMovePreview`. Past `ZONE_MOVE_RANGE_FT` a player's square is drawn refused with the `TARGET_OUT_OF_REACH` wording (`zoneOutOfReach`); the master has none. "Mover a zona" calls `moveSpellZone` once (the client's keyed helper), applies the encounter the server answers, closes on success and says `zoneMoveError` in place on a refusal; `moving` returns to `null` when the sheet closes.
- **Save prompt.** An `EFFECT_SAVE` window whose prompt has `zone_id` is the player's `EffectSaveSheet` (the same `RollEffectSave` path, dice modes and held-roll question as any effect save). `core/effects/zone-save.ts` words it: the title is the zone and its caster (found in `encounter.zones` by `zone_id`), the subtitle "Magia que ficou no mapa" and the reason (`zone_enter` "Você entrou na zona.", `zone_start` "A zona atingiu você no começo do seu turno."), then the ability, the failure from `damage_type_name_pt` and `half_on_save`, the bonus and the spell. The DC is written (", CD 14", and "contra CD 14" after the roll) only when the server sent `dc`; without it there is no number and no placeholder. A zone that restrains adds the static "Soltar-se gasta uma ação." The master's queue words the same window with the reason of the creature. The damage a zone with no save owes is a `PendingDamage` with no zone fields, so its card says "Zona de magia: <spell>" when the caster has a zone of that spell (`zoneLabel`).
