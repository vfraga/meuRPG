import {
  Component,
  ElementRef,
  Injector,
  TemplateRef,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  CRITICAL_COPY,
  EFFECT_FIELD_HINTS,
  EFFECT_TYPE_FIELD_HINTS,
  RESOURCE_PICK_COPY,
  ROLLS_NOTES,
  formulaHintFor,
  typeHintFor,
} from '../../core/content/effect-field-copy';
import {
  type EffectDraft,
  type EffectMenuVm,
  type FeatChoice,
  draftKey,
  emptyEffect,
  meterKey,
  unreadableNumber,
} from '../../core/content/effect-draft';
import { readWhole, wholeText } from '../../core/content/whole-number';
import { neverFolded, visibleFields } from '../../core/content/effect-visible';
import type { EffectMenuField } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { CheckGroup, type CheckOption } from '../form-fields/check-group';
import { CheckRow } from '../form-fields/check-row';
import { FieldNote } from '../form-fields/field-note';
import { PickList } from '../form-fields/pick-list';
import { RadioCards, type RadioCard } from '../form-fields/radio-cards';
import { SelectField, type SelectOption } from '../form-fields/select-field';
import { TextField } from '../form-fields/text-field';
import { CHOICE_GROUP_ICON, insertAt } from './effect-picker-choices';
import { FeatChoiceView } from './feat-choice-view';
import {
  afterTarget,
  fieldLabel,
  fieldView,
  fixedHint,
  fixedMode,
  listOptions,
  modeLockNote,
  refHint,
  refusalTexts,
  resourcePick,
  unreadableText,
} from './effect-picker-parts';

/**
 * The effect picker (MR-025, ADR-0018, section 4; E10-01 state 6, E10-02 state 3): one effect of a feature, chosen from the
 * server's closed menu. The type select lists the menu's types with their one-line explanation; the fields under it are the
 * ones the menu says that type reads, each drawn by how the menu says it is asked (a closed list, several of it, a number, a
 * formula, a text, a yes-or-no switch), and nothing else. Changing the type starts a new effect: what the old type had is dropped, which is also
 * what is never sent. Shared by the race, subrace and background editors and, in 10.12, the class and subclass features.
 *
 * In a feat, the select of a field that may point at one of the feat's named choices (`choices`, from the effects before this one)
 * offers them in the group "A escolha do talento" above the fixed values; a formula gets buttons that write `mod("@name")` and
 * `score("@name")` at the cursor. A choice effect is named by the editor, so the master never types the name.
 *
 * Every input carries its own path (`basePath` + the field) as `data-field`, so a refusal from the server lands on it.
 */
@Component({
  selector: 'app-effect-picker',
  imports: [
    CheckGroup,
    CheckRow,
    FieldNote,
    MatButtonModule,
    MatIconModule,
    NgTemplateOutlet,
    PickList,
    RadioCards,
    SelectField,
    TextField,
  ],
  templateUrl: './effect-picker.html',
  styleUrl: './effect-picker.scss',
})
export class EffectPicker {
  readonly effect = input.required<EffectDraft>();
  readonly menu = input.required<EffectMenuVm>();
  /** "table_race.traits[0].effects[0]": the path the fields' own names are added to. */
  readonly basePath = input.required<string>();
  /** What the server refused at a path, in Portuguese. */
  readonly issuesOf = input<(path: string) => readonly string[]>(() => []);
  /** Offers "Só texto" (no effect) beside the types: the first effect of a feature, which can have none. */
  readonly allowTextOnly = input(false);
  /** The spells a note may grant (the table's catalog): the key and the Portuguese name. */
  readonly spellOptions = input<readonly SelectOption[]>([]);
  /** The feat editor's picker also offers the types only a feat has (`feat_only`). */
  readonly featEditor = input(false);
  /** The effects of the entry this one belongs to: a reroll spends one of its resources. */
  readonly siblings = input<readonly EffectDraft[]>([]);
  /** The label of the type select ("Efeito"). */
  readonly label = input('Efeito');
  /**
   * The named choices of the feat this effect is one of, in the order of its effects (`featChoices`): set only for an effect of
   * the feat itself. With none (null), no field offers a choice, which is what a race, a class and an option of a feat do.
   */
  readonly choices = input<readonly FeatChoice[] | null>(null);
  /** The place of this effect among the effects of the feat: a field offers only the choices that come before it. */
  readonly effectIndex = input(0);
  /** The effect is in an option of a feat: its fields offer fixed values only, and say so. */
  readonly featOption = input(false);
  /**
   * The feat editor draws where a choice of options comes from in its own cards (`afterFields`), so the picker leaves out the
   * "Conjunto" field and, once a set is offered, the list of marked options.
   */
  readonly sourceInCards = input(false);
  /** A block drawn under the fields and above "Mais opções": the feat editor's cards. */
  readonly afterFields = input<TemplateRef<unknown> | null>(null);

