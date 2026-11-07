import type { TableEffect } from '../../../gen/meurpg/rules/v1/table_content_pb';

/**
 * What a player reads of the effects that change a roll: a reroll, a minimum on a die and the critical range, each as a lead and
 * a sentence ("Rolar de novo: jogada de ataque e teste de resistência, com 2 ou menos; gasta 1 uso."). Written here, from the
 * effect the form would send, and not by the server: the editor's preview and the player's page are one text.
 */

/** A lead and its sentence (the same shape as `RollItem` of content-read-parts, which imports this file). */
export interface RollItem {
  readonly title: string;
  readonly text: string;
}

const ROLL_WORDS: Readonly<Record<string, string>> = {
  attack: 'jogada de ataque',
  check: 'teste de habilidade',
  save: 'teste de resistência',
  damage: 'dado de dano',
};

const SCOPE_WORDS: Readonly<Record<string, string>> = {
  weapon: 'armas',
  spell: 'magias',
  both: 'armas e magias',
};

const CRITICAL_RANGE_TARGET = 'critical_range';
const WHOLE = /^\d+$/;

/** "a, b e c". */
function listed(words: readonly string[]): string {
  return words.length < 2 ? words.join('') : `${words.slice(0, -1).join(', ')} e ${words.at(-1)}`;
}

function rollsText(rolls: readonly string[]): string {
  return listed(rolls.map((r) => ROLL_WORDS[r] ?? r));
}

/** "gasta 1 uso": what a use of the reroll costs. A formula says only that it spends. */
function spendText(e: TableEffect): string {
  if (e.resource === '') {
    return '';
  }
  const cost = e.cost.trim() === '' ? '1' : e.cost.trim();
  if (!WHOLE.test(cost)) {
    return '; gasta usos';
  }
  return `; gasta ${cost} ${cost === '1' ? 'uso' : 'usos'}`;
}

/** The lead and the sentence of an effect that changes a roll; `null` for every other effect. */
export function rollEffectItem(e: TableEffect, nameOf: (key: string) => string): RollItem | null {
  switch (e.type) {
    case 'reroll':
      return e.rolls.length > 0 && e.atMost > 0
        ? {
            title: 'Rolar de novo',
            text: `${rollsText(e.rolls)}, com ${e.atMost} ou menos${spendText(e)}.`,
          }
        : null;
    case 'roll_minimum': {
      // A feat's pick ("damage-type:@tipo") is the type the player chose, not a key the catalog names.
      const type =
        e.damageType === ''
          ? ''
          : e.damageType.includes('@')
            ? ' do tipo escolhido'
            : ` de ${nameOf(e.damageType).toLowerCase()}`;
      return e.rolls.length > 0 && WHOLE.test(e.value)
        ? {
            title: 'Valor mínimo no dado',
            text: `${rollsText(e.rolls)}${type}, no mínimo ${e.value}.`,
          }
        : null;
    }
    case 'modifier':
      return e.target === CRITICAL_RANGE_TARGET && WHOLE.test(e.value)
        ? {
            title: 'Margem de crítico',
            text: `${e.value} a 20, em ${SCOPE_WORDS[e.applies] ?? SCOPE_WORDS['weapon']}.`,
          }
        : null;
    default:
      return null;
  }
}

/** When a pool's uses come back, in the few words the "Usos" line has room for. */
const RECHARGE_SHORT: Readonly<Record<string, string>> = {
  short_rest: 'descanso curto',
  long_rest: 'descanso longo',
  dawn: 'ao amanhecer',
  none: 'não voltam sozinhos',
  initiative: 'ao rolar iniciativa',
};

/** "Usos: 3, descanso longo.": a pool of a plain number of uses. A formula is left to the entry's own text, as `effectLine` does. */
function usesItem(e: TableEffect): RollItem | null {
  if (e.type !== 'resource' || !WHOLE.test(e.max)) {
    return null;
  }
  const when = RECHARGE_SHORT[e.recharge];
  return { title: 'Usos', text: `${e.max}${when ? `, ${when}` : ''}.` };
}

/** What an entry's effects say under its text, as the `effects` of its read: the roll effects, then the pools; nothing when it has none. */
export function entryEffectsPart(
  effects: readonly TableEffect[],
  nameOf: (key: string) => string,
): { effects?: RollItem[] } {
  const items = [
    ...effects.map((e) => rollEffectItem(e, nameOf)),
    ...effects.map((e) => usesItem(e)),
  ].filter((i): i is RollItem => i !== null);
  return items.length > 0 ? { effects: items } : {};
}
