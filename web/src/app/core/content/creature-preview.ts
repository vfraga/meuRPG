import type { Creature } from '../../../gen/meurpg/rules/v1/rules_pb';
import { signed, speedsText } from '../creatures/creature-format';
import { typeAndSize } from '../creatures/bestiary-format';
import { joinDots, tieNumbers } from '../format/text';
import { type ModLine, previewMods } from './creature-damage-mods';
import { type CreatureDraft } from './creature-draft';
import { hasFormula, numberValue, signedBonus, withOwner } from './creature-text';
import { readWhole } from './whole-number';

type NameOf = (key: string) => string;

/**
 * "Como fica na mesa" (#578): the creature as the table reads it without an owner, written from the draft and the base's stat
 * block. A pure function of both, so it is tested without a screen.
 */

/** "2d4+2" and 1 is "2d4 + 3": the dice of an attack with the creature's fixed damage bonus (a flat damage stops at 0; "+MOD" is left as it is). */
export function diceWithBonus(dice: string, bonus: number): string {
  const t = dice.replace(/\s+/g, '');
  if (/^\d+$/.test(t)) {
    return String(Math.max(Number(t) + bonus, 0));
  }
  const m = /^(\d+)d(\d+)([+-]\d+)?$/.exec(t);
  if (!m) {
    return spaced(dice);
  }
  const total = Number(m[3] ?? 0) + bonus;
  return `${m[1]}d${m[2]}${total > 0 ? ` + ${total}` : total < 0 ? ` − ${-total}` : ''}`;
}

function spaced(dice: string): string {
  return dice
    .replace(/([+-])/g, ' $1 ')
    .replace(/\s+/g, ' ')
    .trim();
}

export interface PreviewAction {
  readonly name: string;
  readonly origin: 'da base' | 'da mesa';
  readonly detail: string;
}

interface PreviewRow {
  readonly label: string;
  readonly value: string;
}

export interface CreaturePreview {
  readonly name: string;
  /** "Fera média · base Lobo · CA 13 · PV 11 · 12 m". */
  readonly line: string;
  readonly rows: readonly PreviewRow[];
  /** What the line cannot say: a number that follows the owner's formula, and the fixed damage bonus. */
  readonly notes: readonly PreviewRow[];
  /** The attacks without an owner, the base's then the table's own. */
  readonly actions: readonly PreviewAction[];
  /** "Resistências e imunidades": the table's lines and the base's, one line for each kind. */
  readonly mods: readonly ModLine[];
  readonly traits: readonly string[];
  /** Said when a formula makes the numbers of an owner differ from these. */
  readonly ownerNote: string;
}

const present = (parts: readonly string[]): string => joinDots(parts.filter((x) => x));

function numberRow(
  label: string,
  fixed: string,
  baseValue: number | null,
  formula: string,
): PreviewRow | null {
  const n = readWhole(fixed, false);
  const base = baseValue === null ? '' : String(baseValue);
  const value = numberValue(!Number.isNaN(n) && n > 0 ? String(n) : base, formula);
  return value === '' ? null : { label, value: tieNumbers(value) };
}

const DOT = /\s·\s/;

/** "CA 13" for a row that may go on to say the owner's formula, and "12 m" for the speed. */
function briefOf(r: PreviewRow): string {
  return r.label === 'Deslocamento' ? r.value : `${r.label} ${r.value.split(DOT)[0]}`;
}

function previewRows(d: CreatureDraft, base: Creature | null, fixedBonus: number): PreviewRow[] {
  const rows = [
    numberRow('CA', d.armorClass, base?.armorClass ?? null, d.armorClassFormula),
    numberRow('PV', d.hitPoints, base?.hitPoints ?? null, d.hitPointsFormula),
  ].filter((r) => r !== null);
  if (base) {
    rows.push({ label: 'Deslocamento', value: speedsText(base) });
  }
  const bonus = present([signedBonus(fixedBonus), withOwner(d.damageBonusFormula)]);
  if (bonus !== '') {
    rows.push({ label: 'Bônus no dano', value: bonus });
  }
  return rows;
}

/** The attacks of the base's stat block, each with the creature's fixed damage bonus on its first damage. */
function baseActions(base: Creature | null, fixedBonus: number): PreviewAction[] {
  return (base?.actions ?? [])
    .filter((a) => a.hasAttack)
    .map((a) => {
      const parts = a.damage.map((p, i) =>
        `${i === 0 ? diceWithBonus(p.dice, fixedBonus) : spaced(p.dice)} ${p.damageTypePt}`.trim(),
      );
      return {
        name: a.namePt || a.name,
        origin: 'da base',
        detail: present([signed(a.attackBonus), parts.join(' + ')]),
      };
    });
}

/** The attacks the table adds: without an owner they hit with +0 and add the fixed damage bonus. */
function tableActions(d: CreatureDraft, fixedBonus: number, nameOf: NameOf): PreviewAction[] {
  return d.attacks.map((a) => {
    const count = a.diceCount.trim();
    const dice =
      count !== '' && a.diceSides !== ''
        ? diceWithBonus(`${count}d${a.diceSides}`, fixedBonus)
        : '';
    const type = a.damageType === '' ? '' : nameOf(a.damageType).toLowerCase();
    return {
      name: a.name.trim() || 'Ataque sem nome',
      origin: 'da mesa',
      detail: present(['+0', [dice, type].filter((x) => x).join(' ')]),
    };
  });
}

function hasOwnerFormula(d: CreatureDraft): boolean {
  return hasFormula([
    d.hitPointsFormula,
    d.armorClassFormula,
    d.damageBonusFormula,
    d.attackBonusFormula,
    ...d.attacks.flatMap((a) => [a.toHit, a.damageBonus]),
  ]);
}

/** "Como fica na mesa": the creature as the table reads it, from the draft and the base's stat block (`null` until it is read). */
export function creaturePreview(
  d: CreatureDraft,
  base: Creature | null,
  baseName: string,
  nameOf: NameOf,
): CreaturePreview {
  const typed = readWhole(d.damageBonus, true);
  const fixedBonus = Number.isNaN(typed) ? 0 : typed;
  const rows = previewRows(d, base, fixedBonus);
  return {
    name: d.name.trim() || 'Nova criatura',
    line: present([
      base?.summary ? typeAndSize(base.summary) : '',
      baseName ? `base ${baseName}` : '',
      ...rows.filter((r) => r.label !== 'Bônus no dano').map(briefOf),
    ]),
    rows,
    notes: rows.filter((r) => r.label === 'Bônus no dano' || DOT.test(r.value)),
    actions: [...baseActions(base, fixedBonus), ...tableActions(d, fixedBonus, nameOf)],
    mods: previewMods(d.damageMods, base, nameOf),
    traits: d.traits.map((t) => t.name.trim()).filter((n) => n !== ''),
    ownerNote: hasOwnerFormula(d) ? 'Com dono, o acerto e o dano seguem as fórmulas.' : '',
  };
}