  /** The feat's choices as this effect sees them. */
  protected readonly feat = new FeatChoiceView({
    effect: this.effect,
    menu: this.menu,
    choices: this.choices,
    effectIndex: this.effectIndex,
    featOption: this.featOption,
    sourceInCards: this.sourceInCards,
    siblings: this.siblings,
  });

  readonly effectChange = output<EffectDraft>();

  protected readonly typeOptions = computed<SelectOption[]>(() => {
    const types = this.menu()
      .typesFor(this.featEditor())
      .map((t) => ({ value: t.type, label: t.namePt }));
    return this.allowTextOnly() ? [{ value: '', label: 'Só texto' }, ...types] : types;
  });
  protected readonly typeHint = computed(() => {
    const t = this.effect().type;
    return t === ''
      ? 'Só o texto aparece na ficha; o app não faz conta com ele.'
      : typeHintFor(t, this.menu().typeOf(t)?.hintPt ?? '', this.inFeat());
  });
  /** In the feat editor (its effects and its options) the hints are one line. */
  private readonly inFeat = computed(() => this.featEditor() || this.featOption());
  protected readonly allFields = computed(() =>
    visibleFields(this.menu().fieldsOf(this.effect().type), this.effect()).filter(
      // The feat editor names a choice by itself ("habilidade"), so the field is not asked.
      (f) =>
        !(f.name === 'name' && this.featEditor()) &&
        !this.feat.inCards(f, this.issuesOf()(this.path(f.name)).length > 0),
    ),
  );
  /** "Mais opções": the optional fields stay folded until they hold something, a refusal points at them, or the person opens them;
   * a field the menu says is always shown (what the effect does lies in it, as ranged_attack's switches) is never folded. */
  protected readonly expanded = signal(false);
  protected readonly fields = computed(() =>
    this.allFields().filter(
      (f) =>
        this.expanded() ||
        f.required ||
        f.alwaysShown ||
        neverFolded(this.effect().type, f.name) ||
        f.name === 'only_proficient' ||
        f.name === 'cap' ||
        this.filled(f) ||
        this.issuesShown(f.name).length > 0,
    ),
  );
  protected readonly hidden = computed(() => this.allFields().length - this.fields().length);
  protected readonly typeIssues = computed(() => this.issuesOf()(`${this.basePath()}.type`));

  private filled(f: EffectMenuField): boolean {
    const v = this.effect()[meterKey(f.name) ?? draftKey(f.name)];
    return Array.isArray(v)
      ? v.length > 0
      : typeof v === 'number'
        ? v !== 0
        : typeof v === 'string'
          ? v.trim() !== ''
          : v === true;
  }

  protected path(field: string): string {
    return `${this.basePath()}.${field}`;
  }

  protected issues(field: string): readonly string[] {
    const texts = refusalTexts(this.effect(), field, this.issuesOf()(this.path(field)));
    return this.feat.withProblems(field, this.effect().type, texts);
  }

