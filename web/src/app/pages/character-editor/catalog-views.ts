import { abilityLabel } from '../../core/characters/character-labels';
import type { CasterSection } from './class-blocks';
import type {
  BackgroundOptionVm,
  ClassOptionVm,
  RaceOptionVm,
  RulesCatalogVm,
  SubraceOptionVm,
  WeaponOptionVm,
} from './character-editor.types';
import { offered } from './class-blocks';

/** What the editor writes about the entries of the server's catalog: the sentences under a select, the name a
 * message gives an entry, the Constitution the preview adds up, and the skills the lists give. */

const BONUS_LIST = new Intl.ListFormat('pt-BR', { type: 'conjunction' });

/** "+2 e +1". */
export function bonusWords(bonuses: readonly number[]): string {
  return BONUS_LIST.format(bonuses.map((b) => `+${b}`));
}

/** The Portuguese name of any entry of the catalog, for a message that names one (never the key itself). */
export function catalogNameOf(
  catalog: RulesCatalogVm | undefined,
  key: string,
): string | undefined {
  if (!catalog) {
    return undefined;
  }
  return [
    ...catalog.races,
    ...catalog.classes,
    ...catalog.backgrounds,
    ...catalog.spells,
    ...catalog.races.flatMap((r) => r.subraces),
    ...catalog.classes.flatMap((k) => k.subclasses),
  ].find((e) => e.key === key)?.namePt;
}

/** What the first class asks of the skills and the saving throws, from the server's entry for it. */
export function skillNoteOf(c: ClassOptionVm | undefined): string {
  if (!c || c.skillChoose <= 0) {
    return '';
  }
  const saves = c.savingThrows.map((a) => abilityLabel(a));
  const savesText =
    saves.length > 0
      ? ` e dá proficiência nos testes de resistência de ${BONUS_LIST.format(saves)}`
      : '';
  return `O ${c.namePt} escolhe ${c.skillChoose} ${c.skillChoose === 1 ? 'perícia' : 'perícias'} ao começar${savesText}.`;
}

/** The race's "+2 e +1 à sua escolha" (or one of several sets of them): where the player puts them (the server's numbers,
 * from the catalog). A subrace that swaps the race's bonuses takes them away. */
export function choiceBonusesHintOf(
  race: RaceOptionVm | undefined,
  subrace?: SubraceOptionVm,
): string {
  if (subrace?.replacesAbilityBonuses) {
    return '';
  }
  const sets = race?.choiceBonusSets ?? [];
  if (sets.length > 0) {
    return `Dá um destes grupos, nas habilidades que você escolher (uma por valor): ${sets.map((s) => bonusWords(s)).join(' ou ')}. Escolha no passo Escolhas.`;
  }
  const bonuses = race?.choiceBonuses ?? [];
  return bonuses.length > 0
    ? `Dá ${bonusWords(bonuses)} nas habilidades que você escolher, uma diferente para cada valor: escolha no passo Escolhas.`
    : '';
}

const SIZE_NAMES: Readonly<Record<string, string>> = {
  Tiny: 'Miúdo',
  Small: 'Pequeno',
  Medium: 'Médio',
  Large: 'Grande',
};

/** The sizes the player picks one from, for a race that offers two or more; none for a race with only its own size, so the
 * step shows the picker only when there is a real choice. The empty value is the race's own size. */
export function sizeChoicesOf(
  race: RaceOptionVm | undefined,
): readonly { value: string; label: string }[] {
  const sizes = race?.sizes ?? [];
  if (sizes.length < 2) {
    return [];
  }
  const own = race?.size ?? sizes[0];
  return [
    { value: '', label: `${SIZE_NAMES[own] ?? own} (da raça)` },
    ...sizes.filter((s) => s !== own).map((s) => ({ value: s, label: SIZE_NAMES[s] ?? s })),
  ];
}

/** The hint under "Subclasse" while the level is below the one where the class chooses it; empty once the level
 * reaches it. */
export function subclassHintOf(c: ClassOptionVm | undefined, level: number): string {
  return c && c.subclassLevel > 0 && !(level >= c.subclassLevel)
    ? `O ${c.namePt} escolhe a subclasse no nível ${c.subclassLevel}.`
    : '';
}

