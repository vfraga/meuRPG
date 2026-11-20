import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { ACTION_SPENT_WHY, type CombatItemRowVm } from '../../../../core/characters/consumables';
import { cannotActWhy } from '../../../../core/combat/conditions';
import { EconomyMark } from '../../../../shared/economy-shapes/economy-shapes';

let nextId = 0;

/**
 * Why an item row cannot be used: the page is locked, or the item costs the Action and the character cannot act or has spent it.
 * An item that costs none (an oil, a scroll) stays open. The row is disabled with its reason and opens no sheet.
 */
export function itemWhyOf(
  row: CombatItemRowVm,
  own: { readonly conditions?: readonly string[]; readonly actionUsed: boolean },
  locked: string,
): string {
  if (locked || row.economy !== 'action') {
    return locked;
  }
  return cannotActWhy(own) || (own.actionUsed ? ACTION_SPENT_WHY : '');
}

/** The icon of the "Usar" button: the flask of drinking and applying, whatever the item's own icon is. */
const USE_ICON = 'science';

/**
 * One potion or scroll of the combat's "Itens" (board 565-04): the item's icon, its name with the bold count of identical units,
 * one second line with the mark of what using it costs and what it does ("● Ação · cura 2d4 + 2"), and "Usar" in the action
 * column the other tabs share. A row that cannot be used keeps its place and its focus stop, as `ActionRow` does: the mark goes
 * to its spent look, the button is dashed and the reason sits on the row, tied to the button with `aria-describedby`.
 */
@Component({
  selector: 'app-item-row',
  imports: [EconomyMark, MatButtonModule, MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <mat-icon class="row__icon" aria-hidden="true">{{ icon() }}</mat-icon>
    <div class="row__text">
      <span class="row__head">
        <span class="row__name">{{ name() }}</span>
        @if (times() > 1) {
          <span class="row__times"><span aria-hidden="true">×{{ times() }}</span><span class="mr-visually-hidden">, {{ times() }} unidades</span></span>
        }
      </span>
      <span class="row__line">
        @if (economy(); as e) {
          <span class="row__lead"><span class="row__econ" [class.row__econ--spent]="off()"><app-economy-mark [kind]="e" [spent]="off()" />Ação</span>{{ ' ' }}<span class="row__dot" aria-hidden="true">·</span></span>
        }<span class="row__detail">{{ detailText() }}</span>
      </span>
      @if (off() && reason() && !reasonId()) {
        <span class="row__why" [id]="whyId"><mat-icon aria-hidden="true">block</mat-icon>{{ reason() }}</span>
      }
    </div>
    <button
      matButton="outlined"
      type="button"
      class="row__btn"
      [class.row__btn--off]="off()"
      [class.mr-button--off]="off()"
      [disabled]="off() || busy()"
      disabledInteractive
      [attr.aria-label]="buttonLabel()"
      [attr.aria-describedby]="describedBy()"
      (click)="pressed()"
    >
      <mat-icon aria-hidden="true">{{ useIcon }}</mat-icon>{{ ' ' }}Usar
    </button>
  `,
  styleUrl: './item-row.scss',
  host: { '[class.row--off]': 'off()' },
})
export class ItemRow {
  readonly name = input.required<string>();
  /** How many identical units the row stands for ("×2", read as "2 unidades"); 1 for a row of one. */
  readonly times = input(1);
  /** The item's own icon, at the row's left. */
  readonly icon = input.required<string>();
  /** The economy the use spends: the Ação, or `null` for what spends none. */
  readonly economy = input<'action' | null>(null);
  /** What the item does, in lower case, after the mark. */
  readonly detail = input('');
  /** The button's accessible name: "Usar Poção de cura". */
  readonly buttonLabel = input<string | null>(null);
  readonly off = input(false);
  readonly reason = input('');
  readonly busy = input(false);
  /** The id of a line that says the reason for the whole group the row is in (said once there, not on each row); empty when the row says its own. */
  readonly reasonId = input('');

  readonly press = output<void>();

  protected readonly useIcon = USE_ICON;
  /** What the item does; after the cost's dot it starts with a no-break space, so the cost and the dot stay with its first word. */
  protected readonly detailText = computed(() =>
    this.economy() ? `\u00a0${this.detail()}` : this.detail(),
  );
  protected readonly whyId = `item-why-${nextId++}`;
  protected readonly describedBy = computed(() =>
    this.off() && this.reason() ? this.reasonId() || this.whyId : null,
  );

  /** A disabled row keeps its focus stop but does nothing. */
  protected pressed(): void {
    if (!this.off() && !this.busy()) {
      this.press.emit();
    }
  }
}