  /** The select of a field whose pick cannot be read shows "Escolha…": the draft keeps the text until the master picks another. */
  protected unreadable(f: EffectMenuField): boolean {
    return this.feat.problem(f) !== null;
  }

  protected selected(f: EffectMenuField): string {
    return this.unreadable(f) ? '' : this.read(f.name);
  }

  /** How a field is drawn when the menu's kind is not the whole story: the rolls as check rows, a reroll's resource from the entry's
   * own, the critical range's scope as cards. */
  protected viewOf(f: EffectMenuField): string {
    return fieldView(this.effect().type, f, this.resource().list);
  }

  /** Rows the whole width of the effect: the rolls and the scope cards. */
  protected wide(f: EffectMenuField): boolean {
    const v = this.viewOf(f);
    return v === 'rolls' || (v === 'cards' && f.name === 'applies');
  }

  protected readonly resource = computed(() =>
    resourcePick(this.siblings(), this.menu(), this.effect().resource),
  );

  protected rollOptions(f: EffectMenuField): CheckOption[] {
    return this.optionsOf(f).map((o) => ({ value: o.value, label: o.label }));
  }

  /** The line under "Vale nas jogadas". */
  protected rollsNote(): string {
    return ROLLS_NOTES[this.effect().type] ?? '';
  }

  protected cardsNote(f: EffectMenuField): string {
    return f.name === 'applies' ? CRITICAL_COPY.scopeNote : '';
  }

  protected suffixOf(name: string): string {
    return name === 'at_most' ? 'ou menos' : '';
  }

  /** The mode of a target that fixes it is shown and cannot change; a stored mode that is another one stays free, to be corrected. */
  protected modeLocked(f: EffectMenuField): boolean {
    const e = this.effect();
    return f.name === 'mode' && e.mode !== '' && e.mode === fixedMode(this.menu(), e.target);
  }

  protected countLocked(f: EffectMenuField): boolean {
    return this.feat.countLocked(f);
  }

  protected locked(f: EffectMenuField): boolean {
    return this.modeLocked(f) || this.countLocked(f);
  }

  /** The reason a locked field is locked, written beside it with its lock. */
  protected lockNote(f: EffectMenuField): string {
    if (this.countLocked(f)) {
      return this.feat.countNote();
    }
    return this.modeLocked(f) ? modeLockNote(this.effect()) : '';
  }

  /** The id of a lock note: the locked control points `aria-describedby` at it. */
  protected lockId(f: EffectMenuField): string {
    return `${this.path(f.name)}-lock`;
  }

  protected labelOf(f: EffectMenuField): string {
    return fieldLabel(this.effect(), f);
  }

  /** The hint of a field that is not showing a refusal: written beside it. */
  protected beside(f: EffectMenuField): string {
    if (this.wide(f)) {
      return '';
    }
    if (this.effect().type === 'reroll' && f.name === 'resource') {
      return RESOURCE_PICK_COPY.hint;
    }
    return f.name === 'from' || f.name === 'spells' ? this.pickHint(f) : this.hintOf(f);
  }

  protected hintOf(f: EffectMenuField): string {
    const fixed = fixedHint(this.effect(), this.menu(), f);
    if (fixed !== undefined) {
      return fixed;
    }
    const own =
      EFFECT_TYPE_FIELD_HINTS[this.effect().type]?.[f.name] ?? EFFECT_FIELD_HINTS[f.name] ?? '';
    if (f.kind === 'formula') {
      // One hint: a field's own sentence says what it is for; the menu's line (the functions) is the fallback
      return formulaHintFor(own, this.menu().formulaHint(), this.inFeat());
    }
    if (f.kind === 'resource_ref' || f.kind === 'state_ref') {
      return refHint(this.effect(), this.menu(), f, own);
    }
    if (f.kind === 'condition') {
      return 'Deixe vazio para valer sempre. Uma condição, como as fórmulas; active("nome") vale enquanto o estado de mesmo nome desta entrada está ligado.';
    }
    if (f.kind === 'choice') {
      const value = this.read(f.name);
      const picked = this.optionsOf(f).find((o) => o.value === value);
      return picked?.hint || this.feat.hint(f, value) || own;
    }
    return own;
  }