/** The line next to the counter of the Escolhas step: "Tharn · Guerreiro 1 · Draconato". */
export function choicesSubtitleOf(
  name: string,
  cls: ClassOptionVm | undefined,
  level: number,
  race: RaceOptionVm | undefined,
  subrace: SubraceOptionVm | undefined,
): string {
  return [name.trim(), cls ? `${cls.namePt} ${level}` : '', subrace?.namePt ?? race?.namePt ?? '']
    .filter((p) => p !== '')
    .join(' · ');
}

/** The Constitution the rolled hit points use: the typed score plus what the race and subrace give plus the manual
 * bonus (the same sum the server makes; this one only feeds the preview). A subrace that swaps the race's bonuses
 * leaves out the race's. */
export function finalConstitutionOf(
  typed: number | undefined,
  race: RaceOptionVm | undefined,
  subrace: SubraceOptionVm | undefined,
  manual: number | undefined,
): number {
  return (
    (typed ?? 0) +
    (subrace?.replacesAbilityBonuses ? 0 : (race?.constitutionBonus ?? 0)) +
    (subrace?.constitutionBonus ?? 0) +
    (manual ?? 0)
  );
}

/** The skills the race, the subrace and the background give, with where each comes from ("da raça", "do
 * antecedente"). The server counts them as given, never as a pick: the step shows them taken and locked, and a
 * pick on one of them is dropped, as the rules say a repeated proficiency is traded for another of the same
 * kind. The race's skills a subrace replaces are not given. */
export function grantedSkillsOf(
  race: RaceOptionVm | undefined,
  subrace: SubraceOptionVm | undefined,
  background: BackgroundOptionVm | undefined,
  customBackground: boolean,
  customSkills: readonly string[],
): ReadonlyMap<string, string> {
  const granted = new Map<string, string>();
  const give = (keys: readonly string[] | undefined, source: string) => {
    for (const key of keys ?? []) {
      if (!granted.has(key)) {
        granted.set(key, source);
      }
    }
  };
  const replaced = new Set(subrace?.replacedSkillKeys ?? []);
  give(
    (race?.skillKeys ?? []).filter((k) => !replaced.has(k)),
    'da raça',
  );
  give(subrace?.skillKeys, 'da raça');
  if (customBackground) {
    give(customSkills, 'do antecedente');
  } else {
    give(background?.skillKeys, 'do antecedente');
  }
  return granted;
}

/** The skills the player picked, without the ones already given. */
export function chosenSkillsOf(
  selected: ReadonlySet<string>,
  granted: ReadonlyMap<string, string>,
): Set<string> {
  return new Set(Array.from(selected).filter((k) => !granted.has(k)));
}

/** The heading of a section when there is more than one: "Clérigo · 1º nível". */
export function sectionHeadingOf(section: CasterSection): string {
  const name = section.subclassNamePt
    ? `${section.namePt} · ${section.subclassNamePt}`
    : section.namePt;
  const max = section.maxCircle;
  if (max === null) {
    return name;
  }
  return max === 0 ? `${name} · só truques` : `${name} · até o ${max}º nível de magia`;
}

/** The subclass of a one-class sheet waits for the level the class chooses it at (as in a block), unless one is chosen. */
export function subclassWaits(
  c: ClassOptionVm | undefined,
  level: number,
  subclassKey: string,
): boolean {
  return !!c && c.subclassLevel > 0 && level < c.subclassLevel && subclassKey === '';
}

/** What a weapon's row says at its right: "Simples · 1d6", or the category alone for a weapon with no damage (board 580-16). */
export function weaponDetail(w: WeaponOptionVm): string {
  const category = w.category === 'martial' ? 'Marcial' : 'Simples';
  return w.damage ? `${category} · ${w.damage}` : category;
}

/** The weapons a sheet may carry, the book's and the table's apart (#580): a table weapon that is archived is offered to nobody new,
 * and one switched off for the players only to the master; a weapon the sheet already carries stays on the list either way. */
export function weaponGroups(
  weapons: readonly WeaponOptionVm[],
  carried: readonly string[],
  master: boolean,
): { book: WeaponOptionVm[]; table: WeaponOptionVm[] } {
  const shown = offered(weapons, carried, master);
  return { book: shown.filter((x) => !x.fromTable), table: shown.filter((x) => x.fromTable) };
}
