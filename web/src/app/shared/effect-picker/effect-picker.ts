import { Component, computed, input, output, signal } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import {
  EFFECT_FIELD_HINTS,
  EFFECT_FIELD_LABELS,
  EFFECT_TYPE_FIELD_HINTS,
  EFFECT_TYPE_FIELD_LABELS,
} from '../../core/content/effect-field-copy';
import {
  type EffectDraft,
  type EffectMenuVm,
  UNREADABLE_RANGE,
  draftKey,
  emptyEffect,
  unreadableNumber,
} from '../../core/content/effect-draft';
import {
  UNREADABLE_BONUS,
  UNREADABLE_COUNT,
  readWhole,
  wholeText,
} from '../../core/content/whole-number';
import { visibleFields } from '../../core/content/effect-visible';
import type { EffectMenuField } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { CheckRow } from '../form-fields/check-row';
import { FieldNote } from '../form-fields/field-note';
import { PickList } from '../form-fields/pick-list';
import { RadioCards, type RadioCard } from '../form-fields/radio-cards';
import { SelectField, type SelectOption } from '../form-fields/select-field';
import { TextField } from '../form-fields/text-field';

/**
 * The effect picker (MR-025, ADR-0018, section 4; E10-01 state 6, E10-02 state 3): one effect of a feature, chosen from the
 * server's closed menu. The type select lists the menu's types with their one-line explanation; the fields under it are the
 * ones the menu says that type reads, each drawn by how the menu says it is asked (a closed list, several of it, a number, a
 * formula, a text, a yes-or-no switch), and nothing else. Changing the type starts a new effect: what the old type had is dropped, which is also
 * what is never sent. Shared by the race, subrace and background editors and, in 10.12, the class and subclass features.
 *
 * Every input carries its own path (`basePath` + the field) as `data-field`, so a refusal from the server lands on it.
 */
