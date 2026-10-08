# Reachability audit: can the screen reach everything the server can do?

Branch `review/reachability`, from `origin/main`. Nothing outside `review/` changed. The scripts and raw inventories are in `review/reachability/`.

## Summary

| Check | What | Inventory | Findings |
| --- | --- | --- | --- |
| 1 | RPCs with no caller | 234 RPCs, 21 services | 0 (2 by design) |
| 2 | Response fields nobody reads | 2,384 fields in non-request messages; 73 never read, 143 more with at most two reading files, checked in context | 4 (RA-02, RA-05, RA-06, RA-07) plus RA-01 |
| 3 | Options the screen disables or hides by its own rule | `enabled`/`can_*`/`for_you` flags (28 fields), the whole combat action panel | 2 (RA-01, RA-03) |
| 4 | Refusals and reasons without a specific text | 29 reason/refusal enums, 18 `*-errors.ts` files | 1 (RA-08) |
| 5 | Stream events with no reaction | 23 events of `WatchGameSession` (the only stream) | 1 (RA-04) |
| 6 | Enum values with no label | 125 enums, 769 values | 0 |

By severity: 0 high, 2 medium (RA-01, RA-02), 6 low (RA-03 to RA-08).

**The three worst gaps**
1. **RA-01**: the Escudo (Shield) +5 armor class is sent in `Combatant.armor_class_bonus` and no screen shows it. The master's order list and the player's armor class badge keep showing the sheet's armor class.
2. **RA-02**: a spell with two damage types (Tempestade de Gelo) logs only the first type; `more_damages` is never rendered in the combat log.
3. **RA-03**: the off-turn "Ataque de oportunidade" button uses a fixed 5 ft reach; the server uses the weapon's reach (glaive, whip), and sends `too_far`.

The audit found no "server can, screen cannot" gap as large as the bonus attack defect. Every RPC has a caller, the stream is fully wired, and the refusal and label maps are almost complete. What is left is display gaps in combat and a few low items.

## Findings

| Id | Severity | Check | Server side | What the screen does instead | Who is affected, when | Fix size |
| --- | --- | --- | --- | --- | --- | --- |
| RA-01 | medium | 2/3 | `Combatant.armor_class_bonus` (`proto/meurpg/play/v1/combat.proto:1764`): +5 with Escudo until the caster's next turn, "only the master and the combatant's own player"; `armor_class` is the sheet's alone (`:1760`) | Never read. The master's list shows `CA {armorClass}` (`web/src/app/pages/live-session/combat/order-list/order-list.ts:225`); the player's badge shows the sheet's armor class (`web/src/app/pages/live-session/player-vitals/player-vitals.ts:55`) | Master, when a wizard casts Escudo and the master reads the armor class to judge the next attacks; the Escudo player, who is told nothing about the +5 | S |
| RA-02 | medium | 2 | `CombatLogSpellTarget.more_damages` (`combat.proto:3419`): the other damage types of a cast (Tempestade de Gelo: pancada and frio) | `castTargetText` writes only `t.damage` (`web/src/app/core/combat/combat-log.ts:209`); `moreDamages` is read nowhere. The cast sheet itself is fine: it lists `SpellCast.pending_damages` (`cast-sheet.ts:639`) | Everyone who reads the log after a two-type spell; the log undercounts the damage | S |
| RA-03 | low | 3 | Opportunity attack reach is "5 ft, or the reach of its melee weapon" (`combat.proto:325`, `:637`); `TargetInReach.too_far` (`:2511`) | The off-turn button list keeps only melee attacks with a target at `distanceFt <= 5` (`web/src/app/pages/live-session/combat/combat-view.ts:745-751`), ignoring `tooFar` and weapon reach. A player with a reach weapon gets no button for a target at 10 ft; the server's own offer (`opportunity_offers`, `OpportunitySheet`) still works | Player with a glaive, halberd or whip, when an enemy walks away at 10 ft | S |
| RA-04 | low | 5 | `content_changed` (`play.proto:1176`): "open sheets read the character again" | The live session page handles it by forgetting the spell catalog only (`web/src/app/pages/live-session/live-session.ts:567`). The page's own copy of the player sheet (`playerSheet`, used for the trap skills, `:373`) is not read again. Character sheet and editor pages do listen (`XpWatcher`, `ContentWatcher`) | Player on the live page after the master edits a class or race mid-session; stale until a reload. Rare | S |
| RA-05 | low | 2 | `CombatLogEntry.returned_to_reach`, `return_blocked` (`combat.proto:3340-3343`): an opportunity attack dropped the mover to 0 and it went back to the square (or could not) | Never read (`combat-log.ts` `attackText`). The token moves through `combatant_moved`, but the log never says why | Master and players, in the rare 0 HP opportunity attack | S |
| RA-06 | low | 2 | `Content.attribution` (`rules.proto:960`): "the app must show on its credits page, exactly as given" | `web/src/app/pages/credits/credits.ts` holds a copy of the text and never reads the field. Not wrong today; nothing ties the two together | Nobody now; the credits would drift if the server text changed | S |
| RA-07 | low | 2 | `CombatLogSpellTarget.attack_roll`, `cover_bonus`, `CombatLogEntry.attack_roll` (`combat.proto:3403`, `:3416`, `:3293`) | Not read by the log (the cast sheet reads `SpellTargetResult.attack_roll`, `cast-flow.ts:551`). The master's log of a spell attack shows hit or miss without the d20 or the cover bonus | Master, reading back a spell attack | S |
| RA-08 | low | 4 | `DisabledReason.min_level` for `NO_SLOT`: "the lowest slot level that would do" (`rules.proto:1708`) | Never read; the reason is the generic "Sem espaço" (`web/src/app/core/combat/combat-options.ts:41`), while `NO_USES` does use its `recharge` (`:58`) | Player or master looking at a disabled spell with no slot of the needed level | S |

