import { Component, computed, input, output } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatIconModule } from '@angular/material/icon';

import { FieldNote } from './field-note';
import type { SelectOption } from './select-field';

let nextId = 0;

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
  imports: [FieldNote, MatFormFieldModule, MatInputModule, MatIconModule, NgTemplateOutlet],
  template: `
    <div class="pick" role="group" [attr.aria-label]="label()" [attr.data-field]="path() || null">
      @for (key of values(); track key) {
        <span class="chip">
          {{ nameOf(key) }}
          <button type="button" class="chip__x" [attr.aria-label]="removeLabel(key)" [disabled]="disabled()" (click)="removed.emit(key)">
            <mat-icon aria-hidden="true">close</mat-icon>
          </button>
        </span>
      }
      @if (available().length > 0 && (max() === 0 || values().length < max())) {
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
        <ng-template #items let-items>
          @for (o of items; track o.value) {
            <option [value]="o.value">{{ o.side ? o.label + ' · ' + o.side : o.label }}</option>
          }
        </ng-template>
      }
    </div>
    <app-field-note [id]="noteId" [issues]="issues()" [hint]="hint()" />
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
      gap: 2px;
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

    .pick__add {
      min-width: 220px;
      flex: 1 1 320px;
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
  /** A line at the end of the add list that cannot be picked. */
  readonly footer = input('');
  readonly added = output<string>();
  readonly removed = output<string>();

  protected readonly noteId = `fn-p${nextId++}`;
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
