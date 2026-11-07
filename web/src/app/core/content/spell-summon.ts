import { clone, create } from '@bufbuild/protobuf';

import {
  TableEffectApplies,
  type TableSummon,
  TableSummonSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { SpellDraft } from './spell-types';
import { type SummonDraft, wholeOf } from './summon-draft';

/**
 * The "Invocar" mechanic of the spell editor (#578): the spell's `TableSpell.summon`. It is the same message a feature
 * has, but the spell pays with its slot and has its own time, duration and concentration, so the server refuses a resource, a cost,
 * a time, a duration or a concentration on it (`checkSpellSummonOnly`) and the editor never sends them.
 */

export const SUMMON_TURN_LINE_PT = 'Sem recurso, tempo nem duração: a magia tem os seus.';
export const SUMMON_LOCK_PT =
  '“Invocar” pede uma magia de 1º círculo ou mais, com alvo “Só quem conjura”, sem ataque, teste, dano, cura ou área, e que não seja ritual.';

/** The summon as the save sends it: the stored message (the role and any field this build does not know) with what the block edits over it, and no resource, cost, time, duration or concentration. */
export function draftToSpellSummon(d: SummonDraft): TableSummon {
  const out = d.base ? clone(TableSummonSchema, d.base) : create(TableSummonSchema);
  out.creatureKeys = [...d.creatureKeys];
  out.count = d.count;
  out.countPerSlotLevel = wholeOf(d.perSlot);
  out.maxLive = wholeOf(d.maxLive);
  out.turnMode = d.turnMode;
  out.replaces = d.replaces;
  out.resourceKey = '';
  out.resourceCost = 0;
  out.castingTime = undefined;
  out.duration = undefined;
  out.concentration = false;
  return out;
}

/** What the engine will refuse about the spell next to its summon, said before the save, on the field the master changes. */
export interface SummonRefusal {
  readonly path: string;
  readonly text: string;
}

/** The refusals the spell has while "Invocar" is chosen, in the order of the form (empty for any other mechanic). */
export function summonRefusals(d: SpellDraft): readonly SummonRefusal[] {
  if (d.mechanic !== 'summon') {
    return [];
  }
  const out: SummonRefusal[] = [];
  // In the order the fields are on screen: the level, the duration, the ritual, the target, then the summon itself.
  if (d.level < 1) {
    out.push({
      path: 'table_spell.level',
      text: 'Um truque não invoca: a invocação gasta um espaço de magia. Escolha o 1º círculo ou mais.',
    });
  }
  if (d.kept?.effect && d.duration === 'instant') {
    out.push({
      path: 'table_spell.duration.kind',
      text: 'Esta magia guarda um efeito, que dura enquanto a magia dura: escolha uma duração que não seja instantânea, ou outra mecânica.',
    });
  }
  if (d.ritual) {
    out.push({
      path: 'table_spell.ritual',
      text: 'Uma magia que invoca não é ritual: a invocação gasta um espaço de magia.',
    });
  }
  if (d.target !== 'self') {
    out.push({
      path: 'table_spell.target.kind',
      text: 'As criaturas aparecem ao lado de quem conjura: o alvo é “Só quem conjura”.',
    });
  }
  const applies = d.kept?.effect?.applies;
  if (
    applies !== undefined &&
    applies !== TableEffectApplies.UNSPECIFIED &&
    applies !== TableEffectApplies.ALL
  ) {
    out.push({
      path: 'table_spell.summon',
      text: 'Esta magia guarda um efeito só para quem falha no teste ou é atingido, e quem conjura sozinho não tem teste nem ataque. Escolha outra mecânica.',
    });
  }
  if (d.kept?.zone) {
    out.push({
      path: 'table_spell.summon',
      text: 'Esta magia guarda uma zona de área, e a invocação não aceita área. Escolha outra mecânica.',
    });
  }
  return out;
}

/** The inputs "Invocar" draws: where a refusal of the server at `table_spell.summon.*` lands (any other path under it goes to the group's note). */
export const SUMMON_FIELD_PATHS: readonly string[] = [
  'summon',
  'summon.creature_keys',
  'summon.count',
  'summon.count_per_slot_level',
  'summon.max_live',
  'summon.turn_mode',
  'summon.replaces',
];
