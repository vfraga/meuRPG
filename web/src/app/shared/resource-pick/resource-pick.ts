import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/** A radio card of a resource dialog's list. */
export interface PickRow {
  readonly id: string;
  readonly title: string;
  readonly sub: string;
  /** Why it cannot be chosen, written in the card; empty when it can. */
  readonly blocked: string;
  /** The card is off with nothing written in it (a limit reached: the count says it); the others stay as they are. */
  readonly off?: boolean;
}

let nextId = 0;

/** From this many cards the list scrolls inside itself. */
const SCROLLS_FROM = 8;

/**
 * The list of a resource dialog (PM-07c 9, 10, 12): one radio card for each creature or each slot level, with a line of
 * detail ("Ferida · a 1,5 m", "custa 3 pontos") and, for a card that cannot be chosen, the reason written in it with a
 * dashed frame (never colour alone). Native radios under the cards, so the arrows move the choice and a screen reader
 * reads "1 de 3"; the chosen card has the 2px accent frame and a check. A disabled card is not in the tab order but
 * is still read. With `multi` the cards are check boxes ("Em quem · de 1 a 3 aliados"), and a list of eight or more
 * scrolls inside itself.
 */
@Component({
  selector: 'app-resource-pick',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule],
  template: `
    <fieldset class="pick">
      <legend class="pick__legend" [class.mr-visually-hidden]="legendHidden()">{{ label() }}</legend>
      <div class="pick__list" [class.pick__list--scrolls]="scrolls()" [attr.tabindex]="scrolls() ? 0 : null">
        @for (r of rows(); track r.id) {
          <label class="opt" [class.opt--on]="isOn(r.id)" [class.opt--off]="!!r.blocked || !!r.off">
            <input
              [type]="multi() ? 'checkbox' : 'radio'"
              class="mr-visually-hidden"
              [name]="name"
              [checked]="isOn(r.id)"
              [disabled]="!!r.blocked || !!r.off"
              (change)="pick.emit(r.id)"
            />
            <span class="opt__radio" [class.opt__radio--box]="multi()" aria-hidden="true">
              @if (multi() && isOn(r.id)) {
                <mat-icon>check</mat-icon>
              }
            </span>
            <span class="opt__text">
              <b>{{ r.title }}</b>
              @if (r.sub) {
                <span class="opt__sub">{{ r.sub }}</span>
              }
              @if (r.blocked) {
                <span class="opt__why"><mat-icon aria-hidden="true">block</mat-icon>{{ r.blocked }}</span>
              }
            </span>
          </label>
        } @empty {
          <p class="pick__empty">{{ empty() }}</p>
        }
      </div>
    </fieldset>
  `,
  styleUrl: './resource-pick.scss',
})
export class ResourcePick {
  readonly rows = input.required<readonly PickRow[]>();
  readonly chosen = input<string | null>(null);
  /** Several may be chosen (check boxes, the ids in `picked`), instead of one (radio cards, `chosen`). */
  readonly multi = input(false);
  readonly picked = input<readonly string[]>([]);
  /** The group's name ("Quem tocar"). */
  readonly label = input.required<string>();
  readonly legendHidden = input(false);
  /** What the list says when it has no rows. */
  readonly empty = input('Não há ninguém ao alcance.');
  /** The id of the card the person toggled (multi) or chose. */
  readonly pick = output<string>();
  /** A long list scrolls inside itself, so the buttons under it stay in reach. */
  protected readonly scrolls = computed(() => this.rows().length >= SCROLLS_FROM);
  protected readonly name = `resource-pick-${nextId++}`;

  protected isOn(id: string): boolean {
    return this.multi() ? this.picked().includes(id) : id === this.chosen();
  }
}
