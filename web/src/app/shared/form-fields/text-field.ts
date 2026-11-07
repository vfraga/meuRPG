import { Component, computed, input, output } from '@angular/core';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';

import { FieldNote } from './field-note';

let nextId = 0;

/**
 * One text, number or paragraph field of the content editors: the app's outlined field, its path as `data-field` (where a
 * refusal of the server lands, `core/content/content-violations.ts`), the refusal under it and the hint when all is well.
 * The parent owns the value: `valueChange` says what was typed.
 */
@Component({
  selector: 'app-text-field',
  imports: [FieldNote, MatFormFieldModule, MatInputModule],
  template: `
    <mat-form-field appearance="outline" subscriptSizing="dynamic" [floatLabel]="placeholder() ? 'always' : 'auto'" [class.invalid]="invalid()">
      <mat-label>{{ label() }}</mat-label>
      @if (rows() > 0) {
        <textarea
          matInput
          [rows]="rows()"
          [attr.data-field]="path() || null"
          [attr.aria-invalid]="invalid() ? 'true' : null"
          [attr.aria-describedby]="described()"
          [attr.maxlength]="maxlength() || null"
          [value]="value()"
          (input)="valueChange.emit($any($event.target).value)"
        ></textarea>
      } @else {
        <input
          matInput
          autocomplete="off"
          [attr.inputmode]="inputmode() || null"
          [attr.data-field]="path() || null"
          [attr.aria-invalid]="invalid() ? 'true' : null"
          [attr.aria-describedby]="described()"
          [attr.maxlength]="maxlength() || null"
          [attr.placeholder]="placeholder() || null"
          [attr.aria-label]="ariaLabel() || null"
          [disabled]="disabled()"
          [value]="value()"
          (input)="valueChange.emit($any($event.target).value)"
        />
      }
      @if (suffix()) {
        <span matTextSuffix class="suffix">{{ suffix() }}</span>
      }
    </mat-form-field>
    <app-field-note [id]="noteId" [issues]="issues()" [hint]="hint()" />
  `,
  styles: `
    :host {
      display: block;
      min-width: 0;
    }

    mat-form-field {
      width: 100%;
    }

    .invalid {
      --mat-form-field-outlined-outline-color: var(--mr-danger-ink);
      --mat-form-field-outlined-hover-outline-color: var(--mr-danger-ink);
      --mat-form-field-outlined-focus-outline-color: var(--mr-danger-ink);
      --mat-form-field-outlined-label-text-color: var(--mr-danger-ink);
      --mat-form-field-outlined-focus-label-text-color: var(--mr-danger-ink);
    }

    .suffix {
      padding-right: 12px;
      color: var(--mr-ink-muted);
    }
  `,
})
export class TextField {
  readonly label = input.required<string>();
  readonly value = input.required<string>();
  readonly path = input('');
  readonly issues = input<readonly string[]>([]);
  readonly hint = input('');
  readonly suffix = input('');
  /** More than 0 makes it a paragraph field with that many rows. */
  readonly rows = input(0);
  readonly maxlength = input(0);
  readonly inputmode = input('');
  /** What a field left empty means ("da base"); the label then stays above it. */
  readonly placeholder = input('');
  /** The name a screen reader reads when the label shows less than it ("Fixo" in the row of "Pontos de vida"). */
  readonly ariaLabel = input('');
  /** A locked field: it shows its one value and cannot change (a rule fixes it). */
  readonly disabled = input(false);
  /** The id of another element that describes the field (the lock note beside a locked one), read after the note under it. */
  readonly describedBy = input('');
  readonly valueChange = output<string>();

  protected readonly noteId = `fn-${nextId++}`;
  protected readonly invalid = computed(() => this.issues().length > 0);
  protected readonly described = computed(() => {
    const ids = [this.issues().length > 0 || this.hint() ? this.noteId : '', this.describedBy()];
    return ids.filter((i) => i !== '').join(' ') || null;
  });
}
