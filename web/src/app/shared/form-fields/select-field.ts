import {
  Component,
  ElementRef,
  afterEveryRender,
  computed,
  input,
  output,
  viewChild,
} from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';

import { FieldNote } from './field-note';

let nextId = 0;

export interface SelectOption<T extends string | number = string> {
  readonly value: T;
  readonly label: string;
  readonly disabled?: boolean;
  /** The group the option sits in (an `<optgroup>`); adjacent options with the same group sit in one `<optgroup>`. */
  readonly group?: string;
}

/**
 * A native select in the app's outlined field (the same pattern as the trap panel).
 * It is a real `<select>`, so the phone's own picker opens, with the refusal of the server under it.
 * Options with a `group` sit in an `<optgroup>`; `disabled` turns the whole select off.
 * Values are strings or numbers; `valueChange` gives back the option's own value, not its text.
 */
@Component({
  selector: 'app-select-field',
  imports: [FieldNote, MatFormFieldModule, MatInputModule, NgTemplateOutlet],
  template: `
    <mat-form-field appearance="outline" subscriptSizing="dynamic" [class.invalid]="invalid()">
      <mat-label>{{ label() }}</mat-label>
      <select
        #control
        matNativeControl
        [attr.data-field]="path() || null"
        [disabled]="disabled()"
        [attr.aria-describedby]="described()"
        (change)="pick($any($event.target).selectedIndex)"
      >
        @if (placeholder()) {
          <option value="" [selected]="value() === ''" [disabled]="true">{{ placeholder() }}</option>
        }
        @for (seg of segments(); track $index) {
          @if (seg.group) {
            <optgroup [label]="seg.group">
              <ng-container *ngTemplateOutlet="items; context: { $implicit: seg.items }" />
            </optgroup>
          } @else {
            <ng-container *ngTemplateOutlet="items; context: { $implicit: seg.items }" />
          }
        }
      </select>
    </mat-form-field>
    <ng-template #items let-items>
      @for (o of items; track o.value) {
        <option [value]="o.value" [selected]="o.value === value()" [disabled]="o.disabled ?? false">{{ o.label }}</option>
      }
    </ng-template>
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
  `,
})
export class SelectField<T extends string | number = string> {
  readonly label = input.required<string>();
  readonly options = input.required<readonly SelectOption<T>[]>();
  readonly value = input.required<T | ''>();
  readonly path = input('');
  readonly issues = input<readonly string[]>([]);
  readonly hint = input('');
  /** A disabled first option ("Escolha…") for a value that is not chosen yet. */
  readonly placeholder = input('');
  /** A locked field: it shows its one value and cannot change (a rule fixes it); the line beside it says why. */
  readonly disabled = input(false);
  /** The id of another element that describes the field (the lock note beside a locked one), read after the note under it. */
  readonly describedBy = input('');
  readonly valueChange = output<T>();

  protected readonly noteId = `fn-s${nextId++}`;
  protected readonly invalid = computed(() => this.issues().length > 0);
  protected readonly described = computed(() => {
    const ids = [this.issues().length > 0 || this.hint() ? this.noteId : '', this.describedBy()];
    return ids.filter((i) => i !== '').join(' ') || null;
  });

  /** The options in runs of the same group, in order: an option with no group is drawn outside any `<optgroup>`. */
  protected readonly segments = computed(() => {
    const out: { group: string; items: SelectOption<T>[] }[] = [];
    for (const o of this.options()) {
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

  private readonly control = viewChild<ElementRef<HTMLSelectElement>>('control');

  constructor() {
    // Material's own binding on a native select says `aria-invalid="false"` after ours: the attribute is set last, every render.
    afterEveryRender(() => {
      const el = this.control()?.nativeElement;
      if (el && el.getAttribute('aria-invalid') !== String(this.invalid())) {
        el.setAttribute('aria-invalid', String(this.invalid()));
      }
    });
  }

  protected pick(index: number): void {
    const offset = this.placeholder() ? 1 : 0;
    const option = this.options()[index - offset];
    if (option) {
      this.valueChange.emit(option.value);
    }
  }
}