## Checked, by design

- `CampaignService.SetCampaignDiceMode` has no caller: the dice mode is saved with `SetTableRules` (`docs/architecture.md:634`, `:1292`; web `core/campaigns/table-rules.ts:207`).
- `CombatService.EndConcentration` has no caller: the web uses `SetCombatantConditions{end_concentration}`, which the proto says ends it the same way (`combat.proto:1050`; web `combat-client.ts:885`).
- `TurnEconomy.bonus_action`, `MovementLeft.used_ft`, `Combatant.movement_used_ft`: the screen reads `Combatant.bonus_action_used` and `movement_used_dft` instead (same facts).
- `SpellTargetResult.more_pending_damage_ids`: the cast sheet uses `SpellCast.pending_damages`.
- `ReachableSquare.known_trap_point_id`, `PendingDamage.trap_point_id`, `TrapDamageDone.damage_id`, `XPBlocked.treasure_point_id`, `XPAward.given_by_user_id`, `ImageGeneration.source_image_id` / `reference_image_ids`, `FullSheet.content_revision` / `content_baselines` / `known_issues`, `*.generator_version`, `*.already_rolled`: ids or values the client passes back or that are for the server.
- `Attack.versatile_damage_dice`: the sheet shows the text `versatile_damage`; no RPC takes a two-handed choice, so this is a server limit, not a screen gap.
- `GameSessionBlockedReason.SESSION_ALREADY_OPEN`, `SESSION_NOT_ENDED`, `MoveRefusal.TOO_COSTLY`, `PuzzleStopReason.MOVES`: handled by code or default branch with a specific text (`game-session-card.ts:176`, `move-plan.ts:95`).
- `CharacterBlockedReason.TABLE_CONTENT_STAYS`: for copying a character to another campaign, which is Stage 11 (`docs/roadmap.md`, MR-021).
- Roadmap later work: a list of past sessions (the server already answers; `docs/roadmap.md` Stage 11).
- Enum values reached by string keys, not `Enum.VALUE`: conditions (`core/combat/conditions.ts`, all 15 SRD keys), damage types (`character-labels.ts`, 13 of 13), door kinds (a local `DoorKind` with its own labels in `shared/map-layers`).
- `DungeonDoor.trapped` in the preview, `Member.joined_at`, `TreasureItem.category`, `GetMagicItemResponse.category` / `variants` / `variant_of`, `SummonSpellOptions.can_cast_with_slot`, `CharacterCreature.depends_on_concentration`: display extras with no action or decision behind them; not counted as findings.

## Inventories

- RPCs: 234 in 21 services (`rpcs.json`). Wrapper and caller traces: `wrappers.json`, `wrappers2.json`.
- Fields: 2,384 fields of non-request messages. 73 are never read as `.field` in `.ts` or `.html` (`fields-unread.json`). 143 fields share a name with another message and have at most two reading files (`fields-shared.json`, verified in context in `shared-A.txt` / `shared-B.txt`).
- Enums: 125 enums, 769 values (`enums.json`).
- Events: 23 `WatchGameSession` events, each traced from `live-session-source.live.ts` to a handler in `live-stream.ts` and `live-session.ts`. `onContentChanged` is the only partial reaction (RA-04).
- Method limits: the field scan is textual, since the TypeScript service could not type the generated messages without the npm packages (installing them was out of scope). Check 3 was done by reading the combat action panel, `combat-options.ts`, the 28 `can_*`/`enabled`/`for_you` fields and the web's own `[off]` rules, not every screen.
