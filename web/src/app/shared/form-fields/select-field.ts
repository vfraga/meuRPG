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
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import { FieldNote } from './field-note';

let nextId = 0;

/** Whether the browser draws a select's list from our markup and styles (`appearance: base-select`). */
function supportsBaseSelect(): boolean {
  return typeof CSS !== 'undefined' && CSS.supports('appearance', 'base-select');
}

export interface SelectOption<T extends string | number = string> {
  readonly value: T;
  readonly label: string;
  readonly disabled?: boolean;
  /** The group the option sits in (an `<optgroup>`); adjacent options with the same group sit in one `<optgroup>`. */
  readonly group?: string;
  /** A line at the option's right edge ("do efeito 1"); a browser that draws only text reads it after the label. */
  readonly side?: string;
  /** A line of explanation that cannot be picked: it wraps, and is drawn smaller and muted. */
  readonly note?: boolean;
}

/** How the header of a group looks: an icon before its name, in the accent. */
export interface SelectGroup {
  readonly icon?: string;
  readonly accent?: boolean;
}

/**
 * A native select in the app's outlined field (the same pattern as the trap panel).
 * It is a real `<select>`, so the phone's own picker opens, with the refusal of the server under it.
 * Options with a `group` sit in an `<optgroup>`; `disabled` turns the whole select off.
 * A group with an icon, an option with a `side` text, or a `footer` make the list rich where the browser has a customizable select
 * (`appearance: base-select`, asked of `CSS.supports`): the header carries its icon, the side text sits at the right and the footer is the list's last line.
 * Elsewhere the same select is a plain list that draws only text: the group is its label with no icon, the option reads "label · side"
 * and the footer is a line that cannot be picked.
 * Values are strings or numbers; `valueChange` gives back the option's own value, not its text.
 */
@Component({
  selector: 'app-select-field',
  imports: [FieldNote, MatFormFieldModule, MatIconModule, MatInputModule, NgTemplateOutlet],
  template: `
    <mat-form-field appearance="outline" subscriptSizing="dynamic" [class.invalid]="invalid()">
      <mat-label>{{ label() }}</mat-label>
      <select
        #control
        matNativeControl
        [class.rich]="rich()"
        [attr.data-field]="path() || null"
        [disabled]="disabled()"
        [attr.aria-describedby]="described()"
        (change)="pick($any($event.target).selectedIndex)"
      >
        @if (placeholder()) {
          <option class="placeholder" value="" [selected]="value() === ''" [disabled]="true">{{ placeholder() }}</option>
        }
        @for (seg of segments(); track $index) {
          @if (seg.group) {
            <optgroup [label]="seg.group" [class.accent]="groups()[seg.group]?.accent">
              @if (rich()) {
                <legend>
                  @if (groups()[seg.group]?.icon; as icon) {
                    <mat-icon aria-hidden="true">{{ icon }}</mat-icon>
                  }
                  {{ seg.group }}
                </legend>
              }
              <ng-container *ngTemplateOutlet="items; context: { $implicit: seg.items }" />
            </optgroup>
          } @else {
            <ng-container *ngTemplateOutlet="items; context: { $implicit: seg.items }" />
          }
        }
        @if (footer()) {
          <option class="footer" value="" [disabled]="true">{{ footer() }}</option>
        }
      </select>
    </mat-form-field>
    <ng-template #items let-items>
      @for (o of items; track o.value) {
        <option
          [value]="o.value"
          [attr.label]="rich() || !o.side ? null : o.label + ' · ' + o.side"
          [attr.data-side]="o.side || null"
          [selected]="o.value === value()"
          [disabled]="o.disabled ?? false"
          [class.note]="o.note ?? false"
        >{{ o.label }}</option>
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

    // A value wider than the field ends in an ellipsis, never cut under the arrow.
    select {
      text-overflow: ellipsis;
    }

    // The customizable select (Chromium 135 and up): the list is ours to draw. Anywhere else the rules below do not apply.
    @supports (appearance: base-select) {
      select.rich,
      select.rich::picker(select) {
        appearance: base-select;
      }

      select.rich {
        width: 100%;
        border: 0;
        padding: 0;
        background: transparent;
        color: inherit;
        font: inherit;

        &::picker-icon {
          display: none;
        }
      }

      select.rich::picker(select) {
        margin-top: var(--mr-space-1);
        padding: 0;
        border: 1px solid var(--mr-line);
        border-radius: var(--mr-radius-md);
        background: var(--mr-surface);
        color: var(--mr-ink);
      }

      select.rich option {
        display: flex;
        align-items: center;
        gap: var(--mr-space-3);
        min-height: 44px;
        padding: 0 var(--mr-space-4);
        color: var(--mr-ink);
        font-size: var(--mr-font-16);

        &:checked {
          background: var(--mr-accent-soft);
          color: var(--mr-ink);
          font-weight: 700;
        }

        &::checkmark {
          display: none;
        }

        &.placeholder {
          display: none;
        }

        &.note {
          display: block;
          min-height: 0;
          padding-block: var(--mr-space-2) var(--mr-space-3);
          border-bottom: 1px solid var(--mr-line);
          color: var(--mr-ink-muted);
          font-size: var(--mr-font-14);
          line-height: 18px;
        }

        // The line stays at the foot of the list while the options scroll under it.
        &.footer {
          position: sticky;
          bottom: 0;
          background: var(--mr-surface);
          min-height: 0;
          padding-block: var(--mr-space-2);
          border-top: 1px solid var(--mr-line);
          color: var(--mr-ink-muted);
          font-size: var(--mr-font-14);
        }
      }

      select.rich option[data-side]::after {
        content: attr(data-side);
        margin-left: auto;
        color: var(--mr-ink-muted);
        font-size: var(--mr-font-14);
        font-weight: 400;
      }

      select.rich optgroup legend {
        display: flex;
        align-items: center;
        gap: 6px;
        padding: var(--mr-space-2) var(--mr-space-4);
        color: var(--mr-ink-muted);
        font-size: var(--mr-font-14);
        font-weight: 700;

        .mat-icon {
          width: 18px;
          height: 18px;
          font-size: var(--mr-font-18);
        }
      }

      select.rich optgroup.accent legend {
        color: var(--mr-accent-text);
      }
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
  /** How a group's header looks, by the group's name. */
  readonly groups = input<Readonly<Record<string, SelectGroup>>>({});
  /** A line at the end of the list that cannot be picked ("A escolha do talento aparece nos efeitos que vêm depois dela."). */
  readonly footer = input('');
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

  private readonly customizable = supportsBaseSelect();

  /** Whether the list is drawn by us: the browser has a customizable select and there is a group icon, a side text or a footer. */
  protected readonly rich = computed(
    () =>
      this.customizable &&
      (this.footer() !== '' ||
        Object.values(this.groups()).some((g) => g.icon) ||
        this.options().some((o) => o.side)),
  );

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
    if (option && !option.note) {
      this.valueChange.emit(option.value);
    }
  }
}
