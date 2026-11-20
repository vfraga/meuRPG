import {
  ConsumableKind as ServerKind,
  type SheetConsumable,
} from '../../../gen/meurpg/characters/v1/characters_pb';

/**
 * What a consumable entry of the sheet is and costs, as the server says it (`SheetMagicItem.consumable`). The web reads it
 * and decides nothing: which items are consumables, which are oils and which cost the action are the server's rules.
 */

/** What a consumable is: a potion (drunk or given), an oil (applied), a scroll, or any other single-use item. */
export type ConsumableKind = 'potion' | 'oil' | 'scroll' | 'other';

/** The dice of a healing use: how many, their faces and the number added; `text` is the formula as a line writes it. */
export interface HealingDice {
  readonly count: number;
  readonly sides: number;
  readonly modifier: number;
  readonly text: string;
}

/** The lasting effect a use leaves, in the words the server sent: its title ("Resistência a fogo") and its whole length ("por 1 hora"). */
export interface LastingEffectVm {
  readonly title: string;
  readonly length: string;
}

/** An entry of the sheet that "Usar" accepts. */
export interface ConsumableVm {
  readonly kind: ConsumableKind;
  /** Using it spends the user's action in a running combat. */
  readonly costsAction: boolean;
  /** The healing roll of the use; `null` when the use rolls nothing. */
  readonly heal: HealingDice | null;
  /** The effect the use leaves on the receiver, when it leaves one that lasts (the server sends its title and length). */
  readonly lasting?: LastingEffectVm;
}

const KINDS: Record<number, ConsumableKind> = {
  [ServerKind.POTION]: 'potion',
  [ServerKind.OIL]: 'oil',
  [ServerKind.SCROLL]: 'scroll',
  [ServerKind.OTHER]: 'other',
};

/** The kind the server named, or `undefined` when it named none (a line of an item that left the catalog). */
export function kindOf(kind: ServerKind): ConsumableKind | undefined {
  return KINDS[kind];
}

/** The text a number added to the dice is said with: "+ 2", "− 1". */
export function signedText(modifier: number): string {
  return `${modifier < 0 ? '−' : '+'} ${Math.abs(modifier)}`;
}

/** The consumable of an entry, or `undefined` for an item that is none (the server leaves the field empty). */
export function consumableOf(c: SheetConsumable | undefined): ConsumableVm | undefined {
  if (!c) {
    return undefined;
  }
  const heal: HealingDice | null =
    c.healDiceCount > 0 && c.healDiceSides > 0
      ? {
          count: c.healDiceCount,
          sides: c.healDiceSides,
          modifier: c.healBonus,
          text: `${c.healDiceCount}d${c.healDiceSides}${c.healBonus === 0 ? '' : ` ${signedText(c.healBonus)}`}`,
        }
      : null;
  const lasting = c.effectTitlePt
    ? { title: c.effectTitlePt, length: c.effectDurationTextPt }
    : undefined;
  return {
    kind: kindOf(c.kind) ?? 'other',
    costsAction: c.costsAction,
    heal,
    ...(lasting && { lasting }),
  };
}