  /** The options of a closed list, with the menu's one-line explanation of each. */
  protected optionsOf(f: EffectMenuField): (SelectOption & { hint: string })[] {
    return listOptions(this.menu(), this.effect(), f, this.spellOptions());
  }

  /** A choice with a placeholder when it is required, and a real "Padrão" (the field left empty) when it is not. */
  protected choiceOptions(f: EffectMenuField): SelectOption[] {
    const list = this.feat.withChoices(f, this.optionsOf(f));
    const empty = this.effect().type === 'roll_minimum' ? 'Qualquer tipo' : 'Padrão';
    return f.required || this.unreadable(f) ? list : [{ value: '', label: empty }, ...list];
  }

  protected groupsOf(f: EffectMenuField) {
    return this.feat.groups(f);
  }

  protected footerOf(f: EffectMenuField): string {
    return this.feat.footer(f);
  }

  /** The cards of a short closed list (a resistance's kind), with the menu's line under each. */
  protected cardsOf(f: EffectMenuField): RadioCard[] {
    return this.optionsOf(f).map((o) => ({ value: o.value, label: o.label, hint: o.hint }));
  }

  /** The card that is on: what the draft holds, or the first card, which is what an empty field means. */
  protected cardValue(f: EffectMenuField): string {
    return this.read(f.name) || (this.optionsOf(f)[0]?.value ?? '');
  }

  protected pickOptions(f: EffectMenuField): SelectOption[] {
    return f.name === 'spells'
      ? [...this.spellOptions()]
      : this.feat.withChoices(f, this.optionsOf(f));
  }

  protected pickHint(f: EffectMenuField): string {
    if (f.name === 'from' && this.effect().choice === 'feature' && this.list('from').length === 0) {
      return 'Sem lista: oferece as opções da própria característica ("Opções para escolher").';
    }
    if (f.name === 'from' && this.effect().choice !== '' && this.pickOptions(f).length === 0) {
      return 'Sem lista: o jogador escolhe entre todas as opções deste tipo.';
    }
    return this.hintOf(f);
  }

  /** What the draft holds for a field of the menu, by its proto name; a number shows what was typed while that still reads as the number. */
  protected read(name: string): string {
    const v = this.effect()[draftKey(name)];
    if (typeof v === 'number') {
      const typed = this.effect().typed?.[name];
      return typed !== undefined && Object.is(readWhole(typed, this.signed(name)), v)
        ? typed
        : wholeText(v);
    }
    return typeof v === 'string' ? v : '';
  }

  private signed(name: string): boolean {
    return (this.allFields().find((f) => f.name === name)?.min ?? 0) < 0;
  }

  protected list(name: string): readonly string[] {
    const v = this.effect()[draftKey(name)];
    return Array.isArray(v) ? v : [];
  }

  /** What a field in metres holds (the server stores feet): the range and the melee reach. */
  protected meters(name: string): string {
    const key = meterKey(name);
    return key ? this.effect()[key] : '';
  }

  protected inMeters(name: string): boolean {
    return meterKey(name) !== undefined;
  }

  /** Whether a yes-or-no field is on. */
  protected flag(name: string): boolean {
    return this.effect()[draftKey(name)] === true;
  }

  protected readonly choiceIcon = CHOICE_GROUP_ICON;

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  /** Writes a choice into a formula where the cursor is, and puts the cursor after it. */
  protected insert(f: EffectMenuField, text: string): void {
    const input = this.host.nativeElement.querySelector<HTMLInputElement | HTMLTextAreaElement>(
      `[data-field="${this.path(f.name)}"]`,
    );
    const { next, caret } = insertAt(
      this.read(f.name),
      text,
      input?.selectionStart,
      input?.selectionEnd,
    );
    this.setText(f.name, next);
    afterNextRender(
      () => {
        input?.focus();
        input?.setSelectionRange(caret, caret);
      },
      { injector: this.injector },
    );
  }

