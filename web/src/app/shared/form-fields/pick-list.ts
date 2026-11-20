import { Component, computed, input, output } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatIconModule } from '@angular/material/icon';
import { MatMenuModule } from '@angular/material/menu';

import { FieldNote } from './field-note';
import type { SelectOption } from './select-field';

let nextId = 0;

const TABLE_WORDS = ' (da mesa)';

/** The words of a chip whose key is not among the options: its label, and what its button says. */
export interface ChipName {
  readonly label: string;
  readonly remove: string;
}

/**
 * "Idiomas", "Ferramentas", "Magias": several values of a closed list, as chips that can be taken off, with a select to add
 * one more (the list without what is already chosen). Each chip is a 44 px target; the order is the order they were added.
 */
@Component({
  selector: 'app-pick-list',
  imports: [
    FieldNote,
    MatFormFieldModule,
    MatInputModule,
    MatIconModule,
    MatMenuModule,
    NgTemplateOutlet,
  ],
  template: `
    <div class="pick" role="group" [attr.aria-label]="label()" [attr.data-field]="path() || null">
      @for (key of values(); track key) {
        <span class="chip" [class.chip--square]="menuAdd()" [class.chip--bad]="isFlagged(key)" [attr.data-flagged]="isFlagged(key) ? '' : null">
          @if (isFlagged(key)) {
            <mat-icon class="chip__bad" aria-hidden="true">error</mat-icon>
          }
          {{ chipName(key) }}
          @if (isTable(key)) {
            <span class="mr-tag">Da mesa</span>
          }
          @if (badgeOf(key); as badge) {
            <span class="mr-tag mr-tag--compact">{{ badge }}</span>
          }
          @if (isFlagged(key)) {
            <span class="mr-visually-hidden">, com problema</span>
          }
          <button type="button" class="chip__x" [attr.aria-label]="removeLabel(key)" [disabled]="disabled()" (click)="removed.emit(key)">
            <mat-icon aria-hidden="true">close</mat-icon>
          </button>
        </span>
      }
      @if (!stacked()) {
        <ng-container *ngTemplateOutlet="adder" />
      }
      @if (count()) {
        <span class="pick__count">{{ count() }}</span>
      }
    </div>
    @if (stacked()) {
      <app-field-note [id]="noteId" [issues]="issues()" />
      <div class="pick pick--add">
        <ng-container *ngTemplateOutlet="adder" />
      </div>
      <app-field-note [id]="noteId + '-hint'" [hint]="hint()" />
    } @else {
      <app-field-note [id]="noteId" [issues]="issues()" [hint]="hint()" />
    }
    <ng-template #adder>
      @if (available().length > 0 && (max() === 0 || values().length < max())) {
        @if (menuAdd()) {
          <button type="button" class="pick__addbtn" [disabled]="disabled()" [matMenuTriggerFor]="addMenu" [attr.aria-describedby]="describedBy()">
            <mat-icon aria-hidden="true">add</mat-icon>{{ addLabel() }}
          </button>
          <mat-menu #addMenu="matMenu" class="pick-menu">
            @for (o of available(); track o.value) {
              <button type="button" mat-menu-item [disabled]="o.disabled" (click)="added.emit(o.value)">
                <span class="pick-menu__row">
                  <span class="pick-menu__name">{{ chipLabel(o) }}</span>
                  @if (originOf(o); as origin) {
                    <span class="mr-tag mr-tag--compact">{{ origin }}</span>
                  }
                </span>
              </button>
            }
          </mat-menu>
        } @else {
        <mat-form-field class="pick__add" appearance="outline" subscriptSizing="dynamic">
          <mat-label>{{ addLabel() }}</mat-label>
          <select matNativeControl [disabled]="disabled()" [attr.aria-describedby]="describedBy()" (change)="add($event)">
            <option value=""></option>
            @for (seg of segments(); track $index) {
              @if (seg.group) {
                <optgroup [label]="seg.group">
                  <ng-container *ngTemplateOutlet="items; context: { $implicit: seg.items }" />
                </optgroup>
              } @else {
                <ng-container *ngTemplateOutlet="items; context: { $implicit: seg.items }" />
              }
            }
            @if (footer()) {
              <option value="" [disabled]="true">{{ footer() }}</option>
            }
          </select>
        </mat-form-field>
        }
      }
    </ng-template>
    <ng-template #items let-items>
      @for (o of items; track o.value) {
        <option [value]="o.value">{{ o.side || o.badge ? o.label + ' · ' + (o.side || o.badge) : o.label }}</option>
      }
    </ng-template>
  `,
  styles: `
    :host {
      display: block;
    }

    .pick {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      gap: var(--mr-space-2);
    }

    .chip {
      display: inline-flex;
      align-items: center;
      gap: var(--mr-space-2);
      min-height: 46px;
      box-sizing: border-box;
      padding: 0 0 0 14px;
      border: 1px solid var(--mr-control-line);
      border-radius: var(--mr-radius-pill);
      background: var(--mr-surface);
      color: var(--mr-ink);
      font-size: var(--mr-font-16);
    }

    // The 44 px target of design.md, flush with the chip's edge.
    .chip__x {
      // The chip's gap is 8 px; the cross stays 2 px from what comes before it.
      margin-left: -6px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 44px;
      height: 44px;
      padding: 0;
      border: 0;
      border-radius: 50%;
      background: transparent;
      color: var(--mr-ink-muted);
      cursor: pointer;

      &:hover:not(:disabled) {
        color: var(--mr-accent-text);
      }

      &:disabled {
        opacity: 0.4;
        cursor: not-allowed;
      }

      .mat-icon {
        width: 18px;
        height: 18px;
        font-size: var(--mr-font-18);
      }
    }

    // The chip a refusal is about: the danger border and its icon, so the sentence under the chips has something to point at.
    .chip--bad {
      border-color: var(--mr-danger-ink);
      box-shadow: inset 0 0 0 1px var(--mr-danger-ink);
    }

    .chip--square {
      border-radius: var(--mr-radius-sm);
    }

    .chip__bad {
      flex: none;
      width: 18px;
      height: 18px;
      margin-right: -4px;
      font-size: var(--mr-font-18);
      color: var(--mr-danger-ink);
    }

    // The board's "Adicionar opção": an outlined button with a plus, in the row of the chips.
    .pick__addbtn {
      display: inline-flex;
      align-items: center;
      gap: var(--mr-space-2);
      min-height: 46px;
      box-sizing: border-box;
      padding: 0 16px;
      border: 1px solid var(--mr-control-line);
      border-radius: var(--mr-radius-sm);
      background: var(--mr-surface);
      color: var(--mr-accent-text);
      font: inherit;
      font-size: var(--mr-font-16);
      font-weight: 700;
      cursor: pointer;

      &:disabled {
        opacity: 0.4;
        cursor: not-allowed;
      }

      .mat-icon {
        width: 20px;
        height: 20px;
        font-size: var(--mr-font-20);
      }
    }

    // A choice of the menu: the name, and its origin at the right edge with a gap that never closes.
    .pick-menu__row {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--mr-space-3);
      width: 100%;
    }

    .pick-menu__name {
      min-width: 0;
      overflow-wrap: anywhere;
    }

    .pick__add {
      min-width: 220px;
      flex: 1 1 220px;
      max-width: 320px;
    }

    // "1 de 10": at the end of the row, apart from the helper sentence.
    .pick__count {
      margin-left: auto;
      font-size: var(--mr-font-14);
      font-weight: 700;
      color: var(--mr-ink-muted);
      white-space: nowrap;
    }

    .pick--add {
      margin-top: var(--mr-space-2);
    }
  `,
})
export class PickList {
  readonly label = input.required<string>();
  readonly addLabel = input.required<string>();
  readonly options = input.required<readonly SelectOption[]>();
  readonly values = input.required<readonly string[]>();
  readonly path = input('');
  readonly issues = input<readonly string[]>([]);
  readonly hint = input('');
  /** The most it takes; 0 is no limit. */
  readonly max = input(0);
  /** Nothing can be added or taken off; the hint says why. */
  readonly disabled = input(false);
  /** Words for the keys the options do not hold (a pick of a choice that is gone), by key. */
  readonly names = input<Readonly<Record<string, ChipName>>>({});
  /** The keys of the chips a refusal is about: they are drawn with the danger border and icon, and the sentence stands under the chips. */
  readonly flagged = input<readonly string[]>([]);
  /** A count for the end of the row ("1 de 10"), apart from the hint. */
  readonly count = input('');
  /** The add control is a button that opens the system's menu panel (each choice with its origin), not a select: the board's "Adicionar opção". */
  readonly menuAdd = input(false);
  /** A line at the end of the add list that cannot be picked. */
  readonly footer = input('');
  readonly added = output<string>();
  readonly removed = output<string>();

