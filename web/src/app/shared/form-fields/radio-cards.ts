import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';

import { FieldNote } from './field-note';

let nextId = 0;

/** One card of the group: the key the form stores, its name and the line under it. */
export interface RadioCard {
  readonly value: string;
  readonly label: string;
  readonly hint?: string;
}

/**
 * A short closed list asked as radio cards (the effect menu's `cards` kind: the three kinds of a damage resistance): a
 * title and one line for each choice, the checked one outlined in the accent and marked by its dot, so the state never rides
 * on colour alone. Native radios under the cards, so the arrows and a screen reader work as on any radio group. The first
 * card carries the field's path, where a refusal of the server lands.
 */
@Component({
  selector: 'app-radio-cards',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FieldNote],
  template: `
    <div class="group" role="radiogroup" [attr.aria-label]="label()" [attr.aria-describedby]="noteId">
      <span class="legend" aria-hidden="true">{{ label() }}</span>
      <div class="cards">
        @for (o of options(); track o.value; let first = $first) {
          <label class="card">
            <input
              type="radio"
              class="mr-visually-hidden"
              [name]="name"
              [value]="o.value"
              [checked]="value() === o.value"
              [attr.data-field]="first ? path() || null : null"
              (change)="valueChange.emit(o.value)"
            />
            <span class="dot" aria-hidden="true"></span>
            <span class="text">
              <span class="title">{{ o.label }}</span>
              @if (o.hint) {
                <span class="desc">{{ o.hint }}</span>
              }
            </span>
          </label>
        }
      </div>
    </div>
    <app-field-note [id]="noteId" [issues]="issues()" />
  `,
  styles: `
    :host {
      display: block;
      min-width: 0;
    }

    .group {
      display: flex;
      flex-direction: column;
      gap: 8px;
    }

    .legend {
      font-size: 14px;
      color: var(--mr-ink-muted);
    }

    .cards {
      display: flex;
      flex-direction: column;
      gap: 8px;

      @container (min-width: 520px) {
        flex-direction: row;
      }
    }

    .card {
      position: relative;
      flex: 1 1 0;
      display: flex;
      align-items: flex-start;
      gap: 12px;
      box-sizing: border-box;
      min-height: 56px;
      padding: 10px 14px;
      border: 1px solid var(--mr-control-line);
      border-radius: var(--mr-radius-md);
      background: var(--mr-surface);
      color: var(--mr-ink);
      cursor: pointer;

      &:has(:checked) {
        padding: 9px 13px;
        border: 2px solid var(--mr-accent);
        background: var(--mr-accent-soft);
      }

      &:has(:focus-visible) {
        outline: 2px solid var(--mr-focus);
        outline-offset: 2px;
      }
    }

    .dot {
      flex: none;
      box-sizing: border-box;
      width: 22px;
      height: 22px;
      margin-top: 1px;
      border: 2px solid var(--mr-control-line);
      border-radius: 50%;
      background: var(--mr-surface);
      display: flex;
      align-items: center;
      justify-content: center;

      :checked ~ & {
        border-color: var(--mr-ink);

        &::after {
          content: '';
          width: 10px;
          height: 10px;
          border-radius: 50%;
          background: var(--mr-ink);
        }
      }
    }

    .text {
      display: flex;
      flex-direction: column;
      min-width: 0;
    }

    .title {
      font-size: 16px;
      font-weight: 600;
    }

    .desc {
      font-size: 14px;
      color: var(--mr-ink-muted);
    }
  `,
  host: { style: 'container-type: inline-size' },
})
export class RadioCards {
  readonly label = input.required<string>();
  readonly options = input.required<readonly RadioCard[]>();
  readonly value = input.required<string>();
  readonly path = input('');
  readonly issues = input<readonly string[]>([]);
  readonly valueChange = output<string>();

  /** The group's name for the native radios, and the id the group's description points at. */
  private readonly id = nextId++;
  protected readonly name = `radio-cards-${this.id}`;
  protected readonly noteId = `radio-cards-note-${this.id}`;
}