  protected setType(type: string): void {
    if (type !== this.effect().type) {
      this.expanded.set(false);
      this.effectChange.emit(emptyEffect(type));
    }
  }

  protected setText(name: string, text: string): void {
    this.effectChange.emit({ ...this.effect(), [draftKey(name)]: text });
  }

  /** A whole number, with a sign where the menu allows one (a bonus may be negative, and "+1" reads as 1). An empty field is 0, "not set".
   * What was typed stays as typed; the draft holds the number, or a NaN for text that is no whole number the server stores, so an editor
   * does not save instead of sending 0. */
  protected setNumber(f: EffectMenuField, text: string): void {
    const n = readWhole(text, f.min < 0);
    const { [f.name]: _, ...kept } = this.effect().typed ?? {};
    const typed = text === wholeText(n) || text.trim() === '' ? kept : { ...kept, [f.name]: text };
    this.effectChange.emit({
      ...this.effect(),
      [draftKey(f.name)]: n,
      typed: Object.keys(typed).length > 0 ? typed : undefined,
    });
  }

  /** The refusals of each visible field, the client's own (text no number) after the server's. Computed once per change, not per cycle. */
  protected readonly shownIssues = computed<ReadonlyMap<string, readonly string[]>>(() => {
    const e = this.effect();
    return new Map(
      this.allFields().map((f) => {
        const own =
          f.kind === 'number' && unreadableNumber(e, f.name)
            ? [unreadableText(f, this.inMeters(f.name))]
            : [];
        return [f.name, [...this.issues(f.name), ...own]] as const;
      }),
    );
  });

  protected issuesShown(name: string): readonly string[] {
    return this.shownIssues().get(name) ?? [];
  }

  protected setFlag(name: string, on: boolean): void {
    this.effectChange.emit({ ...this.effect(), [draftKey(name)]: on });
  }

  protected setMeters(name: string, text: string): void {
    const key = meterKey(name);
    if (key) {
      this.effectChange.emit({ ...this.effect(), [key]: text });
    }
  }

  protected setChoice(f: EffectMenuField, value: string): void {
    // A new kind of choice has its own `from` list: what was picked for another kind goes.
    const next = { ...this.effect(), [draftKey(f.name)]: value } as EffectDraft;
    if (f.name === 'target') {
      this.effectChange.emit(afterTarget(this.menu(), next));
      return;
    }
    if (f.name === 'choice') {
      // A feat's choice of an ability or a damage type is named by the editor; any other kind has no name.
      const name = this.featEditor() ? this.feat.nameFor(value) : '';
      this.effectChange.emit({ ...next, from: [], name });
      return;
    }
    this.effectChange.emit(next);
  }

  /** The rolls checked. A damage type holds only on damage alone: checking anything else drops it. */
  protected setRolls(rolls: string[]): void {
    const only = rolls.length === 1 && rolls[0] === 'damage';
    this.effectChange.emit({
      ...this.effect(),
      rolls,
      damageType: only ? this.effect().damageType : '',
    });
  }

  /** The resource picked from the list; taking it away drops the cost that went with it. */
  protected setResource(resource: string): void {
    this.effectChange.emit({
      ...this.effect(),
      resource,
      cost: resource === '' ? '' : this.effect().cost,
    });
  }

  protected add(name: string, key: string): void {
    this.effectChange.emit({ ...this.effect(), [draftKey(name)]: [...this.list(name), key] });
  }

  protected remove(name: string, key: string): void {
    this.effectChange.emit({
      ...this.effect(),
      [draftKey(name)]: this.list(name).filter((k) => k !== key),
    });
  }
}
