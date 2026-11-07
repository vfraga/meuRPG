import type { EffectMenuField } from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { ReferenceProblem } from '../../core/content/effect-choices';
import {
  type EffectDraft,
  type EffectMenuVm,
  type FeatChoice,
  defaultChoiceName,
  formulaInsertions,
  referenceProblem,
} from '../../core/content/effect-draft';
import {
  type ReadingPart,
  formulaReading,
  pointsAtChoice,
} from '../../core/content/formula-reading';
import type { ChipName } from '../form-fields/pick-list';
import type { SelectGroup, SelectOption } from '../form-fields/select-field';
import {
  CHOICE_GROUPS,
  CHOICE_GROUP_NOTE,
  LOCKED_GROUPS,
  MISSING_TEXTS,
  OPTION_NOTE,
  type RefOption,
  choiceSide,
  fixedGroup,
  lockedRow,
  pickedRef,
  refKind,
  refOptions,
} from './effect-picker-choices';

/** What the view reads of the picker: its inputs, as the signals themselves. */
export interface FeatChoiceHost {
  readonly effect: () => EffectDraft;
  readonly menu: () => EffectMenuVm;
  readonly choices: () => readonly FeatChoice[] | null;
  readonly effectIndex: () => number;
  readonly featOption: () => boolean;
  readonly sourceInCards: () => boolean;
  readonly siblings: () => readonly EffectDraft[];
}

/**
 * The feat's named choices as one effect of the feat sees them: the group above the fixed values of each field that may point
 * at one, the buttons and the reading of a formula, the name a new choice gets and the lock of "Quantos". Only an effect of the
 * feat itself has choices (`choices` set); an option of the feat has none and says so.
 */
export class FeatChoiceView {
  constructor(private readonly host: FeatChoiceHost) {}

  /** The feat's choices of the kind a field may point at; none when the field takes none, the effect has no choices or is an option of the feat. */
  private ofKind(f: EffectMenuField): readonly FeatChoice[] {
    const kind = refKind(f.name);
    const all = this.host.choices();
    if (!kind || all === null || this.host.featOption()) {
      return [];
    }
    return all.filter((c) => c.kind === kind);
  }

  /** The earlier choices of the feat a field may point at: only its own kind, only from the effects before this one. */
  private refsOf(f: EffectMenuField): RefOption[] {
    const before = this.ofKind(f).filter((c) => c.index < this.host.effectIndex());
    return refOptions(f.name, before, (c) => choiceSide(c, this.damageTypesOf(c)));
  }

  /** The damage types a choice of the feat offers, in lower case ("ácido"); none when it offers them all. */
  private damageTypesOf(c: FeatChoice): string[] {
    const from = this.host.siblings()[c.index]?.from ?? [];
    return from.map((key) => this.host.menu().nameOf('damage_types', key).toLowerCase());
  }

  /** The first choice of the field's kind that comes below this effect: it exists, but the field cannot use it from here. */
  private laterOf(f: EffectMenuField): FeatChoice | undefined {
    return this.ofKind(f).find((c) => c.index > this.host.effectIndex());
  }

  /** The group of choices above the fixed values; with only a later choice, the group locked, saying where it is. */
  withChoices<T extends SelectOption>(f: EffectMenuField, list: readonly T[]): SelectOption[] {
    const refs = this.refsOf(f);
    if (refs.length > 0) {
      return [...refs, ...fixedGroup(list)];
    }
    const later = this.laterOf(f);
    return later ? [lockedRow(later), ...fixedGroup(list)] : [...list];
  }

  /** How the group's header looks: the lock when the group says only that the choice is below. */
  groups(f: EffectMenuField): Readonly<Record<string, SelectGroup>> {
    return this.refsOf(f).length === 0 && this.laterOf(f) ? LOCKED_GROUPS : CHOICE_GROUPS;
  }

  /** Why the pick a field holds cannot be read now; null when it reads or the field holds none. */
  problem(f: EffectMenuField): ReferenceProblem | null {
    const all = this.host.choices();
    if (all === null || this.host.featOption()) {
      return null;
    }
    return referenceProblem(this.host.effect(), f.name, all, this.host.effectIndex());
  }

  /** A field the feat editor's cards hold: the set, and the marked options while a set is offered (a refusal of them still shows). */
  inCards(f: EffectMenuField, refused: boolean): boolean {
    if (!this.host.sourceInCards()) {
      return false;
    }
    const e = this.host.effect();
    return (
      f.name === 'set' || (f.name === 'from' && e.choice === 'feature' && e.set !== '' && !refused)
    );
  }

