import type { Creature } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { TableCreatureDamageMod } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { newRowId } from './effect-draft';

/**
 * The resistances, immunities and vulnerabilities of a table creature (#585): up to 12 rows, each a kind, the damage types it names and
 * a "Vale contra" qualifier (only for a resistance or an immunity, only for bludgeoning, piercing and slashing). They add to the
 * base's lists and never take one away. Pure functions of the draft, so the words are tested without a screen.
 */

export type DamageModKind = 'resist' | 'immune' | 'vulnerable';

/** The most rows one creature has (the server's MaxCreatureDamageMods). */
export const MAX_DAMAGE_MODS = 12;

export interface DamageModDraft {
  /** A stable id the editor tracks the row by (never sent). */
  id: string;
  kind: string;
  /** "damage-type:" keys, in the order they were added. */
  types: string[];
  /** "", "nonmagical", "nonmagical_unless_silvered" or "nonmagical_unless_adamantine". */
  qualifier: string;
}

export function emptyDamageMod(): DamageModDraft {
  return { id: newRowId(), kind: 'resist', types: [], qualifier: '' };
}

export function modToDraft(m: TableCreatureDamageMod): DamageModDraft {
  return { id: newRowId(), kind: m.kind, types: [...m.damageTypes], qualifier: m.qualifier };
}

export const MOD_KINDS: readonly { value: string; label: string; icon: string }[] = [
  { value: 'resist', label: 'Resistência', icon: 'shield' },
  { value: 'immune', label: 'Imunidade', icon: 'verified_user' },
  { value: 'vulnerable', label: 'Vulnerabilidade', icon: 'gpp_maybe' },
];

export const QUALIFIERS: readonly { value: string; label: string; short: string }[] = [
  { value: '', label: 'Todo dano desses tipos', short: '' },
  { value: 'nonmagical', label: 'Ataque não mágico', short: 'de ataque não mágico' },
  {
    value: 'nonmagical_unless_silvered',
    label: 'Ataque não mágico nem prateado',
    short: 'de ataque não mágico nem prateado',
  },
  {
    value: 'nonmagical_unless_adamantine',
    label: 'Ataque não mágico nem adamantino',
    short: 'de ataque não mágico nem adamantino',
  },
];

const PHYSICAL = ['damage-type:bludgeoning', 'damage-type:piercing', 'damage-type:slashing'];

/** Why "Vale contra" is off for a row, or nothing when it is open: a vulnerability has none, and neither has a damage type that is not physical. */
export function qualifierWhy(kind: string, types: readonly string[]): string {
  return kind !== 'vulnerable' && types.every((t) => PHYSICAL.includes(t))
    ? ''
    : 'Só resistência ou imunidade a concussão, perfurante e cortante pode ter outro "Vale contra".';
}

/** "concussão, perfurante e cortante". */
export function listWords(words: readonly string[]): string {
  return words.length < 2
    ? words.join('')
    : `${words.slice(0, -1).join(', ')} e ${words[words.length - 1]}`;
}

/** The name of a kind; a kind this build does not know (an entry from a newer one) is shown as it is, never as nothing. */
export const kindLabel = (kind: string): string =>
  MOD_KINDS.find((k) => k.value === kind)?.label ?? (kind || 'Linha sem tipo');
const shortOf = (q: string): string => QUALIFIERS.find((x) => x.value === q)?.short ?? '';

type NameOf = (key: string) => string;

/** The line of a collapsed row: "Resistência a concussão, perfurante e cortante". */
export function modTitle(m: DamageModDraft, nameOf: NameOf): string {
  const types = listWords(m.types.map((t) => nameOf(t).toLowerCase()));
  return types === '' ? kindLabel(m.kind) : `${kindLabel(m.kind)} a ${types}`;
}

/** The line under it: "Todo dano desse tipo" or "de ataque não mágico nem prateado". */
export function modSub(m: DamageModDraft): string {
  return (
    shortOf(m.qualifier) || (m.types.length > 1 ? 'Todo dano desses tipos' : 'Todo dano desse tipo')
  );
}

/** "Necrótico está duas vezes. Tire o repetido.": the first damage type a row holds twice, or '' when it holds none twice. */
export function duplicateText(types: readonly string[], nameOf: NameOf): string {
  const twice = types.find((t, i) => types.indexOf(t) !== i);
  if (twice === undefined) {
    return '';
  }
  const name = nameOf(twice);
  return `${name.charAt(0).toUpperCase()}${name.slice(1)} está duas vezes. Tire o repetido.`;
}

/** One part of a line of the preview: "a necrótico" and where it comes from. */
interface ModPart {
  readonly text: string;
  readonly origin: 'da mesa' | 'da base';
}

export interface ModLine {
  readonly kind: string;
  readonly parts: readonly ModPart[];
}

/** What the base's stat block says in one of its lists, as the preview writes it ("a veneno"); the book's own note stays as written. */
function baseParts(list: Creature['resistances']): ModPart[] {
  return list.map((m) => {
    const types = listWords(m.types.map((t) => t.namePt.toLowerCase()));
    const text = types === '' ? m.note : `a ${types}${m.note ? ` ${m.note}` : ''}`;
    return { text, origin: 'da base' };
  });
}

/** "Resistências e imunidades" of the preview: the lines of the table first, then the base's, one line for each kind. */
export function previewMods(
  mods: readonly DamageModDraft[],
  base: Creature | null,
  nameOf: NameOf,
): readonly ModLine[] {
  const fromBase: Record<string, ModPart[]> = {
    resist: base ? baseParts(base.resistances) : [],
    immune: base ? baseParts(base.immunities) : [],
    vulnerable: base ? baseParts(base.vulnerabilities) : [],
  };
  return (['resist', 'vulnerable', 'immune'] as const)
    .map((kind) => {
      const own = mods
        .filter((m) => m.kind === kind && m.types.length > 0)
        .map((m) => {
          const types = listWords(m.types.map((t) => nameOf(t).toLowerCase()));
          return { text: `a ${types} ${shortOf(m.qualifier)}`.trim(), origin: 'da mesa' as const };
        });
      return { kind: kindLabel(kind), parts: [...own, ...fromBase[kind]] };
    })
    .filter((l) => l.parts.length > 0);
}

/** "imunidade a veneno; resistência a fogo e gelo": what the base's stat block already says, for the line over the rows. Empty for none. */
export function baseModsText(base: Creature | null): string {
  return previewMods([], base, (k) => k)
    .map((l) => `${l.kind.toLowerCase()} ${l.parts.map((p) => p.text).join(' e ')}`)
    .join('; ');
}
