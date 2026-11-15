import type { DamagePartRoll } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { groupHolder, inTypedGroup } from '../../../../core/combat/damage-parts';

/** One row of the master's table of a rolled damage: a part on its own, the head of a typed group, or a part of it. */
export interface DamageRow {
  readonly kind: 'part' | 'group' | 'child';
  readonly key: string;
  readonly roll?: DamageRoll;
  readonly label: string;
  /** What the dice and the fixed number were ("1d6 + 3"), or the group's ("4d6 (10) + 3 · uma soma digitada"). */
  readonly formula: string;
  readonly total: number;
}

type DamageRoll = DamagePartRoll;

function upFirst(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function flatText(flat: number): string {
  return flat === 0 ? '' : ` ${flat < 0 ? '−' : '+'} ${Math.abs(flat)}`;
}

/** "4d6", or "1d8 + 1d6" when the faces differ. */
function diceOf(rolls: readonly DamageRoll[]): string {
  const bySides = new Map<number, number>();
  for (const r of rolls) {
    bySides.set(r.diceSides, (bySides.get(r.diceSides) ?? 0) + r.diceCount);
  }
  return [...bySides].map(([sides, count]) => `${count}d${sides}`).join(' + ');
}

/**
 * The rows of a typed group: its head says the group's dice, the one sum that was typed and the fixed numbers; under it
 * each part with its own dice and fixed number, the parts a take-out can reach with their button. `typeNames` maps a damage
 * type key to its Portuguese name ("perfurante").
 */
export function damageRows(
  rolls: readonly DamageRoll[],
  typeNames: ReadonlyMap<string, string>,
): DamageRow[] {
  const rows: DamageRow[] = [];
  const done = new Set<string>();
  for (const r of rolls) {
    if (!inTypedGroup(r, rolls)) {
      rows.push({ kind: 'part', key: r.partKey, roll: r, label: r.labelPt, formula: '', total: 0 });
      continue;
    }
    if (done.has(r.damageTypeKey)) {
      continue;
    }
    done.add(r.damageTypeKey);
    const members = rolls.filter(
      (o) => inTypedGroup(o, rolls) && o.damageTypeKey === r.damageTypeKey,
    );
    const counted = members.filter((o) => o.counted);
    const holder = counted.find((o) => !groupHolder(o, rolls));
    const sum = holder?.sum ?? 0;
    const flat = counted.reduce((n, o) => n + o.flat, 0);
    rows.push({
      kind: 'group',
      key: `group:${r.damageTypeKey}`,
      label: upFirst(typeNames.get(r.damageTypeKey) ?? 'Mesmo tipo'),
      formula: `${diceOf(counted)} (${sum})${flatText(flat)} · uma soma digitada`,
      total: sum + flat,
    });
    for (const m of members) {
      rows.push({
        kind: 'child',
        key: m.partKey,
        roll: m,
        label: m.labelPt,
        formula: `${diceOf([m])}${flatText(m.flat)}`,
        total: m.counted ? m.flat : 0,
      });
    }
  }
  return rows;
}
