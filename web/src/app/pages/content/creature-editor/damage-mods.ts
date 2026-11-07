import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterEveryRender,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatMenuModule } from '@angular/material/menu';
import { MatSelectModule } from '@angular/material/select';

import {
  type DamageModDraft,
  MAX_DAMAGE_MODS,
  MOD_KINDS,
  QUALIFIERS,
  duplicateText,
  emptyDamageMod,
  qualifierWhy,
} from '../../../core/content/creature-damage-mods';
import { DUPLICATE_TYPE_TEXT } from '../../../core/content/content-violation-texts-creature';
import { focusAfterRender } from '../../../core/content/entry-saver';
import { FieldNote } from '../../../shared/form-fields/field-note';
import type { SelectOption } from '../../../shared/form-fields/select-field';

const P = 'table_creature.damage_mods';

interface Choice {
  readonly value: string;
  readonly label: string;
  readonly icon?: string;
  readonly disabled?: boolean;
}

/** A value this build does not know (an entry from a newer one) stays an option, so nothing is rewritten. */
const unknown = (value: string): string => `valor desconhecido: ${value || 'vazio'}`;

/**
 * "Resistências, imunidades e vulnerabilidades" of the table creature (#585): up to 12 rows, each one table line with a kind, the
 * damage types it names and a "Vale contra" qualifier. They add to the base's lists and never take one away. The qualifier is off
 * (dashed) for a vulnerability and for a damage type that is not bludgeoning, piercing or slashing; refusals sit under their select.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-creature-damage-mods',
  imports: [
    FieldNote,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatMenuModule,
    MatSelectModule,
  ],
  templateUrl: './damage-mods.html',
  styleUrls: ['../editor.scss', './creature-rows.scss', './damage-mods.scss'],
})
export class CreatureDamageMods {
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  constructor() {
    // A select with no form control never marks itself invalid: the refusals of its path do, after Material's own binding.
    afterEveryRender(() => {
      this.host.nativeElement
        .querySelectorAll<HTMLElement>('mat-select[data-field]')
        .forEach((el) => {
          const invalid = String(this.invalid(el.dataset['field'] ?? ''));
          if (el.getAttribute('aria-invalid') !== invalid) {
            el.setAttribute('aria-invalid', invalid);
          }
        });
    });
  }

  readonly mods = input.required<readonly DamageModDraft[]>();
  readonly damageTypes = input.required<readonly SelectOption[]>();
  readonly nameOf = input.required<(key: string) => string>();
  /** What the base's stat block already says, as `baseModsText` writes it. */
  readonly baseText = input('');
  readonly baseName = input('');
  /** The refusals of a path. */
  readonly issues = input.required<(path: string) => readonly string[]>();
  readonly modsChange = output<DamageModDraft[]>();

  protected readonly path = P;
  protected readonly max = MAX_DAMAGE_MODS;
  protected readonly full = computed(() => this.mods().length >= MAX_DAMAGE_MODS);
  protected readonly status = signal('');

  /** The live region says a repeated message again: the text alternates by a no-break space, so it always changes. */
  private say(text: string): void {
    this.status.update((s) => (s === text ? `${text}\u00a0` : text));
  }

  protected rowPath(i: number): string {
    return `${P}[${i}]`;
  }

  protected invalid(path: string): boolean {
    return this.issues()(path).length > 0;
  }

  /** The note under a select is read with it only when it holds a refusal. */
  protected describedBy(id: string, tag: string, path: string): string {
    return this.invalid(path) ? `cr-${tag}-${id}` : '';
  }

  protected kindOption(m: DamageModDraft): Choice {
    return this.kindOptions(m).find((k) => k.value === m.kind) as Choice;
  }

  protected kindOptions(m: DamageModDraft): readonly Choice[] {
    return MOD_KINDS.some((k) => k.value === m.kind)
      ? MOD_KINDS
      : [...MOD_KINDS, { value: m.kind, label: unknown(m.kind), icon: 'help' }];
  }

  protected typeOptions(m: DamageModDraft): readonly SelectOption[] {
    const known = this.damageTypes();
    const extra = [...new Set(m.types)]
      .filter((t) => !known.some((o) => o.value === t))
      .map((t) => ({ value: t, label: unknown(t) }));
    return [...known, ...extra];
  }

  protected typesText(m: DamageModDraft): string {
    return m.types.map((t) => this.nameOf()(t).toLowerCase()).join(', ');
  }

  /** The words of a duplicate refusal name the type the row holds twice. */
  protected typeIssues(m: DamageModDraft, at: string): readonly string[] {
    const twice = duplicateText(m.types, this.nameOf());
    return this.issues()(`${at}.damage_types`).map((t) =>
      t === DUPLICATE_TYPE_TEXT && twice !== '' ? twice : t,
    );
  }

  /** "Vale contra" is off for a row it cannot apply to, unless it already holds a value that must stay in view. */
  protected qualifierOff(m: DamageModDraft): boolean {
    return qualifierWhy(m.kind, m.types) !== '' && m.qualifier === '';
  }

  protected qualifierOptions(m: DamageModDraft): readonly Choice[] {
    const impossible = qualifierWhy(m.kind, m.types) !== '';
    const all: Choice[] = QUALIFIERS.map((q) => ({
      value: q.value,
      label: q.label,
      disabled: impossible && q.value !== '' && q.value !== m.qualifier,
    }));
    return all.some((q) => q.value === m.qualifier)
      ? all
      : [...all, { value: m.qualifier, label: unknown(m.qualifier) }];
  }

  protected qualifierOption(m: DamageModDraft): Choice {
    return this.qualifierOptions(m).find((q) => q.value === m.qualifier) as Choice;
  }

  private change(id: string, p: Partial<DamageModDraft>): void {
    this.modsChange.emit(this.mods().map((m) => (m.id === id ? { ...m, ...p } : m)));
  }

  /** A kind or a type that takes "Vale contra" away from the row takes the qualifier with it, so the save is not refused for it. */
  private withQualifier(m: DamageModDraft, p: Partial<DamageModDraft>): Partial<DamageModDraft> {
    const next = { ...m, ...p };
    return qualifierWhy(next.kind, next.types) === '' ? p : { ...p, qualifier: '' };
  }

  protected setKind(m: DamageModDraft, kind: string): void {
    this.change(m.id, this.withQualifier(m, { kind }));
  }

  /**
   * The select gives back the set of types that are checked. A type that is gone loses one place (a row that holds it twice keeps
   * the other until it is unchecked too); a type that is new goes to the end.
   */
  protected setTypes(m: DamageModDraft, checked: readonly string[]): void {
    const types = [...m.types];
    for (const t of new Set(m.types)) {
      if (!checked.includes(t)) {
        types.splice(types.lastIndexOf(t), 1);
      }
    }
    types.push(...checked.filter((t) => !m.types.includes(t)));
    this.change(m.id, this.withQualifier(m, { types }));
  }

  protected setQualifier(m: DamageModDraft, qualifier: string): void {
    this.change(m.id, { qualifier });
  }

  protected add(): void {
    if (this.full()) {
      return;
    }
    const m = emptyDamageMod();
    const at = this.mods().length;
    this.modsChange.emit([...this.mods(), m]);
    this.say('Linha adicionada.');
    focusAfterRender(this.host.nativeElement, this.injector, `${this.rowPath(at)}.kind`);
  }

  protected remove(id: string): void {
    this.modsChange.emit(this.mods().filter((m) => m.id !== id));
    this.say('Linha removida.');
    afterNextRender(
      () => {
        this.host.nativeElement.querySelector<HTMLElement>('.add button')?.focus();
      },
      { injector: this.injector },
    );
  }
}
