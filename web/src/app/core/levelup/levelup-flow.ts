import {
  LevelUpProficiencyKind,
  LevelUpSpellsKind,
  type LevelUpFeatureChoice,
  type LevelUpOptions,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { type Skill, type Spell } from '../../../gen/meurpg/rules/v1/rules_pb';
import { abilityKeyOf } from '../characters/ability-gen';
import { abilityLabel, spellLevelLabel } from '../characters/character-labels';
import { isTableKey } from '../content/catalog';
import { joinDots } from '../format/text';

/**
 * The pure half of the guided level-up (MR-040, RN-12): which steps a level
 * has, how many of each choice it asks, and what the pickers list. The counts
 * and the lists come from the server (`GetLevelUpOptions`, the campaign's
 * content); the browser only counts what was picked against them. Every
 * number on screen that is a rule comes from `PreviewLevelUp` (ADR-0008).
 */

/** The steps, in the order Vinicius ruled: Habilidades, Vida, Magias, Resumo;
 * "Escolhas" (subclass, feature options, skills, expertise) is the one extra,
 * and sits before Magias because a subclass can add cantrips. */
export type StepKey = 'class' | 'abilities' | 'hp' | 'picks' | 'spells' | 'summary';

export const STEP_LABELS: Record<StepKey, string> = {
  class: 'Classe',
  abilities: 'Habilidades',
  hp: 'Vida',
  picks: 'Escolhas',
  spells: 'Magias',
  summary: 'Resumo',
};

/** How many of each choice the level asks, with the chosen subclass's share. */
export interface Totals {
  readonly cantrips: number;
  readonly spells: number;
  readonly skills: number;
  readonly expertise: number;
  readonly featureChoices: readonly LevelUpFeatureChoice[];
}

/** The level's counts once `subclassKey` (empty before it is picked) adds its own. */
export function totalsFor(o: LevelUpOptions, subclassKey: string): Totals {
  const sub = o.subclasses.find((s) => s.key === subclassKey);
  return {
    cantrips: o.cantrips + (sub?.cantrips ?? 0),
    spells: o.spells + (sub?.spells ?? 0),
    skills:
      o.skillChoices + (sub?.skillChoices ?? 0) + proficiencyPick(o, LevelUpProficiencyKind.SKILL),
    expertise: o.expertiseChoices + (sub?.expertiseChoices ?? 0),
    featureChoices: [...o.featureChoices, ...(sub?.featureChoices ?? [])],
  };
}

/**
 * The level's options once `subclassKey` is picked: a third caster's subclass (the table's, or a
 * fighter's or rogue's) brings its own spells, the list they come from, the highest circle and
 * whether it prepares (slice 10.3's `LevelUpSubclass` fields 8 to 13). Before the pick, or for a
 * subclass that casts nothing, the options are the server's own, untouched. The browser only picks
 * which of the server's numbers to read; none is worked out.
 */
export function withSubclass(o: LevelUpOptions, subclassKey: string): LevelUpOptions {
  const sub = o.subclasses.find((s) => s.key === subclassKey);
  if (!sub || (sub.spells === 0 && !sub.spellListClassKey && !sub.prepares)) {
    return o;
  }
  return {
    ...o,
    spells: o.spells + sub.spells,
    spellsKind: sub.spellsKind || o.spellsKind,
    spellListClassKey: sub.spellListClassKey || o.spellListClassKey,
    maxSpellLevel: sub.maxSpellLevel || o.maxSpellLevel,
    prepares: o.prepares || sub.prepares,
    preparedMaxAfter: sub.prepares ? sub.preparedMaxAfter : o.preparedMaxAfter,
  };
}

/** How many more spells to prepare: the new maximum less what is prepared today. */
export function preparedMore(o: LevelUpOptions, maxAfter: number, preparedNow: number): number {
  return o.prepares ? Math.max(0, maxAfter - preparedNow) : 0;
}

/** How many of a multiclass table's picks of this kind a new class asks (a skill, the Bard's instrument). */
export function proficiencyPick(o: LevelUpOptions, kind: LevelUpProficiencyKind): number {
  return o.proficiencyChoices.find((c) => c.kind === kind)?.count ?? 0;
}

/** The steps this level has: one with nothing to choose does not exist. Classe and Vida never go
 * ("Subir em qual classe?" always opens the flow). */
export function stepsFor(o: LevelUpOptions, t: Totals, more: number): StepKey[] {
  const steps: StepKey[] = ['class'];
  if (o.abilityScoreImprovement) {
    steps.push('abilities');
  }
  steps.push('hp');
  if (
    o.subclassDue ||
    t.featureChoices.length > 0 ||
    t.skills > 0 ||
    t.expertise > 0 ||
    o.lateChoices.length > 0 ||
    o.newChoices.length > 0 ||
    proficiencyPick(o, LevelUpProficiencyKind.INSTRUMENT) > 0 ||
    // Without the ability step, "Trocar uma escolha" (a table feature's swap) is one more panel of this step.
    (!o.abilityScoreImprovement && o.swapGroups.length > 0)
  ) {
    steps.push('picks');
  }
  if (t.cantrips > 0 || t.spells > 0 || more > 0) {
    steps.push('spells');
  }
  steps.push('summary');
  return steps;
}

/** One row of a picker: a content key, its Portuguese name and the small line under it. */
export interface PickItem {
  readonly key: string;
  readonly name: string;
  readonly sub: string;
  /** A spell off the class's own list (a Bard's Magical Secrets). */
  readonly outside?: boolean;
  /** The master's own spell ("Da mesa"). */
  readonly table?: boolean;
  /** Why the row cannot be picked now ("Limite de 2 de outra classe"); empty or unset when it can. */
  readonly disabled?: string;
}

const COLLATOR = new Intl.Collator('pt-BR');

function byLevelThenName(a: Spell, b: Spell): number {
  return a.level - b.level || COLLATOR.compare(a.namePt, b.namePt);
}

/** "2º nível · Conjuração", the way a spell row reads; extras follow ("ritual"). */
export function spellSub(spell: Spell, ...extras: string[]): string {
  return joinDots(
    [spellLevelLabel(spell.level), spell.schoolNamePt, ...extras].filter((s) => s !== ''),
  );
}

function toItem(spell: Spell, ...extras: string[]): PickItem {
  return {
    key: spell.key,
    name: spell.namePt,
    sub: spellSub(spell, ...extras),
    ...(isTableKey(spell.key) ? { table: true } : {}),
  };
}

/** What the sheet has today, by content key: the pickers never offer it twice. */
export interface SheetKeys {
  readonly cantrips: readonly string[];
  readonly known: readonly string[];
  readonly prepared: readonly string[];
  readonly skills: readonly string[];
  readonly expertise: readonly string[];
}

/** The class's cantrips the character does not know yet. */
export function cantripOptions(
  o: LevelUpOptions,
  spells: readonly Spell[],
  have: SheetKeys,
): PickItem[] {
  const owned = new Set(have.cantrips);
  return spells
    .filter((s) => s.level === 0 && s.classKeys.includes(o.spellListClassKey) && !owned.has(s.key))
    .sort(byLevelThenName)
    .map((s) => toItem(s));
}

/** The new spells for the book or the spells known: the class's list, up to the
 * highest circle it casts, minus what is known. A Bard's Magical Secrets takes any
 * class's list (the server checks how many may come from outside). */
export function spellOptions(
  o: LevelUpOptions,
  spells: readonly Spell[],
  have: SheetKeys,
): PickItem[] {
  const owned = new Set(have.known);
  return spells
    .filter(
      (s) =>
        s.level >= 1 &&
        s.level <= o.maxSpellLevel &&
        !owned.has(s.key) &&
        (o.anyClassSpells > 0 || s.classKeys.includes(o.spellListClassKey)),
    )
    .sort(byLevelThenName)
    .map((s) => {
      const outside = !s.classKeys.includes(o.spellListClassKey);
      return { ...toItem(s, ...(outside ? ['de outra classe'] : [])), outside };
    });
}

/** The spells that can be prepared in the new slots: the spellbook's (with the new
 * ones just picked) for a Wizard, the class's list for the others. */
export function preparedOptions(
  o: LevelUpOptions,
  spells: readonly Spell[],
  have: SheetKeys,
  newSpells: ReadonlySet<string>,
): PickItem[] {
  const prepared = new Set(have.prepared);
  const book = new Set([...have.known, ...newSpells]);
  const fromBook = o.spellsKind === LevelUpSpellsKind.SPELLBOOK;
  return spells
    .filter(
      (s) =>
        s.level >= 1 &&
        s.level <= o.maxSpellLevel &&
        !prepared.has(s.key) &&
        (fromBook ? book.has(s.key) : s.classKeys.includes(o.spellListClassKey)),
    )
    .sort(byLevelThenName)
    .map((s) =>
      toItem(
        s,
        ...[newSpells.has(s.key) ? 'nova no livro' : '', s.ritual ? 'ritual' : ''].filter(
          (e) => e !== '',
        ),
      ),
    );
}

function skillItem(skill: Skill): PickItem {
  const key = abilityKeyOf(skill.ability);
  return { key: skill.key, name: skill.namePt, sub: key ? abilityLabel(key) : '' };
}

/** New skills: any the character is not trained in yet. A class taken as a later class gives its own list, in which the
 * skills the character already has stay, turned off ("Você já tem"). */
export function skillOptions(
  skills: readonly Skill[],
  have: SheetKeys,
  o?: LevelUpOptions,
): PickItem[] {
  const owned = new Set(have.skills);
  const pick = o?.proficiencyChoices.find((c) => c.kind === LevelUpProficiencyKind.SKILL);
  if (pick) {
    const from = new Map(pick.from.map((f) => [f.key, f.alreadyHave]));
    return skills
      .filter((s) => from.has(s.key))
      .map((s) =>
        from.get(s.key) || owned.has(s.key)
          ? { ...skillItem(s), disabled: 'Você já tem' }
          : skillItem(s),
      )
      .sort((a, b) => COLLATOR.compare(a.name, b.name));
  }
  return skills
    .filter((s) => !owned.has(s.key))
    .map(skillItem)
    .sort((a, b) => COLLATOR.compare(a.name, b.name));
}

/** New expertise: a skill the character is trained in (today, or picked just now) and has no expertise in. */
export function expertiseOptions(
  skills: readonly Skill[],
  have: SheetKeys,
  newSkills: ReadonlySet<string>,
): PickItem[] {
  const trained = new Set([...have.skills, ...newSkills]);
  const done = new Set(have.expertise);
  return skills
    .filter((s) => trained.has(s.key) && !done.has(s.key))
    .map(skillItem)
    .sort((a, b) => COLLATOR.compare(a.name, b.name));
}

/** "Falta escolher 1 magia." / "Faltam escolher 2 magias.", the one sentence a step's footer shows. */
export function needText(n: number, one: string, many: string, verb = 'escolher'): string {
  return n === 1 ? `Falta ${verb} 1 ${one}.` : `Faltam ${verb} ${n} ${many}.`;
}

/** What is still missing in a step: the reason, and which picker holds it (its id, for the focus). */
export interface Missing {
  readonly step: StepKey;
  readonly id: string;
  readonly text: string;
}

/** What the level-up page leaves for the sheet in the navigation state (never Web Storage): "Pensantus subiu para o nível 4." */
export interface LevelUpDone {
  readonly name: string;
  readonly level: number;
}