@Component({
  selector: 'app-effect-picker',
  imports: [CheckRow, FieldNote, MatIconModule, PickList, RadioCards, SelectField, TextField],
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
  /** The label of the type select ("Efeito"). */
  readonly label = input('Efeito');

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
      : (this.menu().typeOf(t)?.hintPt ?? '');
  });
  protected readonly allFields = computed(() =>
    visibleFields(this.menu().fieldsOf(this.effect().type), this.effect()),
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
        f.name === 'only_proficient' ||
        this.filled(f) ||
        this.issuesShown(f.name).length > 0,
    ),
  );
  protected readonly hidden = computed(() => this.allFields().length - this.fields().length);
  protected readonly typeIssues = computed(() => this.issuesOf()(`${this.basePath()}.type`));

  private filled(f: EffectMenuField): boolean {
    const v = this.effect()[draftKey(f.name)];
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
    return this.issuesOf()(this.path(field));
  }

  protected labelOf(f: EffectMenuField): string {
    const typed = EFFECT_TYPE_FIELD_LABELS[this.effect().type]?.[f.name];
    if (typed) {
      return typed;
    }
    // The resource an action spends is the same proto field as a resource's own name; a hook's `restore` gives back.
    if (f.kind === 'resource_ref') {
      return this.effect().type === 'on_event'
        ? (RESOURCE_REF_LABEL_HOOK[f.name] ?? 'Gasta usos de')
        : 'Gasta usos de';
    }
    return EFFECT_FIELD_LABELS[f.name] ?? f.name;
  }

  /** The hint of a field that is not showing a refusal: written beside it. */
  protected beside(f: EffectMenuField): string {
    return f.name === 'from' || f.name === 'spells' ? this.pickHint(f) : this.hintOf(f);
  }

  protected hintOf(f: EffectMenuField): string {
    const own =
      EFFECT_TYPE_FIELD_HINTS[this.effect().type]?.[f.name] ?? EFFECT_FIELD_HINTS[f.name] ?? '';
    if (f.name === 'count' && f.max > 0) {
      return `De ${f.min} a ${f.max}.`;
    }
    if (f.name === 'tags') {
      return this.menu().tagHint();
    }
    if (f.kind === 'formula') {
      // One hint: a field's own sentence says what it is for; the menu's line (the functions) is the fallback
      return own || this.menu().formulaHint();
    }
    if (f.kind === 'resource_ref' || f.kind === 'state_ref') {
      return this.refHint(f, own);
    }
    if (f.kind === 'condition') {
      return 'Deixe vazio para valer sempre. Uma condição, como as fórmulas; active("nome") vale enquanto o estado de mesmo nome desta entrada está ligado.';
    }
    if (f.kind === 'choice') {
      const list = this.optionsOf(f);
      const picked = list.find((o) => o.value === this.read(f.name));
      return picked?.hint ?? own;
    }
    return own;
  }

  /** The hint of a field that names a resource or a state of the entry. */
  private refHint(f: EffectMenuField, own: string): string {
    if (f.kind === 'state_ref') {
      return this.stateRefHint(f);
    }
    // `uses` has its own sentence (an empty resource is at will, not a disabled action), as `restore` does
    if (this.effect().type === 'state') {
      return STATE_RESOURCE_HINT;
    }
    return OWN_HINT_REFS.has(f.name) ? own : RESOURCE_REF_HINT;
  }

  /** Under "Só enquanto o estado": the name of a state effect of the entry, and the ready values the menu lists ("rage"). */
  private stateRefHint(f: EffectMenuField): string {
    const ready = this.menu()
      .list(f.list)
      .map((o) => `${o.key} (${o.namePt})`);
    const own =
      'O nome de um estado que esta entrada define (o campo "Nome do estado" de um efeito Estado).';
    return ready.length > 0 ? `${own} Também vale: ${ready.join(', ')}.` : own;
  }

  /** The options of a closed list, with the menu's one-line explanation of each. */
  protected optionsOf(f: EffectMenuField): (SelectOption & { hint: string })[] {
    if (f.name === 'spell') {
      return this.spellOptions().map((o) => ({ ...o, hint: '' }));
    }
    const menu = this.menu();
    const list =
      f.name === 'from' && f.list === ''
        ? this.effect().type === 'spell_modifier'
          ? menu.list('damage_types')
          : menu.fromOptions(this.effect().choice)
        : f.name === 'spells'
          ? []
          : menu.list(f.list);
    return list.map((o) => ({ value: o.key, label: o.namePt, hint: o.hintPt }));
  }

  /** A choice with a placeholder when it is required, and a real "Padrão" (the field left empty) when it is not. */
  protected choiceOptions(f: EffectMenuField): SelectOption[] {
    const list = this.optionsOf(f);
    return f.required ? list : [{ value: '', label: 'Padrão' }, ...list];
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
    return f.name === 'spells' ? [...this.spellOptions()] : this.optionsOf(f);
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
    return name === 'reach_ft' ? this.effect().reachM : this.effect().rangeM;
  }

  protected inMeters(name: string): boolean {
    return name === 'range_ft' || name === 'reach_ft';
  }

  /** Whether a yes-or-no field is on. */
  protected flag(name: string): boolean {
    return this.effect()[draftKey(name)] === true;
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
          f.kind === 'number' && unreadableNumber(e, f.name) ? [this.unreadableText(f)] : [];
        return [f.name, [...this.issues(f.name), ...own]] as const;
      }),
    );
  });

  protected issuesShown(name: string): readonly string[] {
    return this.shownIssues().get(name) ?? [];
  }

  private unreadableText(f: EffectMenuField): string {
    if (f.name === 'range_ft' || f.name === 'reach_ft') {
      return UNREADABLE_RANGE;
    }
    return f.min < 0 ? UNREADABLE_BONUS : UNREADABLE_COUNT;
  }

  protected setFlag(name: string, on: boolean): void {
    this.effectChange.emit({ ...this.effect(), [draftKey(name)]: on });
  }

  protected setMeters(name: string, text: string): void {
    this.effectChange.emit({ ...this.effect(), [name === 'reach_ft' ? 'reachM' : 'rangeM']: text });
  }

  protected setChoice(f: EffectMenuField, value: string): void {
    // A new kind of choice has its own `from` list: what was picked for another kind goes.
    const next = { ...this.effect(), [draftKey(f.name)]: value } as EffectDraft;
    this.effectChange.emit(f.name === 'choice' ? { ...next, from: [] } : next);
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

/** The labels of a hook's two resource fields: the one it spends and the one it gives back. */
const OWN_HINT_REFS: ReadonlySet<string> = new Set(['restore', 'uses']);

const RESOURCE_REF_LABEL_HOOK: Readonly<Record<string, string>> = {
  resource: 'Gasta usos de',
  restore: 'Devolve usos de',
};

const STATE_RESOURCE_HINT =
  'O nome de um recurso que esta entrada define (o campo "Nome do recurso" de um efeito Recurso). Cada vez que o estado liga, gasta 1 uso; sem usos, o cartão fica desligado.';

const RESOURCE_REF_HINT =
  'O nome de um recurso que esta entrada define (o campo "Nome do recurso" de um efeito Recurso). Gasta o custo (1 se vazio); sem usos o bastante, a ação fica desligada.';
