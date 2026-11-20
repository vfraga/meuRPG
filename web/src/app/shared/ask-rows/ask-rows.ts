import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { Asks } from '../../core/content/asks';
import { focusAfterRender } from '../../core/content/entry-saver';
import { PickList } from '../form-fields/pick-list';
import { SearchPick, type SearchChip, type SearchOption } from '../form-fields/search-pick';
import { SelectField, type SelectOption } from '../form-fields/select-field';
import { TextField } from '../form-fields/text-field';

export type { Asks };
export type AskKind = 'class' | 'pact' | 'spell' | 'options';

interface Ask {
  kind: AskKind;
  label: string;
  icon: string;
  /** The first path (after the base) is the one that takes the focus. */
  paths: readonly string[];
}

const ASKS: readonly Ask[] = [
  {
    kind: 'class',
    label: 'Nível de classe',
    icon: 'shield',
    paths: ['class_level.class_key', 'class_level.level', 'class_level'],
  },
  { kind: 'pact', label: 'Dádiva do pacto', icon: 'bookmark', paths: ['requires_pact'] },
  {
    kind: 'spell',
    label: 'Magia que conhece',
    icon: 'auto_fix_high',
    paths: ['requires_spell_key'],
  },
  {
    kind: 'options',
    label: 'Outras opções',
    icon: 'format_list_bulleted',
    paths: ['requires_option_keys'],
  },
];

const CLEARED: Record<AskKind, Partial<Asks>> = {
  class: { classKey: '', classLevel: '' },
  pact: { requiresPact: '' },
  spell: { requiresSpell: '' },
  options: { requiresOptions: [] },
};

/** The most "Outras opções" holds, as the server counts it. */
const OPTIONS_MAX = 10;

function filled(kind: AskKind, d: Asks): boolean {
  switch (kind) {
    case 'class':
      return d.classKey !== '' || d.classLevel !== '';
    case 'pact':
      return d.requiresPact !== '';
    case 'spell':
      return d.requiresSpell !== '';
    default:
      return d.requiresOptions.length > 0;
  }
}