  /** The texts of a field's refusal, then the editor's own for a pick it cannot read, each once. */
  withProblems(name: string, type: string, texts: readonly string[]): readonly string[] {
    const field = this.host
      .menu()
      .fieldsOf(type)
      .find((f) => f.name === name);
    const own = field ? this.problemTexts(field) : [];
    return own.length === 0 ? texts : [...new Set([...texts, ...own])];
  }

  /** The refusal the editor writes by itself for a pick it cannot read, so nothing is re-pointed behind the master's back. */
  problemTexts(f: EffectMenuField): string[] {
    const p = this.problem(f);
    return p ? [p.why === 'removed' ? MISSING_TEXTS.removed(p.name) : MISSING_TEXTS.later()] : [];
  }

  /** The chips of a list field whose pick is not among its entries: a choice that was removed or sits below this effect, named. */
  chipNames(f: EffectMenuField, values: readonly string[]): Readonly<Record<string, ChipName>> {
    const out: Record<string, ChipName> = {};
    for (const key of values) {
      const name = /@([a-z0-9_]+)/.exec(key)?.[1];
      if (name === undefined || name === 'mesa' || this.refsOf(f).some((o) => o.value === key)) {
        continue;
      }
      const below = this.ofKind(f).some((c) => c.name === name);
      out[key] = below
        ? { label: `Escolha abaixo (${name})`, remove: `Tirar a escolha abaixo ${name}` }
        : { label: `Escolha removida (${name})`, remove: `Tirar a escolha removida ${name}` };
    }
    return out;
  }

  /** The line a field's value has beside it when it is one of the group's entries. */
  hint(f: EffectMenuField, value: string): string {
    return pickedRef(this.refsOf(f), value)?.hint ?? '';
  }

  /** The line at the foot of a field's list: where the group comes from, or in an option of the feat that it has none. */
  footer(f: EffectMenuField): string {
    if (this.refsOf(f).length > 0) {
      return CHOICE_GROUP_NOTE;
    }
    return this.host.featOption() && refKind(f.name) !== null ? OPTION_NOTE : '';
  }

  /** This effect among the feat's choices, once it has a name. */
  own(): FeatChoice | null {
    const e = this.host.effect();
    return e.type === 'choice' && e.name !== ''
      ? (this.host.choices()?.find((c) => c.index === this.host.effectIndex()) ?? null)
      : null;
  }

  /** A choice with a name picks one: "Quantos" shows 1 and cannot change. */
  countLocked(f: EffectMenuField): boolean {
    const e = this.host.effect();
    return f.name === 'count' && e.type === 'choice' && e.name !== '';
  }

  countNote(): string {
    return this.host.effect().choice === 'damage_type'
      ? 'Um tipo por escolha.'
      : 'Uma por escolha. Para duas, adicione outra escolha.';
  }

  /** The name a choice gets when its kind is picked: "habilidade", then "habilidade_2"; none for a kind that has no name. */
  nameFor(kind: string): string {
    const others = this.host
      .siblings()
      .filter((_, i) => i !== this.host.effectIndex())
      .map((e) => e.name);
    return defaultChoiceName(kind, others);
  }

  /** The earlier ability choices a formula may read, as buttons: "Modificador · Habilidade", "Valor · Habilidade". */
  insertions(f: EffectMenuField) {
    const all = this.host.choices();
    if (all === null || this.host.featOption() || f.kind !== 'formula') {
      return [];
    }
    return formulaInsertions(all.filter((c) => c.index < this.host.effectIndex()));
  }

  /** What a formula says in words, when it points at a choice of the feat. */
  reading(f: EffectMenuField, text: string): ReadingPart[] {
    const all = this.host.choices();
    if (all === null || (f.kind !== 'formula' && f.kind !== 'condition') || !pointsAtChoice(text)) {
      return [];
    }
    const parts = formulaReading(text, all);
    return parts.map((p, i) => ({
      ...p,
      text: i === 0 ? p.text.trimStart() : i === parts.length - 1 ? p.text.trimEnd() : p.text,
    }));
  }

  /** Under "Texto na ficha": where the number of the formula goes, when the text has the place and the formula reads a choice. */
  valueSlot(f: EffectMenuField): string {
    const e = this.host.effect();
    if (f.name !== 'text_pt' || this.host.choices() === null || !e.textPt.includes('{value}')) {
      return '';
    }
    if (!pointsAtChoice(e.value)) {
      return '';
    }
    return /\bCD\b/.test(e.textPt) ? 'é onde o número da CD entra.' : 'é onde o número entra.';
  }
}
