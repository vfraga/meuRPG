# Spell zones

The areas that Spirit Guardians, Moonbeam, Cloudkill, Web, Grease, Entangle, Fog Cloud, Darkness, Stinking Cloud, Silence, Spike Growth and Wall of Fire keep on the map (RN-37).
The code is `zones_*.go` in `backend/internal/play`; the rules are in [Rules](../../product/rules.md#rn-37-areas-that-spells-keep-on-the-map).

## The model

A zone is a row of `spell_zones` ([data](../../data/combat.md#spell_zones)): the combat, the caster (`owner_id`), the spell, the squares it covers now, and what a trigger needs (the save and the damage as the cast made them).
It is not a lasting effect (RN-22).
A lasting effect belongs to one target and has its own clock; a zone has no single target and carries geometry.
Its life has three points, each a single call: the cast makes it (`createZone`), the end of a concentration deletes it (`endZonesOf`, called by `endConcentrationEffects`, so every way a concentration ends reaches it, `stopConcentrating` and `endConcentrationAtZero` included), and the start of the caster's turn deletes it when its round is out (`endZonesOutOfTime`).
The foreign keys delete it with the combat or the caster.

`zoneDefs` (`zones_catalog.go`) is the table of the twelve spells: kind (damage, restrain, prone, open, retch, spikes or wall), shape and size (`growsFt` is what a slot above the spell's level adds to the radius, `sizeAt`), how the cast places it (`AreaPlacement`), how it moves, the save, when it catches (`onEnter`, `onStart`, `onCast`, `onTurnEnd`), the effect a restraint puts on a creature (`restrainKey`), whether it spares the caster's side, whether the cast still hits the creatures inside, and what it does to sight (`sight`: none, cloud or dark).
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
Fog Cloud, Darkness and Silence (`zoneOpen`) catch nobody.
Stinking Cloud (`zoneRetch`) catches at the start of a turn only.
Spike Growth (`zoneSpikes`) catches nobody at the cast.
Wall of Fire (`zoneWall`) catches the creatures inside the wall at the cast.
Web runs `hitTarget` and `applyLastingSpell`, so the creatures inside get the web's effect.
`createZone` keeps `plan.origin` and `plan.squares` (what `planArea` worked out, walls included), the caster's DC, the damage at the cast's slot level and the end round (`SpellEffectRounds`).
The cast's event records every creature a catch at the cast caught, whatever its save: `made.Hits` holds one `castHit` for each, with the save of an NPC the server rolled at once (none for a creature whose save waits), and the conditions a player's character had before for Grease.
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
| `zonesAfterSteps` (`zones_hooks.go`) | Called with the creatures that changed square: `moveInTx` (the mover and the creature it dragged; this also covers the master's move and a placement), `castFlow.teleport` (Misty Step's landing, `combat_teleport.go`), `pusher.enterZones` (a push by a cast or by the Open Hand technique) and the shove outcome (`shovePushes`). A step has where the creature stood and the squares it went through (the straight line for a walk, only the end for a jump or a placement; a walk, a push and a shove also say they travelled, and Spike Growth counts the squares of the straight line from where the creature stood to where it landed). A zone catches a step that comes into it: one square inside whose predecessor is outside |
| `zonesAtTurnStart` | Called at the end of `startOfTurnEffects`: ends the zones whose round is out, moves the zones that drift, then catches every creature that starts its turn inside |
| `zonesAtTurnEnd` | Called when a member's part of the turn ends (`closeTurnPart`), right before the saves of the effects open: both questions are then open together, in no order. It catches the creature if it stands in a zone that asks at the end of the turn: in Grease, or in reach of a Wall of Fire (`endsOn`). Its hit mark is `"<round>:<leader>:end"`, so the entry and the end of one turn each ask once |
| `followCaster` | Part of `zonesAfterSteps`: when the creature that moved owns a zone centred on it, the zone is placed on its square and the creatures the new area covers and the old one did not are caught as if they had come in |

The undo of a move, of a push or of a shove puts the creature back and re-evaluates nothing.

## Catching a creature

`catchZone` (`zones_trigger.go`) is the whole of it, in this order:

1. The checks: the creature is on the map and standing; it is not spared (Spirit Guardians leaves the caster and the caster's `side` alone); the zone has not caught it on this turn (`hit_marks`, below). A restraining zone also skips a creature the spell holds or asks a save of already (`heldByZone`). The cast's catch has its own mark (`"<mark>:cast"`), so a creature caught at the cast is asked again if it leaves and comes back on that turn.
2. The turn is written first: `hit_marks[creature] = "<round>:<first combatant of the turn>"`. At a turn start the mark is built from the turn that starts; in the middle of a change it is built from the encounter's current turn.
3. The saving throw: `zoneSave` calls `spellSave` with a `link.Spell` made of the zone's save, so it is the cast's save (modes, auto-fail, Lucky, effect dice).
4. The effect: `dealZoneDamage` rolls the dice and halves them on a pass, applies the target's resistances, immunities and vulnerabilities through `settleDamage` (the reader of an attack's damage, `settleLanding`: the target's own, the lasting effects' and Silence's thunder), and inserts an effect pending damage (`InsertEffectPendingDamage`, `effect_source_key` the spell), with the steps kept (`pending_damages.steps`) when any applied, so the master reads them in the pending damage (a hidden effect's step shows its name to the master alone, RN-10); landed by `landDamage` (an NPC or creature takes it, a character's waits for the master); `afterDamage` then opens the concentration save as for any damage. A restraining zone instead adds its `restrainKey` effect on a failure (`restrainInZone`: `effect:web-restrained`, `effect:entangle-restrained`; both are broken by a Strength check), and a prone zone gives `condition:prone` (`knockProne`), which lasts until the creature stands up.
5. One event, `spell_zone_triggered`, with a `lastingEvent` (change `saved` or `failed`, reason `zone_enter`, `zone_start`, `zone_cast`, `zone_end` or `zone_move`, the roll, the DC and the damage).

The events a zone leaves (`spell_zone_triggered`, `spell_zone_moved`, and the `lasting_effect_ended` of a zone that timed out) are held on the change (`combatTx.zoneOut`) and written by `record` right after the change's own event, so the log reads the move and then what it brought.
They are registered as effect lines in `logHandlers` and `effectLogKinds`; `COMBAT_LOG_EFFECT_CHANGE_ZONE_MOVED` is the change of a zone that moved.
None of the three is undoable (`undoableKinds`), so a move that caught someone cannot be undone as a move: the damage is a pending damage, discarded and undone as one.

## Moving a zone

- Spirit Guardians follows its caster (`followCaster`).
- Cloudkill drifts at the start of its caster's turn (`driftZones`): `planPush` from the caster's square, `driftFt` (10) long, so it stops at a wall; a cloud on the caster's own square stays.
- Moonbeam is moved by `MoveSpellZone` (`zones_move.go`), a keyed `write` of kind `spell_zone_moved`. A player must be the caster's player, on the caster's turn (`mustActNow`), with the action free (it is spent), and within `moveRangeFt` (60) of the zone's origin (`TARGET_OUT_OF_REACH` with `missing_ft`); the square must be on the map and not a wall. The master moves any zone but a wall, with none of that. A wall is not moved (`failed_precondition`): the master ends it and the spell is cast again. A zone the caller does not read is `not_found`, as one that does not exist. The creatures the new area reaches are caught by `relocate`, the same step that follows and drifts.

## Zones that block sight

Fog Cloud, Stinking Cloud and Wall of Fire are clouds (`sightCloud`); Darkness is magical darkness (`sightDark`).
The squares of these zones reach the fog through `CombatPositions` (`mapseam.go`, `sightZonesOn`): `Clouds` and `MagicDark`.
`vision.Scene.Clouds` and `MagicDark` are layers of the map's grid; the rules are in [Fog of war](../maps/fog.md#zones-that-block-sight).
The scene's cache key (`litKeyOf`) hashes both lists, so a zone with other squares is another scene.
No RPC or event of its own carries it: any change to the squares or the existence of a sight zone tells the fog and the master's light warning.
A hidden caster's cloud or wall still blocks sight.

## Stinking Cloud

A creature that starts its turn with its square in the cloud makes a Constitution save, and on a failure its action for the turn is spent (`retchOnTheTurn`).
Creatures occupy one square, so "wholly inside" means the creature's square is in the cloud.
A creature immune to poison damage or to the poisoned condition is not asked (`immuneToRetching`).
Nothing else counts: no rule reads the creature's type, because the SRD data has none for this.
The text a player reads when asked is in `zoneSaveText` (`zones_save.go`).

## Silence

Silence (`zoneOpen`, `silences`) asks nobody and the cast hits nobody; the zone is a sphere of 20 feet that does not spread around corners.
Its three effects are read where they matter, from the zones of the combat; none is stored on a creature.
Creatures occupy one square, so "entirely inside" is the creature's square in the sphere.
`occupiedSquares` (`zones_silence.go`) is the one place that says which squares a creature covers, and `zone.holds` asks that all of them are in the zone.
`silencedBy` says a combatant is entirely inside any Silence of the combat.

- **Verbal gate.** `castFlow.checkSilence` runs right after `pickSlot`, before anything is spent.
  A caster entirely inside a Silence who casts a spell with `Spell.Verbal` gets `failed_precondition`.
  The text names no spell, so it tells a player nothing of a hidden caster's Silence.
  Revivify has its own cast and `revivifyWrite` asks `refuseMuted` too.
  A reactor inside a Silence is offered no Shield, Hellish Rebuke or Feather Fall (`kit.muted`, `slotsFor`).
  A spell with only somatic or material components is cast as usual.
- **Thunder.** `modifiersOf` (the steps of a damage, `combat_parts.go`) adds an immunity step to thunder damage.
  The step names no source, so it tells a player nothing of a hidden caster's Silence.
  `afterResistance` (the damage of zones, effects, traps and reactions) adds thunder to the immunities of a target inside.
  Both read the zones when the damage lands, so a creature pushed in or out by the same cast counts where it stands then.
- **Deafened.** The condition is derived, not stored: `loadEncounter` gives the combatants inside a Silence the `condition:deafened` key (`withSilenceDeaf`), so every read of the combat shows it and nothing is written.
  It ends with the step out or with the zone, and it stays while another Silence covers the creature.
  A deafness a creature already has from another source is left as it is.
  `deafened` is an owner-only condition (`ownerOnlyConditions`): only the master and the creature's player read it.
  A `condition:deafened` the master adds by hand to a creature inside a Silence is dropped (`withoutDerivedDeaf`) and is not kept for when the creature leaves.
  The rules that read the stored condition (a bard's cut of the reaction, the hearing of the Channel Divinity, the hearing flags of the combatant's options) do not see the derived one.
- Sound that passes through the sphere, and objects inside it, are not ruled.
- Whether Subtle Spell lets a caster cast a verbal spell inside a Silence is the master's to rule: the engine has no component model for it, so the app refuses the cast.

## Spike Growth

Spike Growth (`zoneSpikes`) is a cylinder of 20 feet of radius at a point, difficult terrain, concentration up to 10 minutes (100 rounds).
It asks no saving throw and the cast hits nobody.
A creature that moves through the area takes 2d4 piercing damage for every square it enters inside it (`spikeCatch`).
The damage is a pending damage awaiting its roll, with the caster as attacker, rolled by whoever rolls the caster's damage (`catchWithoutSave`).
A move of several squares is one pending damage with the dice multiplied by the squares crossed inside.
Only a movement that crosses squares counts: a walk, a push, a shove and the drag of a grappled creature (`zoneStep.travels`).
The dragged creature is hurt for the squares it travels, and its grappler for its own.
A creature that flies is not hurt, as difficult terrain costs it nothing (`spikeCatch` reads `moverOf(who).Flier`).
A jump, a teleport and the master's placement hurt nobody, because the creature crosses no square.
It spares nobody, the caster included.
It is not once for each turn: the creature is hurt again on every move, so it has no hit mark.
A creature that starts inside, or is put inside, is hurt only when it moves.
The zone shows to every player.
The Perception check that lets a creature see the camouflaged ground is not ruled; the master rules who noticed it.

## Wall of Fire

Wall of Fire (`zoneWall`, `zones_wall.go`) is a wall of fire laid from a point, concentration up to 1 minute (10 rounds).
The caster says the wall in `CastSpellRequest.wall` and `PreviewSpellAreaRequest.wall` (`SpellWall`), with the point in `origin`.

- **The request.** A straight wall has a `direction` (one of the eight) and the side `LEFT` or `RIGHT`; a ring has no direction and the side `INNER` or `OUTER`.
  `length_ft` is the straight wall's length or the ring's diameter: 0 is the full size (60 feet, 20 feet), anything else a multiple of 5 up to it.
  A cast of the spell without `wall` is refused with `WALL_PLACEMENT_REQUIRED` before anything is spent.
  A malformed wall, or a wall sent for another spell, is `invalid_argument`.
- **The squares of a straight wall.** The wall starts on the point and takes one square at a time in its direction while the map's distance from the point, plus the square, is at most the length.
  The distance is the map's own (`grid.RangeFt`), so 60 feet are 12 squares along a row and 9 along a diagonal.
- **The squares of a ring.** The radius is half the diameter taken up to the next multiple of 5, and the ring is the squares at that distance from the point, which is not one of them.
  A diameter of 20 feet is 16 squares and a diameter of 10 feet is the 8 squares around the point.
- **The terrain.** A square off the map or a wall of the map is left out; a straight wall ends at the first such square, and a ring is cut by the line of effect from its point.
- **The damaging side.** The cast stores the wall in `spell_zones` (`wall_side`, `wall_dx`, `wall_dy`, `wall_length_ft`, migration 00311), and the zone keeps the squares the damage reaches (`reach`): the wall's own squares and the squares within 10 feet of one of them on the damaging side.
  For a wall that runs east the left side is north.
  For a ring the inner side is the squares closer to the point than the ring.
- **The cast.** The creatures in the wall's squares make a Dexterity save, the caster too, for 5d8 fire damage (one more d8 for each slot above the 4th), half on a pass.
  A player's character is asked as for any zone.
- **The hits after the cast.** They ask no save and are a pending damage rolled by whoever rolls the caster's damage (`catchWithoutSave`).
  A creature that ends its turn in reach of the damaging side, or inside the wall, takes the damage once for each wall.
  A creature that enters the wall's squares takes it too, as a separate hit: entering and ending the turn inside are two hits.
  Entering twice on a turn is one hit.
  The marks of the cast, the entry and the end of a turn are different (`:cast`, none, `:end`), so a caster standing in its own wall is asked at the cast and hit at the end of its turn.
  A defeated creature is not hit.
- **Sight and movement.** The wall is a cloud (`sightCloud`): its squares block sight as Fog Cloud's do (see above).
  It does not block movement.
- **The turn options.** The spell is listed like any other: the cast sheet's "Onde fica" step sends `wall`.
  The server never sends `DISABLED_REASON_CODE_WALL_PLACEMENT_REQUIRED`; the value stays on the wire.
- **The burning side.** `SpellZone.wall_side` and `burning_squares` tell it: the squares the damage reaches that are not the wall's own (`burning`).
  The master reads all of them and a player the ones they see now.
  `PreviewSpellAreaResponse.burning_squares` and `length_ft` tell the caster the same for the wall they are laying.
- **Privacy.** A hidden caster's wall is the master's alone: no player reads its squares.
  The pending damage of a hit names the caster only to the master and the caster's own player.

### The web: "Onde fica"

The web knows a spell lays a wall by its catalog key, `spell:wall-of-fire`, named once in `WALL_SPELL_KEY` (`core/combat/spell-wall.ts`); the turn options give it only `AreaPlacement.POINT`, as for Moonbeam.
`AreaFlow` takes that flag and keeps the wall as a signal (`wall`, a `WallChoice`: shape, direction, length in feet, side), a line to the east at 60 feet with no side picked (`WallSide.UNSPECIFIED`).
The server refuses a wall without a side, so until one is picked the preview is asked with a provisional side (`askable`: LEFT or INNER), whose burning squares the map does not draw, and the step is not `ready`: the cast is never sent without a side.
Turning the wall (`reorient`, a drag or the pad) takes the picked side back, because LEFT and RIGHT would swap their sense on the screen.
`AreaFlow.stops` compares the squares of the answer with the open-floor count of a straight wall (`stopsText`, the map's distance as the server counts it, 9 squares for 60 feet along a diagonal); a ring is not compared.
Every change of the wall, and every placed start, asks `PreviewSpellArea` again with the wall; only the answer of the latest ask counts.
The cast sends the same wall in `CastExtras.wall`, and the cast's key follows it.
The cast sheet and the master's dialog show the step "Onde fica" (`app-wall-controls`, `shared/area-picker/wall-controls.ts`):
- The shape is a group of two radios, "Linha reta" and "Anel", above the map.
- A line has a direction pad of eight labelled buttons ("Para o nordeste") in a group named with the current way ("Direção da muralha: leste"), beside the drag on the map; a ring has no direction.
- The length of a line is a stepper of 1,5 m (one square, 5 feet) from 1,5 m to 18 m.
- A ring has two sizes, 3 m and 6 m across (10 and 20 feet), because the server rounds the radius up to 5 feet, so 5 and 10 feet are one ring and 15 and 20 feet another.
- The side is a group of two radios named by where the side lies on the screen (`sideChoices`): for a wall that runs (dx, dy) the left side lies toward (dy, -dx), so an east wall has "Acima" and "Abaixo"; a ring has "Dentro" and "Fora".
- "Calculando onde a muralha fica…" (hourglass) shows while the preview is on its way, and the stop warning shows in the same live region.
- The note of the side names the dice of the slot (`damageDice`).

The map (`app-area-map` with `wall`) draws only what the server answered: `squares` as bricks and `burningSquares` as arrows with a dashed edge, through the same zone layer and looks as a laid zone.
The first square of a line, or the centre of a ring, is a square outlined in the accent colour, named "Começo" or "Centro" in the legend.
A press on a square and a release on another sets the start and the direction (`WallDrag`), and the reader of the screen hears the direction; a tap only sets the start.
The start stays on the placed square when the mouse passes over another, and the last wall stays drawn until the next answer arrives.
The map takes the focus when the sheet opens with `preventScroll`, so the steps and the slot row above it stay in view.
The map keeps `touch-action: none`, so a finger on the map does not scroll the sheet; the hint says "Ou use as setas abaixo" for the direction pad.
The cast sheet's footer is pinned under the scrolling body with "Cancelar" and "Próximo: quem está nela" side by side, and the list step, "2 Quem está nela", follows as for any area.
`app-area-step` takes `all` (`WALL_STEPS`) to show both steps in a row, the current one being the heading that takes the focus.
A ring has a card under its controls (`wallSummary`, `core/combat/wall-summary.ts`): "Anel de 6 m · 16 quadrados" from `squares` and `length_ft` of the preview, and "Na muralha agora: Kobold 2 (teste de Destreza)." from its `targets`, so it names only the creatures the caller sees.
The card is not drawn while a newer preview is on its way, and a straight wall has none (its note counts the squares).
The master's dialog has the same button text and no card; it lists who is inside in its second column.

## What a zone does not change in an attack

A cloud or darkness changes what a viewer sees on the map.
An attack or damage roll gets no advantage or disadvantage from a cloud or a Darkness.
Who sees whom for a reaction follows them: a reactor that cannot see the mover or the caster does not get an opportunity attack, a Counterspell or a reaction to damage.
Whether an NPC attacks a player it cannot see with disadvantage is not ruled, nor is any attack rule on a map with the fog off (issue #700, under #640).

## The save question, the end and the other effects

- **The question.** A zone with a save that catches a player's character (`Kind == kindPlayer`), or an NPC with Legendary Resistance and a use left (`asksTheSave`, `zones_trigger.go`), opens an `EFFECT_SAVE` reaction window with `windowTrigger.Zone` (`askZoneSave`, `zones_save.go`); the answer is `RollEffectSave`, which loads the zone in `loadZoneSave` and settles it in `settleZoneSave` (damage halved on a pass, or the web's restraint) through the same `applyZone` an NPC's catch uses.
  The prompt (`zoneSavePrompt`) carries `zone_id`, `zone_reason`, the damage type and the half, and the DC only for the master, the caster's player and, with the table rule `show_save_dc` on, the player whose creature rolls the save (`saveDCFor`).
  The windows of one creature on one turn share a group (`zoneWindowGroup`), so the existing order rule applies, and `openOne` lists a player the first only.
  A zone that ends closes its windows (`closeZoneWindows`).
- **An NPC with Legendary Resistance.** The zone asks its save of the master, not of a player: the window is the master's alone, and the master answers it with the roll and may name the trait ([Creatures](creatures.md#legendary-resistance)).
  A player reads the wait any window gives.
  The cast's own line lists no save for that creature, as for a player's character, and the line of the save follows the answer.
  An NPC without the trait, or with no use left, is rolled at once.
  A zone's save counts the spell as magic: advantages against magic apply, as for any save against a spell.
- **No save.** A zone whose row has no save ability inserts a pending damage awaiting its roll with the caster as attacker (`catchWithoutSave`), rolled and applied by the paths of any damage.
- **Ending.** `endZonesOf` writes an `ended` line for each zone (reason `concentration` or `concentration_replaced`), the clock writes `duration`, `EndSpellZone` (master only) writes `ended_by_master`. They are written at once, not held, because a concentration also ends outside the changes that flush the zone lines.
- **Speed and terrain.** A zone with `slows` adds `effect:spirit-guardians-slowed` (speed 50%, until the end of the creature's turn) when it catches a creature. A zone with `difficult` is added to the `ZoneDifficult` layer of `terrainOf` (`withZoneTerrain`, a layer of its own, so the map's `Difficult` layer is untouched). `Terrain.DifficultFor` reads both: a mover with `Mover.IgnoresMapDifficult` (the `ignore_difficult_terrain` flag, set by `moverWith`) skips the map's layer and still pays for a zone's.

## The view and the privacy

`Encounter.zones` is built by `encounterData.zonesView` from the zones read with the combat (`loadEncounter`).
A player gets the zones of casters they `sees` (not hidden, not in the dark of a fog map); the master gets all, with `SPELL_ZONE_VISIBILITY_MASTER_ONLY` on the zones of hidden casters.
`rounds_left` is the round the zone ends at, minus the current round, 0 for a zone with no round count.
The hint of a zone that moved goes to the master alone when the caster is hidden (`publishZoneMoved`).
The event of a catch is `Secret` for a hidden caster or a hidden creature, and its actor is the caster and its target the creature, so the log's own rule hides it from a player who does not see either (RN-10); the DC is the master's, and the target's player's with `show_save_dc` on (`effectLogRoll`).
A player gets the numbers (`save_*`, damage, `notes`, `moves_per_turn_ft`, `rounds_left`) only for a zone of their own character; for another they get `details_hidden` and the name, shape, squares, concentration and the occupants they see. On a fog map a player reads the squares of a zone only where they see now (`onlySeen`, `zones_view.go`), never the squares they remember: a zone with none left is not sent, and the origin is one of the squares they read.
A zone that blocks sight has no square the fog counts as seen, so its face counts: a square of it that touches a square the player sees now (`faces`).
The answer of a zone half in sight is the same as a zone whose squares are all the ones they see: no flag and no count says there is more.
A straight wall seen by its end tells the length of the squares read, and no side until a burning square is in sight.
The caster's own player and the master read the whole zone, and a map without the fog sends it whole.

## Tests

`zones_silence_test.go` (Silence: the gate, the immunity, the deafened state, the overlap and the hidden caster), `zones_grease_test.go` (Grease: the shape, the cast, entering, the end of the turn, the end of the zone and the undo; Entangle: the shape, the cast only, the end with the concentration and the undo) and `zones_test.go` (Spirit Guardians, Moonbeam, Cloudkill, Web, the push, the forced move, the undo, the player's character, the permissions and the retry) and `zones_hidden_test.go` (the viewer matrix for a hidden caster, a revealed one and a hidden creature, and the stream hint), `zones_spikes_test.go` (Spike Growth: the area, the damage for every square, what counts as movement) and `zones_wall_test.go` (Wall of Fire: the geometry, the request, the cast, the later hits, the fog, the placement in the turn options and the refusal of a cast without a wall); `internal/leaktest` classifies `MoveSpellZone`.
`internal/leaktest` reads the map as a player (`mapzones_test.go`): a Darkness takes a creature off a darkvision player's map while a Devil's Sight player and the master keep it, a hidden caster's Fog Cloud or Wall of Fire reaches no player but the unseen squares, a revealed caster's wall reaches a player only where they see (the master reads the burning side behind it), the damage a hidden caster's wall leaves names no caster to a player, a hidden caster's Silence deafens in the view of the master and the creature's own player alone, and a cloud that ends leaves nothing new in the streams.
The grid cube is in `rules/grid/area_test.go`.

## The web

The zones ride on `Encounter.zones`: a change arrives as the usual `encounter_changed` hint (the master alone when the caster is hidden) and the combat is read again, so a refresh carries them too and nothing else is wired.
`core/combat/zones.ts` turns each `SpellZone` into a `ZoneView` (label, detail pieces, rounds left, concentration, occupants, the "Só você vê" flag, `canMove`, `canEnd`); a zone with `details_hidden` reads name, area, caster, concentration and occupants only, even if a field slipped through, and the client never fills what the server left out.
`zone-move.ts` holds the move range copy (`ZONE_MOVE_RANGE_FT`, the server's `moveRangeFt`) and `zoneMovePreview`, which draws the open-floor squares with `sphereSquares`; no wall cuts the preview, the server cuts the area on commit.
`MoveSpellZone` and `EndSpellZone` are in `CombatCastCalls`, one idempotency key per request kept across retries and renewed after success; `zone-errors.ts` words their refusals.
The log lines for `ZONE_MOVED`, the `zone_enter`, `zone_start`, `zone_cast`, `zone_end` and `zone_move` catches and the end reasons are in `combat-log-effects.ts`.

### The map, the panel and the end

- **Map layer.** `shared/combat-map/zone-layer` (`app-zone-layer`) is projected into `app-combat-map` by `app-combat-map-card`, the way the area overlay is, so it follows the map's zoom and crop and sits under the tokens.
  It draws each `ZoneView.squares` with `cellsPath` and `outlinePath`.
  A wall's `ZoneView.burningSquares` (the server's `burning_squares`, already limited to what the viewer may read) is a layer of its own.
  `core/combat/zone-pattern.ts` gives every catalog spell a look of its own.
  A look is a drawing class (vertical, horizontal and diagonal lines, dots, waves, triangles, rings, asterisks, squares, crosses, light dots, bricks, arrows), a tint of the map's tokens and a veil.
  The veil says what the zone does to sight: 38% white for the first six and for Silence and Spike Growth, 72% white for Fog Cloud and Stinking Cloud, 80% ink for Darkness, 72% garnet for the wall.
  The burning side is its own layer: garnet arrows, dashed outline.
  `arrowTurn` turns the arrows from the server's `wall_side`.
  A straight wall runs from its origin to its farthest square, and the arrows point to its left or right, snapped to eight directions.
  A ring has one fixed axis for the whole pattern: its arrows point right for the outer side and left for the inner side, so they are not radial.
  On a ring the dashed edge and the legend's "Lado que queima" carry the side.
  A wall whose way is unknown (one visible square, or only the origin) gets the dashed edge and no arrows.
  Only a spell the table defined, or one the app does not know, takes a hash of its key.
  The hash lands only on the looks flagged `hashable`, so it never gives a spell a veil that hides sight.
  The map's tokens do not follow the theme, so a look reads the same in both.
  Where zones lie over each other, each shared square is cut on its diagonal (`zone-overlap.ts`).
  The older zone's pattern fills the upper left triangle, the newer one's the lower right, and both outlines stay.
  A zone's own fill leaves those shared squares out, so only the triangles paint them.
  A square that three zones cover shows its two newest.
  No name floats on the map: each zone carries its letter in a white corner diamond (`ZoneView.letter`).
  The letters follow the viewer's list in the server's order, oldest first, so when a zone ends the later ones move up.
  The master's list also holds the zones of hidden casters, so a zone's letter can differ between the master and a player.
  The live region says which zones took another letter.
  The burning side takes the wall's letter and has no badge of its own.
  `zone-badge.ts` places each badge in a corner of its zone's box (inside the corner square only when a square is at least as wide as the badge), then along the edges when the corners are taken, and keeps every badge inside the map, and apart where there is room.
  The badge's size, inset and border come from constants in `zone-badge.ts` that the layer and the legend's diamond both read as CSS variables.
  The legend (`zoneLegend`) and the list show the same letter in a diamond (`app-zone-letter`, read as "Zona A") beside the name, the caster and the concentration.
  A burning side that is a master-only wall's is master-only in the legend too.
  The master's calls (Darkness against a light spell, Fog Cloud's wind, Stinking Cloud's breathing) are a `call` key on the zone def, sent as a note (`master_light`, `master_wind`, `master_breath`); `addDetails` sends the key to the master alone (the caster's player gets the other notes and no call; a leak row in `internal/leaktest` reads the raw body), `zoneView` turns it into the one-line `reminder`, and the app rules none of them.
  The layer and its badges are `aria-hidden`; the panel is the accessible version.
  The move page's map does not draw zones.
- **Panel.** `pages/live-session/combat/zones-panel` (`app-zones-panel`) lists the zones: the master's column and the player's side column. Its live region (`zoneNotice`) compares the ids of the list with the last one it read. `details_hidden` cards render the name, the area and the caster only.
- **End.** The panel asks in place and emits `end`; `ZoneFlow` (`zone-flow.ts`, built in `combat-view-parts.ts`) calls `endSpellZone` under the screen's one-call-at-a-time rule (`calls.busy`) and words a refusal with `zoneEndError`.
- **Move.** "Mover" in the zone's "Mais ações" menu emits `move`; `ZoneFlow.move` (only for a zone whose view says `canMove`) sets `moving` and asks `CombatSheets.moveZone` to open `ZoneMoveSheet` (`pages/live-session/combat/zone-move`), a bottom sheet on a phone and a dialog from a tablet up (`openSheet`); a scene without a battle map opens nothing and says so in the panel. The sheet holds the picked square and wraps `app-zone-move-map` (`shared/area-picker`): `app-combat-map` in `pickSquares` mode (click, arrows from the zone's origin, Enter) with two `app-area-overlay` layers, the current footprint dimmed and the new one from `zoneMovePreview`. Past `ZONE_MOVE_RANGE_FT` a player's square is drawn refused with the `TARGET_OUT_OF_REACH` wording (`zoneOutOfReach`); the master has none. "Mover a zona" calls `moveSpellZone` once (the client's keyed helper), applies the encounter the server answers, closes on success and says `zoneMoveError` in place on a refusal; `moving` returns to `null` when the sheet closes.
- **Save prompt.** An `EFFECT_SAVE` window whose prompt has `zone_id` is the player's `EffectSaveSheet` (the same `RollEffectSave` path, dice modes and held-roll question as any effect save). `core/effects/zone-save.ts` words it: the title is the zone and its caster (found in `encounter.zones` by `zone_id`), the subtitle "Magia que ficou no mapa" and the reason (`zone_enter` "Você entrou na zona.", `zone_start` "A zona atingiu você no começo do seu turno."), then the ability, the failure from `damage_type_name_pt` and `half_on_save`, the bonus and the spell. The DC is written (", CD 14", and "contra CD 14" after the roll) only when the server sent `dc`; without it there is no number and no placeholder. A zone that restrains adds the static "Soltar-se gasta uma ação." The master's queue words the same window with the reason of the creature. The damage a zone with no save owes is a `PendingDamage` with no zone fields, so its card says "Zona de magia: <spell>" when the caster has a zone of that spell (`zoneLabel`).