  protected readonly noteId = `fn-p${nextId++}`;
  /** A refusal that names chips: its sentence comes right under the chips, and the select under the sentence. */
  protected readonly stacked = computed(
    () => this.flagged().length > 0 && this.issues().length > 0,
  );

  protected chipLabel(o: SelectOption): string {
    return o.label.endsWith(TABLE_WORDS) ? o.label.slice(0, -TABLE_WORDS.length) : o.label;
  }

  /** Where a choice comes from: "Opção 2" for a sibling, "Da mesa", or "SRD". */
  protected originOf(o: SelectOption): string {
    return o.badge || o.side || (o.label.endsWith(TABLE_WORDS) ? 'Da mesa' : 'SRD');
  }

  protected isFlagged(key: string): boolean {
    return this.flagged().includes(key);
  }
  protected readonly available = computed(() =>
    this.options().filter((o) => !this.values().includes(o.value)),
  );

  /** The values still free in runs of the same group, in order (an option with no group is drawn outside any `<optgroup>`). */
  protected readonly segments = computed(() => {
    const out: { group: string; items: SelectOption[] }[] = [];
    for (const o of this.available()) {
      const group = o.group ?? '';
      const last = out.at(-1);
      if (last && last.group === group) {
        last.items.push(o);
      } else {
        out.push({ group, items: [o] });
      }
    }
    return out;
  });

  protected describedBy(): string | null {
    return this.issues().length > 0 || this.hint() !== '' ? this.noteId : null;
  }

  /** A table entry's name, its "(da mesa)" taken off: the badge says it. */
  protected isTable(key: string): boolean {
    return this.nameOf(key).endsWith(TABLE_WORDS);
  }

  protected chipName(key: string): string {
    const name = this.nameOf(key);
    return name.endsWith(TABLE_WORDS) ? name.slice(0, -TABLE_WORDS.length) : name;
  }

  protected badgeOf(key: string): string {
    return this.options().find((o) => o.value === key)?.badge ?? '';
  }

  protected nameOf(key: string): string {
    return this.options().find((o) => o.value === key)?.label ?? this.names()[key]?.label ?? key;
  }

  protected removeLabel(key: string): string {
    return this.names()[key]?.remove ?? `Tirar ${this.nameOf(key)}`;
  }

  protected add(event: Event): void {
    const select = event.target as HTMLSelectElement;
    if (select.value !== '') {
      this.added.emit(select.value);
    }
    select.value = '';
  }
}
