import { Component, computed, input, output, signal } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import {
  EFFECT_FIELD_HINTS,
  EFFECT_FIELD_LABELS,
  type EffectDraft,
  type EffectMenuVm,
  emptyEffect,
} from '../../core/content/effect-draft';
import type { EffectMenuField } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { PickList } from '../form-fields/pick-list';
import { SelectField, type SelectOption } from '../form-fields/select-field';
import { TextField } from '../form-fields/text-field';

/**
 * The effect picker (MR-025, ADR-0018, section 4; E10-01 state 6, E10-02 state 3): one effect of a feature, chosen from the
 * server's closed menu. The type select lists the menu's types with their one-line explanation; the fields under it are the
 * ones the menu says that type reads, each drawn by how the menu says it is asked (a closed list, several of it, a number, a
 * formula, a text), and nothing else. Changing the type starts a new effect: what the old type had is dropped, which is also
 * what is never sent. Shared by the race, subrace and background editors and, in 10.12, the class and subclass features.
 *
 * Every input carries its own path (`basePath` + the field) as `data-field`, so a refusal from the server lands on it.
 */
@Component({
  selector: 'app-effect-picker',
  imports: [MatIconModule, PickList, SelectField, TextField],
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
  /** The label of the type select ("Efeito"). */
  readonly label = input('Efeito');

  readonly effectChange = output<EffectDraft>();

  protected readonly typeOptions = computed<SelectOption[]>(() => {
    const types = this.menu().types.map((t) => ({ value: t.type, label: t.namePt }));
    return this.allowTextOnly() ? [{ value: '', label: 'Só texto' }, ...types] : types;
  });
  protected readonly typeHint = computed(() => {
    const t = this.effect().type;
    return t === ''
      ? 'Só o texto aparece na ficha; o app não faz conta com ele.'
      : (this.menu().typeOf(t)?.hintPt ?? '');
  });
  protected readonly allFields = computed(() => this.menu().fieldsOf(this.effect().type));
  /** "Mais opções": the optional fields stay folded until they hold something, a refusal points at them, or the person opens them. */
  protected readonly expanded = signal(false);
  protected readonly fields = computed(() =>
    this.allFields().filter(
      (f) => this.expanded() || f.required || this.filled(f) || this.issues(f.name).length > 0,
    ),
  );
  protected readonly hidden = computed(() => this.allFields().length - this.fields().length);
  protected readonly typeIssues = computed(() => this.issuesOf()(`${this.basePath()}.type`));

  private filled(f: EffectMenuField): boolean {
    const v = this.effect()[camel(f.name)];
    return Array.isArray(v)
      ? v.length > 0
      : typeof v === 'number'
        ? v > 0
        : typeof v === 'string'
          ? v.trim() !== ''
          : false;
  }

  protected path(field: string): string {
    return `${this.basePath()}.${field}`;
  }

  protected issues(field: string): readonly string[] {
    return this.issuesOf()(this.path(field));
  }

  protected labelOf(f: EffectMenuField): string {
    return EFFECT_FIELD_LABELS[f.name] ?? f.name;
  }

  /** The hint of a field that is not showing a refusal: written beside it. */
  protected beside(f: EffectMenuField): string {
    return f.name === 'from' || f.name === 'spells' ? this.pickHint(f) : this.hintOf(f);
  }

  protected hintOf(f: EffectMenuField): string {
    const own = EFFECT_FIELD_HINTS[f.name] ?? '';
    if (f.name === 'count' && f.max > 0) {
      return `De ${f.min} a ${f.max}.`;
    }
    if (f.name === 'tags') {
      return this.menu().tagHint();
    }
    if (f.kind === 'formula') {
      return this.menu().formulaHint();
    }
    if (f.kind === 'condition') {
      return 'Deixe vazio para valer sempre. Uma condição, como as fórmulas.';
    }
    if (f.kind === 'choice') {
      const list = this.optionsOf(f);
      const picked = list.find((o) => o.value === this.read(f.name));
      return picked?.hint ?? own;
    }
    return own;
  }

  /** The options of a closed list, with the menu's one-line explanation of each. */
  protected optionsOf(f: EffectMenuField): (SelectOption & { hint: string })[] {
    const menu = this.menu();
    const list =
      f.name === 'from'
        ? menu.fromOptions(this.effect().choice)
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

  protected pickOptions(f: EffectMenuField): SelectOption[] {
    return f.name === 'spells' ? [...this.spellOptions()] : this.optionsOf(f);
  }

  protected pickHint(f: EffectMenuField): string {
    if (f.name === 'from' && this.effect().choice !== '' && this.pickOptions(f).length === 0) {
      return 'Sem lista: o jogador escolhe entre todas as opções deste tipo.';
    }
    return this.hintOf(f);
  }

  /** What the draft holds for a field of the menu, by its proto name. */
  protected read(name: string): string {
    const v = this.effect()[camel(name)];
    return typeof v === 'string' ? v : typeof v === 'number' ? (v > 0 ? String(v) : '') : '';
  }

  protected list(name: string): readonly string[] {
    const v = this.effect()[camel(name)];
    return Array.isArray(v) ? v : [];
  }

  protected meters(): string {
    return this.effect().rangeM;
  }

  protected setType(type: string): void {
    if (type !== this.effect().type) {
      this.expanded.set(false);
      this.effectChange.emit(emptyEffect(type));
    }
  }

  protected setText(name: string, text: string): void {
    this.effectChange.emit({ ...this.effect(), [camel(name)]: text });
  }

  protected setNumber(name: string, text: string): void {
    const n = Number(text.trim());
    this.effectChange.emit({
      ...this.effect(),
      [camel(name)]: Number.isInteger(n) && n > 0 ? n : 0,
    });
  }

  protected setMeters(text: string): void {
    this.effectChange.emit({ ...this.effect(), rangeM: text });
  }

  protected setChoice(f: EffectMenuField, value: string): void {
    // A new kind of choice has its own `from` list: what was picked for another kind goes.
    const next = { ...this.effect(), [camel(f.name)]: value } as EffectDraft;
    this.effectChange.emit(f.name === 'choice' ? { ...next, from: [] } : next);
  }

  protected add(name: string, key: string): void {
    this.effectChange.emit({ ...this.effect(), [camel(name)]: [...this.list(name), key] });
  }

  protected remove(name: string, key: string): void {
    this.effectChange.emit({
      ...this.effect(),
      [camel(name)]: this.list(name).filter((k) => k !== key),
    });
  }
}

/** "text_pt" → "textPt": the draft's key for a proto field. */
function camel(name: string): keyof EffectDraft {
  return name.replace(/_(\w)/g, (_, c: string) => c.toUpperCase()) as keyof EffectDraft;
}
