import type { DiceRoll } from '../../../gen/meurpg/play/v1/combat_pb';
import type { Reroll } from '../../../gen/meurpg/play/v1/combat_rolls_pb';

/**
 * The halfling's Lucky, "Sortudo" (SRD 5.1, Halfling): a 1 on the d20 of an attack roll, an ability check or a saving
 * throw may be rolled again, and the new roll stands. The server holds the roll and asks (`held_roll_offer` on the roll's
 * response: the weapon attack, and the scene check, group check, trap search, hint try, spell attack, effect save and
 * concentration save; `HeldRollController` answers it); these are the words the sheets and the combat log use for it.
 */

/** Whatever tells the dice a feature rolled again: a `DiceRoll`, a group check's `CheckRoll`, an effect's
 * `EffectSaveResult` or the log's `CombatLogEffect`. */
export interface WithRerolls {
  readonly rerolled: readonly Reroll[];
}

/** The content key of the trait: a d20 it rolled again names it in `DiceRoll.rerolled`. */
export const LUCKY_TRAIT = 'trait:lucky';

/** The question's title. */
export const LUCKY_TITLE = 'Tirou 1: rolar de novo?';

/** The line under the title. With advantage or disadvantage only the die that showed the 1 is rolled again (SRD 5.1,
 * "Advantage and Disadvantage"), so the line names that die. */
export function luckyWhat(offer: { readonly d20?: DiceRoll } | null | undefined): string {
  const pair = (offer?.d20?.faces.length ?? 0) > 1;
  return pair
    ? 'Sortudo: você pode rolar de novo o dado que deu 1 e fica com o novo resultado.'
    : 'Sortudo: você pode rolar esse d20 de novo e fica com o novo resultado.';
}

/** The content key of the fighter's feature: a save it rolled again names it in `DiceRoll.rerolled`. */
export const INDOMITABLE_FEATURE = 'feature:indomitable-1-use';

/** The content key of the rogue's 20th-level feature: a roll it changed names it in `DiceRoll.rerolled`. On an attack
 * `from` and `to` are the same face, because the d20 is not changed: the miss became a hit. */
export const STROKE_FEATURE = 'feature:stroke-of-luck';

function rerollText(r: {
  readonly sourceKey: string;
  readonly from: number;
  readonly to: number;
}): string {
  if (r.sourceKey === STROKE_FEATURE) {
    return r.from === r.to
      ? 'Golpe de Sorte: o erro virou acerto'
      : `Golpe de Sorte: ${r.from} → ${r.to}`;
  }
  return `${r.sourceKey === LUCKY_TRAIT ? 'Sortudo' : 'Indomitável'}: ${r.from} → ${r.to}`;
}

/** "Sortudo: 1 → 14", "Indomitável: 4 → 15" or "Golpe de Sorte: 6 → 20", beside the d20 the feature changed; `''` when it
 * changed nothing (no 1, the 1 kept, the failed roll kept). */
export function luckyText(roll: WithRerolls | undefined): string {
  return (roll?.rerolled ?? [])
    .filter(
      (r) =>
        r.sourceKey === LUCKY_TRAIT ||
        r.sourceKey === INDOMITABLE_FEATURE ||
        r.sourceKey === STROKE_FEATURE,
    )
    .map(rerollText)
    .join(', ');
}
