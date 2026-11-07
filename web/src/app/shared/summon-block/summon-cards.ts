import {
  TableSummonReplaces,
  TableSummonTurnMode,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { RadioCard } from '../form-fields/radio-cards';

/** The two radio-card questions of a summon, the same on a feature's block and on a spell's "Invocar". */
export const TURN_CARDS: readonly RadioCard[] = [
  {
    value: String(TableSummonTurnMode.OWN_INITIATIVE),
    label: 'Iniciativa própria',
    hint: 'Cada criatura rola a sua e age no seu turno.',
  },
  {
    value: String(TableSummonTurnMode.OWNER_ROLL_GROUP),
    label: 'Em grupo',
    hint: 'Uma iniciativa para todas, rolada pelo jogador.',
  },
];

/** What a stored turn mode no card names (acting right after the owner, which the server refuses) is called, so the control is never empty. */
const STORED_TURN: Readonly<Record<number, string>> = {
  [TableSummonTurnMode.AFTER_OWNER]: 'Logo depois do dono',
};

/** The cards, with one more for a stored value none of them has: it shows as the checked one, and the server says what is wrong with it. */
export function withStored(
  cards: readonly RadioCard[],
  value: number,
  label: string,
): readonly RadioCard[] {
  const v = String(value || 1);
  return cards.some((c) => c.value === v)
    ? cards
    : [...cards, { value: v, label, hint: 'Valor guardado; escolha outro.' }];
}

export function turnCardsFor(mode: number): readonly RadioCard[] {
  return withStored(TURN_CARDS, mode, STORED_TURN[mode] ?? 'Outro turno');
}

export function replacesCardsFor(mode: number): readonly RadioCard[] {
  return withStored(REPLACES_CARDS, mode, 'Outra escolha');
}

export const REPLACES_CARDS: readonly RadioCard[] = [
  {
    value: String(TableSummonReplaces.NONE),
    label: 'As anteriores ficam',
    hint: 'Até o máximo ao mesmo tempo.',
  },
  {
    value: String(TableSummonReplaces.SAME_SUMMON),
    label: 'As novas tomam o lugar',
    hint: 'As anteriores vão embora.',
  },
];
