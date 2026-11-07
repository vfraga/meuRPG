import {
  TableSummonReplaces,
  TableSummonTurnMode,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { RadioCard } from '../form-fields/radio-cards';

/** The two radio-card questions of a summon, the same on a feature's block and on a spell's "Invocar" (board C2). */
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
