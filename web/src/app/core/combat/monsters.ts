import { Code, ConnectError } from '@connectrpc/connect';

import { EncounterBlockedReason } from '../../../gen/meurpg/play/v1/combat_pb';
import { listWithE } from '../creatures/bestiary-format';
import { describeConnectError } from '../connect/connect-errors';
import { ActionKey } from '../connect/idempotency';
import { combatErrorMessage, encounterBlocked, sessionClosed } from './combat-errors';

/** The most monsters of one creature one "Pôr no combate" puts in (`AddMonsters`: 1 to 10). */
export const ADD_MAX = 10;
/** The most a combat has (combat.proto: 40 combatants in all). */
export const COMBAT_MAX = 40;
/** The longest base name (`AddMonsters`: 1 to 30 characters, one line). */
export const MONSTER_NAME_MAX = 30;

/**
 * The names the monsters take: "Bandido 1", "Bandido 2" and "Bandido 3" for three, and the plain name for
 * one (the server keeps it plain when no other combatant has it; "Bandido 2" may already be there, then the
 * server numbers on, which only it knows: the preview says what an empty combat would do).
 */
export function monsterLabels(base: string, count: number): string[] {
  const name = base.trim();
  return count <= 1 ? [name] : Array.from({ length: count }, (_, i) => `${name} ${i + 1}`);
}

/** "Bandido 1, Bandido 2 e Bandido 3", for the line "Entram como …". */
export function monsterSentence(base: string, count: number): string {
  return listWithE(monsterLabels(base, count));
}

/** How many more fit in a combat that has `existing` combatants, never past one add's 10. */
export function roomFor(existing: number): number {
  return Math.max(0, Math.min(ADD_MAX, COMBAT_MAX - existing));
}

/** What one "Pôr no combate" asks, as the key follows it: a retry with the same parameters repeats the key. */
export interface MonsterAdd {
  readonly creatureKey: string;
  readonly count: number;
  readonly name: string;
  readonly hp: 'average' | 'rolled';
  readonly hidden: boolean;
  /** The combat it goes into, or `''` for the start of one. */
  readonly target: string;
}

/**
 * One idempotency key per sheet open, kept while the parameters stay the same: the server answers a retry of
 * the same add with the same ids and refuses ("invalid_argument") the same key with other parameters. So a
 * second tap, or a retry after a lost answer, adds once; when the master changed something since the last
 * try, it is a new add and takes a new key.
 */
export class AddKeys {
  private readonly key = new ActionKey();

  keyFor(add: MonsterAdd): string {
    return this.key.keyFor([add.creatureKey, add.count, add.name, add.hp, add.hidden, add.target]);
  }
}

/**
 * The Portuguese message for a refused "Pôr no combate" or "Criar o combate e pôr". The 40 combatants are the
 * server's `invalid_argument` with no detail of its own, so the sheet checks the room before it asks
 * (`roomFor`) and says it in words; here the other refusals: no combat, a combat that ended, no open session,
 * no map for a combat with one, and a player.
 */
export function addMonstersErrorMessage(err: unknown): string {
  const blocked = encounterBlocked(err);
  if (blocked?.reason === EncounterBlockedReason.TOO_MANY_COMBATANTS) {
    return 'Não cabem mais combatentes neste combate. Nada foi gasto.';
  }
  if (blocked?.reason === EncounterBlockedReason.ENCOUNTER_ENDED) {
    return 'Esse combate já terminou. Feche e comece outro.';
  }
  if (sessionClosed(err)) {
    return 'Não há sessão aberta: os monstros entram num combate da sessão. Abra a sessão e tente de novo.';
  }
  if (blocked || ConnectError.from(err, Code.Unavailable).code === Code.FailedPrecondition) {
    return combatErrorMessage(err, 'pôr os monstros');
  }
  return describeConnectError(err, {
    [Code.InvalidArgument]:
      'Não deu para pôr os monstros: o combate tem no máximo 40 combatentes, e o nome pede de 1 a 30 letras, numa linha só. Confira e tente de novo.',
    [Code.NotFound]: 'Esse combate não existe mais. Feche e abra a folha de novo.',
    [Code.PermissionDenied]: 'Só o mestre da campanha põe monstros no combate.',
    [Code.Unavailable]: 'Não deu para pôr os monstros: o servidor não respondeu. Tente de novo.',
  });
}

/** "Bugbear 1 e 2", "Hobgoblin 1 a 4", "Ogro": the names a group of an encounter becomes ("vira ..."). */
export function becomesText(base: string, count: number): string {
  if (count <= 1) {
    return base;
  }
  return count === 2 ? `${base} 1 e 2` : `${base} 1 a ${count}`;
}

/**
 * "Bandido 1 a 3", "Bandido 1 e 2", "Ogro": the monsters of one creature as the end-of-combat XP lists them (E10-08 state 9).
 * Labels that do not share a base name and a number (a renamed monster) are listed one by one.
 */
export function groupLabel(labels: readonly string[]): string {
  if (labels.length <= 1) {
    return labels[0] ?? '';
  }
  const parts = labels.map((l) => /^(.*\S) (\d+)$/.exec(l));
  const base = parts[0]?.[1];
  if (!base || parts.some((p) => p?.[1] !== base)) {
    return labels.join(', ');
  }
  const numbers = parts.map((p) => Number(p![2]));
  return numbers.length === 2
    ? `${base} ${numbers[0]} e ${numbers[1]}`
    : `${base} ${Math.min(...numbers)} a ${Math.max(...numbers)}`;
}
