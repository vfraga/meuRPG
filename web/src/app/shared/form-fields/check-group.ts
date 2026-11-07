import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';

import { CheckRow } from './check-row';
import { FieldNote } from './field-note';

let nextId = 0;

/** One box of the group: the key the form stores and its name. */
export interface CheckOption {
  readonly value: string;
  readonly label: string;
}

/**
 * Several of a short closed list asked as check rows under one label (the effect menu's `rolls`: the rolls a reroll covers),
 * two to a line where there is room, one under another where there is not. The group is named by its label; the note under it
 * says what the choice means, or, when the server refused it, why (the refusal in the danger ink and the label with it, with the
 * alert icon, so colour is never the only sign). The group carries the field's path: a refusal focuses its first box.
 */
@Component({
  selector: 'app-check-group',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CheckRow, FieldNote],
  template: `
    <div class="group" role="group" [attr.aria-labelledby]="labelId" [attr.aria-describedby]="noteId" [attr.data-field]="path() || null">
      <span class="legend" [class.legend--issue]="invalid()" [id]="labelId">{{ label() }}</span>
      <div class="boxes">
        @for (o of options(); track o.value) {
          <app-check-row [label]="o.label" [checked]="values().includes(o.value)" (toggled)="toggle(o.value, $event)" />
        }
      </div>
    </div>
    <app-field-note [id]="noteId" [issues]="issues()" [hint]="hint()" />
  `,
  styles: `
    :host {
      display: block;
      min-width: 0;
    }

    .group {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-1);
    }

    .legend {
      font-weight: 700;
    }

    .legend--issue {
      color: var(--mr-danger-ink);
    }

    .boxes {
      display: grid;
      grid-template-columns: minmax(0, 1fr);
      column-gap: var(--mr-space-4);

      @container (min-width: 520px) {
        grid-template-columns: repeat(2, minmax(0, 1fr));
      }
    }
  `,
  host: { style: 'container-type: inline-size' },
})
export class CheckGroup {
  readonly label = input.required<string>();
  readonly options = input.required<readonly CheckOption[]>();
  readonly values = input.required<readonly string[]>();
  readonly path = input('');
  readonly issues = input<readonly string[]>([]);
  /** The line under the boxes, while nothing is refused. */
  readonly hint = input('');
  /** The checked keys, in the list's order. */
  readonly valuesChange = output<string[]>();

  private readonly id = nextId++;
  protected readonly labelId = `check-group-label-${this.id}`;
  protected readonly noteId = `check-group-note-${this.id}`;
  protected readonly invalid = computed(() => this.issues().length > 0);

  protected toggle(value: string, on: boolean): void {
    this.valuesChange.emit(
      this.options()
        .map((o) => o.value)
        .filter((k) => (k === value ? on : this.values().includes(k))),
    );
  }
}
