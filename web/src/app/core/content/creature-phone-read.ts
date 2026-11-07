import type { Creature } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { TableCreature } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { typeAndSizeInWords } from '../creatures/bestiary-format';
import { joinDots } from '../format/text';
import type { EntryRead, ReadItem, ReadRow } from './content-read-parts';
import type { Carrier } from './creature-carriers';
import { creatureToDraft } from './creature-draft';
import { creaturePreview } from './creature-preview';
import { withOwner } from './creature-text';
import { SQUARE_FT, metersText } from '../units';

type NameOf = (key: string) => string;

/** The phone's rows are one line: the formula says "nível" and "proficiência", the table's words for the owner's. */
function short(text: string): string {
  return text.replace(/nível do dono/g, 'nível').replace(/bônus de proficiência/g, 'proficiência');
}

/** "Corpo a corpo, 1,5 m" or "À distância, 9 m e 18 m", from the feet the entry or the stat block holds. */
function shapeOf(a: {
  melee: boolean;
  reachFt: number;
  rangeFt: number;
  longRangeFt: number;
}): string {
  if (a.melee) {
    return `Corpo a corpo, ${metersText(a.reachFt > 0 ? a.reachFt : SQUARE_FT)}`;
  }
  const far = a.rangeFt > 0 ? metersText(a.rangeFt) : '';
  return `À distância${far ? `, ${far}` : ''}${a.longRangeFt > 0 ? ` e ${metersText(a.longRangeFt)}` : ''}`;
}

/** "Lobo (SRD), fera média": the base as the master reads it, the name alone until the stat block comes. */
function baseValue(base: Creature | null, baseName: string): string {
  const facts = base?.summary ? typeAndSizeInWords(base.summary) : '';
  return facts === '' ? `${baseName} (SRD)` : `${baseName} (SRD), ${facts}`;
}

/** "Trazida por Companheiro do Vale e Chamado da Matilha.", or nothing when no entry brings it. */
export function carriersNote(carriers: readonly Carrier[]): string {
  const names = [...new Set(carriers.map((c) => c.name))];
  if (names.length === 0) {
    return '';
  }
  const list = names.length < 2 ? names[0] : `${names.slice(0, -1).join(', ')} e ${names.at(-1)}`;
  return `Trazida por ${list}.`;
}

/**
 * A table creature as the master reads it on a phone (#578): the base, the numbers (the base's where the table sets none), the
 * damage lines with where each comes from, then the traits and the attacks in one card with the entries that bring it. Built
 * from the same preview as the editor's side panel, so the two say the same. Only the master gets this: the damage lines and
 * the carriers are not the players'.
 */
export function readCreatureForMaster(
  c: TableCreature,
  base: Creature | null,
  baseName: string,
  nameOf: NameOf,
  carriers: readonly Carrier[],
): EntryRead {
  const draft = creatureToDraft(c);
  const preview = creaturePreview(draft, base, baseName, nameOf);
  const order = ['PV', 'CA', 'Bônus no dano'];
  const numbers = order.flatMap((o) => preview.rows.filter((r) => r.label === o));
  const attackBonus = withOwner(draft.attackBonusFormula);
  const mods = preview.mods.map((l) => ({
    label: l.kind,
    value: l.parts.map((p) => `${p.text.replace(/^a /, '')} (${p.origin})`).join('; '),
  }));
  const rows: ReadRow[] = [
    { label: 'Base', value: baseValue(base, baseName) },
    ...numbers,
    ...(attackBonus === '' ? [] : [{ label: 'Bônus de ataque', value: attackBonus }]),
    ...mods,
  ].map((r) => ({ label: r.label, value: short(r.value) }));
  // `creaturePreview` lists the base's attacks first, in the stat block's order, then the table's own, in the entry's order.
  const baseAttacks = (base?.actions ?? []).filter((a) => a.hasAttack);
  const attacks: ReadItem[] = preview.actions.map((a, i) => {
    const from = i < baseAttacks.length ? baseAttacks[i] : c.attacks[i - baseAttacks.length];
    const detail = a.origin === 'da mesa' ? a.detail.replace(/^\+0\s·\s/, '') : a.detail;
    const title = a.origin === 'da mesa' ? a.name : `${a.name} (${a.origin})`;
    return { title, text: `${joinDots([shapeOf(from), detail])}.` };
  });
  const traits: ReadItem[] = c.traits.map((t) => ({ title: t.namePt, text: t.textPt }));
  const note = carriersNote(carriers);
  const items = [...traits, ...attacks];
  return {
    rows,
    rowsNote:
      mods.length > 0 ? 'Só você muda estas linhas. Quem tem a criatura as lê na ficha dela.' : '',
    sections:
      items.length > 0 || note !== ''
        ? [{ title: '', label: 'Traços e ataques', items, note }]
        : [],
    text: [],
  };
}
