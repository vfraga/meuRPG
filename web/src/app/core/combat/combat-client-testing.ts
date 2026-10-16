import { CombatCastCalls } from './combat-client-cast';
import { CombatCombatantCalls } from './combat-client-combatant';
import { CombatAttackCalls } from './combat-client-attack';
import { CombatMoveCalls } from './combat-client-move';
import { CombatTurnCalls } from './combat-client-turn';
import { type CombatClient } from './combat-client';

type PartName = 'turn' | 'moves' | 'attacks' | 'casts' | 'combatants';

const PARTS: Record<PartName, { prototype: object }> = {
  turn: CombatTurnCalls,
  moves: CombatMoveCalls,
  attacks: CombatAttackCalls,
  casts: CombatCastCalls,
  combatants: CombatCombatantCalls,
};

/** The part of `CombatClient` that holds a call, or `undefined` for a name no part has. */
export function combatPartOf(call: string): PartName | undefined {
  return (Object.keys(PARTS) as PartName[]).find((part) =>
    Object.getOwnPropertyNames(PARTS[part].prototype).includes(call),
  );
}

/**
 * A `CombatClient` stand-in for the specs: the calls it answers by name, placed in the part that owns each (the parts come
 * from the real classes, so a call that moves keeps the fake in step). Members that are not calls (a fake's own state) are left out.
 */
export function fakeCombatClient(calls: object): CombatClient {
  const fake: Record<PartName, Record<string, unknown>> = {
    turn: {},
    moves: {},
    attacks: {},
    casts: {},
    combatants: {},
  };
  for (const [name, call] of Object.entries(calls)) {
    const part = combatPartOf(name);
    if (part) {
      fake[part][name] = call;
    }
  }
  return fake as unknown as CombatClient;
}

/** A `CombatClient` whose every part answers any call by name with what `answer` makes of it. */
export function anyCallCombatClient(answer: (call: string) => unknown): CombatClient {
  const part = () => new Proxy({}, { get: (_target, call: string) => answer(call) });
  return {
    turn: part(),
    moves: part(),
    attacks: part(),
    casts: part(),
    combatants: part(),
  } as unknown as CombatClient;
}