/**
 * The rows of "O que ... pede" (boards 580-01 and 580-02): one row for each of the four kinds the master has filled, opened
 * or had refused, an X that clears it, and "Adicionar pré-requisito", whose menu names the four, those on the list as "Já na
 * lista". The parent owns the values: `change` says what to put in the draft.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-ask-rows',
  imports: [MatIconModule, PickList, SearchPick, SelectField, TextField],
  template: `
    @if (head()) {
      <div class="asks-head">
        <h3 class="sub">Classe, pacto, magia e opções</h3>
        <span class="hint">O jogador precisa de todos</span>
      </div>
    }
    @for (a of rows(); track a.kind) {
      <div class="ask">
        <span class="ask__name"><mat-icon aria-hidden="true">{{ a.icon }}</mat-icon>{{ a.label }}</span>
        <div class="ask__body">
          @switch (a.kind) {
            @case ('class') {
              <div class="grid grid--level">
                <app-select-field label="Classe" [options]="classOptions()" [value]="value().classKey" [path]="at('class_level.class_key')" [issues]="issues(at('class_level.class_key'))" (valueChange)="asksChange.emit({ classKey: $event })" />
                <app-text-field label="Nível mínimo" suffix="de 20" [value]="value().classLevel" inputmode="numeric" [path]="at('class_level.level')" [issues]="issues(at('class_level.level'))" (valueChange)="asksChange.emit({ classLevel: $event })" />
              </div>
              @for (m of issues(at('class_level')); track m) {
                <p class="inline-issue" role="note">{{ m }}</p>
              }
            }
            @case ('pact') {
              <app-select-field label="Dádiva" [options]="pactOptions()" [value]="value().requiresPact" [path]="at('requires_pact')" [issues]="issues(at('requires_pact'))" (valueChange)="asksChange.emit({ requiresPact: $event })" />
            }
            @case ('spell') {
              <app-search-pick label="Magia" placeholder="Procurar magia" emptyText="Nenhuma magia com esse nome." [single]="true" [options]="spellSearch()" [chips]="spellChips()" [path]="at('requires_spell_key')" [issues]="issues(at('requires_spell_key'))" (added)="asksChange.emit({ requiresSpell: $event })" (removed)="asksChange.emit({ requiresSpell: '' })" />
            }
            @default {
              <app-pick-list label="Outras opções" addLabel="Adicionar opção" [menuAdd]="true" [options]="requirable()" [values]="value().requiresOptions" [max]="optionsMax" [path]="at('requires_option_keys')" [issues]="issues(at('requires_option_keys'))" [hint]="optionsHint()" [count]="optionsCount()" [flagged]="flaggedOptions()" (added)="asksChange.emit({ requiresOptions: [...value().requiresOptions, $event] })" (removed)="asksChange.emit({ requiresOptions: value().requiresOptions.filter((k) => k !== $event) })" />
            }
          }
        </div>
        <button type="button" class="ask__x" [attr.aria-label]="'Tirar ' + a.label" (click)="clear(a.kind)"><mat-icon aria-hidden="true">close</mat-icon></button>
      </div>
    }
    <div class="asks-add">
      @if (missing().length > 0) {
        <div class="asks-add__menu">
          <button type="button" class="asks-add__link" aria-haspopup="menu" [attr.aria-expanded]="menuOpen()" (click)="menuOpen.set(!menuOpen())"><mat-icon aria-hidden="true">add</mat-icon>Adicionar pré-requisito<mat-icon aria-hidden="true">arrow_drop_down</mat-icon></button>
          @if (menuOpen()) {
            <div class="asks-add__list" role="menu">
              @for (a of all; track a.kind) {
                <button type="button" role="menuitem" [disabled]="isOnList(a.kind)" (click)="add(a.kind)">
                  <mat-icon aria-hidden="true">{{ a.icon }}</mat-icon>
                  <span class="ask-menu__name">{{ a.label }}</span>
                  <span class="ask-menu__tag">{{ tagOf(a.kind) }}</span>
                </button>
              }
              <p class="asks-add__foot">Cada tipo entra uma vez; “Outras opções” leva até 10 opções.</p>
            </div>
          }
        </div>
      } @else {
        <p class="hint"><mat-icon aria-hidden="true">lock</mat-icon>Os quatro tipos já estão na lista. Outra opção entra na linha "Outras opções".</p>
      }
    </div>
  `,
  styleUrl: './ask-rows.scss',
})
export class AskRows {
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly value = input.required<Asks>();
  /** "table_feat.prerequisite" or "table_option.prerequisite": where the refusals of the rows land. */
  readonly base = input.required<string>();
  readonly issuesOf = input.required<(path: string) => readonly string[]>();
  readonly classOptions = input.required<readonly SelectOption[]>();
  readonly pactOptions = input.required<readonly SelectOption[]>();
  readonly requirable = input.required<readonly SelectOption[]>();
  readonly spellSearch = input.required<readonly SearchOption[]>();
  /** The asking option is one of a feature's list: "Outras opções" offers its saved siblings, and the hint says so. */
  readonly inList = input(false);
  /** The keys of "Outras opções" that the refusal of the row is about (the option that waits for this one, or one that is gone). */
  readonly flaggedOptions = input<readonly string[]>([]);
  /** How many options of the list have no key yet: they cannot be asked for until the feature is saved. */
  readonly unsavedPeers = input(0);
  /** The title line of the feat's block; the option's has none. */
  readonly head = input(false);
  /** Changes with the entry open, so the rows the master added and left empty go. */
  readonly resetKey = input('');
  readonly asksChange = output<Partial<Asks>>();

  protected readonly all = ASKS;
  protected readonly optionsMax = OPTIONS_MAX;
  protected readonly menuOpen = signal(false);
  /** The kinds the master opened from the menu and has not filled yet. */
  private readonly opened = signal<readonly AskKind[]>([]);

  /** The rows on the list: the kinds with a value, a refusal, or that the master just added. */
  protected readonly rows = computed(() =>
    ASKS.filter(
      (a) =>
        this.opened().includes(a.kind) ||
        filled(a.kind, this.value()) ||
        a.paths.some((p) => this.issues(this.at(p)).length > 0),
    ),
  );
  protected readonly missing = computed(() =>
    ASKS.filter((a) => !this.rows().some((r) => r.kind === a.kind)),
  );
  protected readonly optionsCount = computed(
    () => `${this.value().requiresOptions.length} de ${OPTIONS_MAX}`,
  );
  protected readonly optionsHint = computed(() => {
    if (!this.inList()) {
      return 'Opções que o personagem já escolheu, do SRD ou da mesa. Nunca a própria opção.';
    }
    const saved =
      this.unsavedPeers() > 0
        ? ' Uma opção nova só pode ser pedida depois de salvar a característica.'
        : '';
    return `Uma opção desta lista, de um conjunto do SRD ou da mesa. Nunca a própria opção.${saved}`;
  });
  protected readonly spellChips = computed<SearchChip[]>(() => {
    const found = this.spellSearch().find((o) => o.value === this.value().requiresSpell);
    return found ? [{ value: found.value, label: found.label, hint: found.hint, tag: '' }] : [];
  });

  constructor() {
    effect(() => {
      this.resetKey();
      untracked(() => {
        this.opened.set([]);
        this.menuOpen.set(false);
      });
    });
  }

  protected issues(path: string): readonly string[] {
    return this.issuesOf()(path);
  }

  protected at(path: string): string {
    return `${this.base()}.${path}`;
  }

  protected isOnList(kind: AskKind): boolean {
    return !this.missing().some((a) => a.kind === kind);
  }

  protected tagOf(kind: AskKind): string {
    if (this.isOnList(kind)) {
      return 'Já na lista';
    }
    return kind === 'options' ? 'Até 10' : '';
  }

  protected add(kind: AskKind): void {
    this.opened.update((o) => [...o, kind]);
    this.menuOpen.set(false);
    focusAfterRender(
      this.host.nativeElement,
      this.injector,
      this.at(ASKS.find((a) => a.kind === kind)?.paths[0] ?? ''),
    );
  }

  protected clear(kind: AskKind): void {
    this.opened.update((o) => o.filter((k) => k !== kind));
    this.asksChange.emit(CLEARED[kind]);
  }
}
