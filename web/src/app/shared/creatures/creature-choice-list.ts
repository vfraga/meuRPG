import {
  Component,
  ElementRef,
  afterNextRender,
  computed,
  effect,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { AttackLine } from '../../core/creatures/creature-format';
import { TABLE_CREATURE_PICK_NOTE } from '../../core/creatures/creature-format';
import { TableMark } from '../table-mark/table-mark';
import { CreatureArt } from './creature-art';

/** One creature of a choice list. */
export interface ChoiceRow {
  readonly key: string;
  /** The Portuguese name. */
  readonly title: string;
  /** The book's English name, shown in parentheses by the master's search. */
  readonly alias?: string;
  /** "Miúdo · 1 PV · 3 m, voo 15 m". */
  readonly subtitle: string;
  /** Shows the silhouette (the master's search); the forms of a spell are plain. */
  readonly art?: boolean;
  /** The creature's attacks (from its stat block): the name and numbers under the subtitle, and the book's text under them. */
  readonly attacks?: readonly AttackLine[];
  /** A creature of the table's own (key ends with "@mesa"): "Da mesa" beside the name, and the note in place of numbers it does not have yet. */
  readonly table?: boolean;
}

let nextId = 0;

/**
 * A list of creatures to pick from (E9-10). One choice: native radios under the rows, the chosen row
 * with a frame AND a filled marker (never colour alone). Several (`multi`, a casting that makes
 * three skeletons and a zombie): each row has "−", its count and "+", and `step` says which. With
 * `searchable`, a field above filters the rows by name while typing; it never scrolls away. The list
 * is the only part that scrolls: it takes the room its sheet leaves (`min-height` keeps two rows), and
 * a soft fade at the bottom says there is more below.
 */
@Component({
  selector: 'app-creature-choice-list',
  imports: [CreatureArt, MatIconModule, TableMark],
  template: `
    @if (searchable()) {
      <label class="search">
        <mat-icon aria-hidden="true">search</mat-icon>
        <input
          type="search"
          autocomplete="off"
          [attr.aria-label]="searchLabel()"
          [placeholder]="searchLabel()"
          [value]="filter()"
          (input)="filter.set($any($event.target).value)"
        />
      </label>
    }
    <div
      #list
      class="rows"
      [class.rows--flow]="flow()"
      [class.rows--more]="more() && !flow()"
      [attr.role]="multi() ? 'group' : 'radiogroup'"
      [attr.aria-label]="label()"
      (scroll)="measure()"
    >
      @for (r of shown(); track r.key) {
        @let n = counts()[r.key] ?? 0;
        <label class="row" [class.row--on]="multi() ? n > 0 : r.key === chosen()">
          @if (multi()) {
            <!-- Only a chosen row has the whole stepper; the others have the "+" that chooses one. -->
            <span class="step" [class.step--one]="n === 0">
              @if (n > 0) {
                <button type="button" class="step__b" [attr.aria-label]="'Menos ' + r.title" (click)="step.emit({ key: r.key, delta: -1 })">
                  <mat-icon aria-hidden="true">remove</mat-icon>
                </button>
                <output class="step__n" [attr.aria-label]="n + ' de ' + r.title">{{ n }}</output>
              }
              <button type="button" class="step__b" [attr.aria-label]="(n === 0 ? 'Escolher ' : 'Mais ') + r.title" [disabled]="full()" (click)="step.emit({ key: r.key, delta: 1 })">
                <mat-icon aria-hidden="true">add</mat-icon>
              </button>
            </span>
          } @else {
            <input
              type="radio"
              class="mr-visually-hidden"
              [name]="id"
              [value]="r.key"
              [checked]="r.key === chosen()"
              (change)="pick.emit(r.key)"
            />
            <mat-icon class="row__mark" aria-hidden="true">{{ r.key === chosen() ? 'radio_button_checked' : 'radio_button_unchecked' }}</mat-icon>
          }
          @if (r.art) {
            <app-creature-art [monsterKey]="r.key" />
          }
          <span class="row__text">
            <span class="row__title">{{ r.title }}@if (r.alias) { <span class="row__alias" lang="en">({{ r.alias }})</span> }@if (r.table) { <app-table-mark [fromTable]="true" /> }</span>
            @if (r.table) {
              <span class="row__sub">{{ tableNote }}</span>
            } @else {
              <span class="row__sub">{{ r.subtitle }}</span>
            }
            @if (r.attacks?.length && (attacksAlways() || (multi() ? n > 0 : r.key === chosen()))) {
              <span class="row__attacks">
                @for (a of r.attacks; track a.name) {
                  <span class="row__atk"><b [attr.lang]="a.english ? 'en' : null">{{ a.name }}</b> {{ a.detail }}</span>
                  @if (a.text && (multi() ? n > 0 : r.key === chosen())) {
                    <span class="row__rider" lang="en">{{ a.text }}</span>
                  }
                }
              </span>
            }
          </span>
        </label>
      } @empty {
        <p class="empty">{{ emptyText() }}</p>
      }
    </div>
  `,
  styleUrl: './creature-choice-list.scss',
  host: { '[style.--rows-max]': 'rowsMax()', '[class.flow]': 'flow()' },
})
export class CreatureChoiceList {
  readonly rows = input.required<readonly ChoiceRow[]>();
  readonly chosen = input('');
  /** Several creatures, each with a count (`counts`); the radios become steppers. */
  readonly multi = input(false);
  readonly counts = input<Readonly<Record<string, number | undefined>>>({});
  /** No more can be added: the "+" buttons are off. */
  readonly full = input(false);
  /** The group's name for a screen reader: "Forma". */
  readonly label = input('Criatura');
  /** Every row says what its creature attacks with, so the creatures can be compared; otherwise only the chosen one does. */
  readonly attacksAlways = input(false);
  /** The list shows all its rows and the sheet around it scrolls (a phone's bottom sheet), instead of scrolling inside itself. */
  readonly flow = input(false);
  readonly searchable = input(false);
  readonly searchLabel = input('Buscar');
  readonly emptyText = input('Nenhuma criatura encontrada.');
  readonly pick = output<string>();
  readonly step = output<{ key: string; delta: number }>();

  protected readonly tableNote = TABLE_CREATURE_PICK_NOTE;
  protected readonly id = `creature-choice-${nextId++}`;
  protected readonly filter = signal('');
  protected readonly shown = computed(() => {
    const needle = fold(this.filter());
    return needle
      ? this.rows().filter((r) => fold(`${r.title} ${r.alias ?? ''}`).includes(needle))
      : this.rows();
  });
  /** The tallest the list needs to be: a generous 96 px for each row it shows. */
  protected readonly rowsMax = computed(() => `${Math.max(1, this.shown().length) * 96}px`);
  /** Whether there is more below the last visible row: the fade. */
  protected readonly more = signal(false);
  private readonly list = viewChild.required<ElementRef<HTMLElement>>('list');

  constructor() {
    // The row just chosen opens (its attacks and their text): bring all of it into view, in the list and in the sheet around it.
    effect(() => {
      const key = this.chosen();
      if (!key || this.multi()) {
        return;
      }
      setTimeout(() =>
        this.list().nativeElement.querySelector('.row--on')?.scrollIntoView?.({ block: 'nearest' }),
      );
    });
    afterNextRender(() => {
      this.measure();
      // The sheet around changes size (the keyboard, a rotation): measure again.
      const el = this.list().nativeElement;
      if (typeof ResizeObserver === 'function') {
        new ResizeObserver(() => this.measure()).observe(el);
      }
    });
  }

  protected measure(): void {
    const el = this.list().nativeElement;
    this.more.set(el.scrollHeight - el.scrollTop - el.clientHeight > 2);
  }
}

/** Lower case without accents, so "aranha" finds "Aranha" and "cobra" finds "Serpente venenosa". */
function fold(text: string): string {
  return text.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase().trim();
}
