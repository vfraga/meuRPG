import type { TableEffect } from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { EffectDraft, EffectInit } from './effect-draft.types';
import { signed } from '../creatures/creature-format';
import { outsideTheBlock } from './power-attack-limits';

/** The stored effect a draft carries untouched, or `undefined` when the editor's block can show it: the draft's other fields are then
 * the editor's own and the effect is written from them. A power_attack with a number outside the range of its block, a weapon kind that
 * is not one of the three or a filter the block cannot hold is kept exactly as stored: shown read-only, written back untouched, never
 * rewritten into something the block would make of it. */
export function keptOf(e: TableEffect): TableEffect | undefined {
  return e.type === 'power_attack' && outsideTheBlock(e) ? e : undefined;
}

/** What a kept effect sends: the stored effect with only the fields that hold something, as a request carries any effect. */
export function keptToInit(kept: TableEffect): EffectInit {
  const out: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(kept)) {
    const empty =
      key.startsWith('$') ||
      value === '' ||
      value === 0 ||
      value === false ||
      value === undefined ||
      (Array.isArray(value) && value.length === 0);
    if (!empty) {
      out[key] = Array.isArray(value) ? [...(value as unknown[])] : value;
    }
  }
  return out as EffectInit;
}

/** The numbers of a kept effect, as the lines the editor shows in place of its fields: [label, text]. A closed-list key is
 * shown by `nameOf` (the Portuguese name the menu gives it, the key when it does not know it). */
export function keptLines(
  d: EffectDraft,
  nameOf: (list: string, key: string) => string,
): readonly (readonly [string, string])[] {
  const k = d.kept;
  if (!k) {
    return [];
  }
  const lines: [string, string][] = [
    ['Penalidade no ataque', signed(-k.attackPenalty)],
    ['Bônus no dano', signed(k.damageBonus)],
    ['Arma', WEAPON_PT[k.weapon === '' ? 'any' : k.weapon] ?? k.weapon],
  ];
  if (k.weaponProperties.length > 0) {
    lines.push([
      'Propriedades da arma',
      k.weaponProperties.map((p) => nameOf('weapon_properties', p)).join(', '),
    ]);
  }
  lines.push(['Só com arma proficiente', k.onlyProficient ? 'sim' : 'não']);
  if (k.resource !== '') {
    lines.push(['Recurso gasto', k.resource]);
  }
  return lines;
}

const WEAPON_PT: Readonly<Record<string, string>> = {
  any: 'qualquer',
  melee: 'corpo a corpo',
  ranged: 'à distância',
};

/** The line that says why nothing of a kept effect is editable. */
export const KEPT_NOTE =
  'Este efeito tem valores fora do que o editor aceita, e o editor não muda o que não entende. Ele é salvo como está; só dá para removê-lo.';

/** What the picker draws for a kept effect: its lines and the note, or `null` for an effect the editor writes. */
export function keptView(d: EffectDraft, menu: { nameOf(list: string, key: string): string }) {
  return d.kept ? { rows: keptLines(d, (l, k) => menu.nameOf(l, k)), note: KEPT_NOTE } : null;
}
