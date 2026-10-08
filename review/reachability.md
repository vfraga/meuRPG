# Reachability audit — work in progress

Status: check 1 done (every RPC has a caller). Checks 2–6 follow; this file is rewritten at the end.

## Check 1 — RPCs with no caller

234 RPCs in 21 `.proto` files (`review/reachability/rpcs.json`, by `rpcs.mjs`). Each was traced to a wrapper method in `web/src/app/core/**` (or a page's own source/port class) and from there to a component or store (`wrappers.mjs`, `wrappers2.mjs`; the raw second-hop dump is `wrappers2.json`). No RPC is without a screen path except the two below, both by design.

- `CampaignService.SetCampaignDiceMode` (`proto/meurpg/campaigns/v1/campaigns.proto:192`): no caller. The dice mode travels in `SetTableRules.rules.dice_mode` (`web/src/app/core/campaigns/table-rules.ts:207`), the "Regras da mesa" page. `docs/architecture.md:634` ("the dice mode goes in the same transaction") and `:1292` ("the master's 'Dados' panel only summarizes the mode and links here").
- `CombatService.EndConcentration` (`combat.proto:1067`): the web ends concentration through `SetCombatantConditions{end_concentration}` (`web/src/app/core/combat/combat-client.ts:885`), which the proto comment (`combat.proto:1050`) says does the same thing ("this one has no other job").
