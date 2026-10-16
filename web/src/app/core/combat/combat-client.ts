import { Injectable } from '@angular/core';
import { CombatAttackCalls } from './combat-client-attack';
import { CombatClientBase } from './combat-client-base';
import { CombatCombatantCalls } from './combat-client-combatant';
import { CombatCastCalls } from './combat-client-cast';
import { CombatMoveCalls } from './combat-client-move';
import { CombatTurnCalls } from './combat-client-turn';

/**
 * Thin wrapper around the generated `CombatService` client (MR-013), in the
 * same shape as `MapsClient`. `providedIn: 'root'`, and only lazy code
 * imports it, so the generated combat code stays out of the initial bundle.
 * Every write sends an idempotency key made for that call: a double tap
 * never runs twice, and a call the app repeats on purpose is a new action.
 * Callers map errors to Portuguese with `combat-errors.ts`.
 *
 * The calls sit in parts, one per subject, that share the generated client and
 * the keyed sender through one base: `turn` (setting up, turns, actions, undo,
 * log), `moves` (movement, sides, cover, opportunities), `attacks` (attacks,
 * damage, roll modes, reactions), `casts` (spells) and `combatants` (death
 * saves, conditions, effects, hit points).
 */
@Injectable({ providedIn: 'root' })
export class CombatClient {
  private readonly base = new CombatClientBase();
  readonly turn = new CombatTurnCalls(this.base);
  readonly moves = new CombatMoveCalls(this.base);
  readonly attacks = new CombatAttackCalls(this.base);
  readonly casts = new CombatCastCalls(this.base);
  readonly combatants = new CombatCombatantCalls(this.base);
}
