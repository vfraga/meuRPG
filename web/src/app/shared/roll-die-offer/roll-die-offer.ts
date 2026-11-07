import { ChangeDetectionStrategy, Component, computed, input, model } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { RollDie } from '../../../gen/meurpg/rules/v1/rules_pb';
import { type DieUses, attackDieCost, rollDieCost } from '../../core/effects/roll-die';
import { OptionCard } from '../option-card/option-card';

let nextId = 0;

/**
 * "Usar <nome>" on a roll sheet (MR-025): a die the character's table features add to this roll (a maneuver on a check, a
 * save, the initiative or a weapon attack). Only the dice that fit the roll are given, so nothing shows when none does. One checkbox for each;
 * a roll takes one die, so ticking one unticks the other. The cost is written under the name. Space ticks.
 *
 * The weapon attack also gives `uses` (the pool each die spends from and what is left): the line then says the uses left, a
 * die with none is off with "Sem usos de ..." under the group, and with `exclusive` and two dice or more the choice is a
 * radio group that starts on "Nenhum" (a card of one line; what it means is read by a screen reader from the group).
 */
@Component({
  selector: 'app-roll-die-offer',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule, OptionCard],
  template: `
    @if (dice().length > 0) {
      <fieldset class="offer" [attr.aria-describedby]="radios() ? group + '-nenhum' : null">
        @if (legend()) {
          <legend class="offer__legend">{{ legend() }}</legend>
        } @else {
          <legend class="mr-visually-hidden">Dados das suas características para esta jogada</legend>
        }
        @if (radios()) {
          <app-option-card
            kind="radio"
            [name]="group"
            title="Nenhum"
            [checked]="picked() === ''"
            (chosen)="picked.set('')"
          />
        }
        @if (radios()) {
          <span class="mr-visually-hidden" [id]="group + '-nenhum'">O ataque sai sem dado de manobra.</span>
        }
        @for (d of dice(); track d.key) {
          <app-option-card
            [kind]="radios() ? 'radio' : 'checkbox'"
            [name]="radios() ? group : ''"
            [title]="'Usar ' + d.namePt"
            [sub]="cost(d)"
            [disabled]="empty(d)"
            [describedBy]="empty(d) ? noneId(d) : ''"
            [checked]="picked() === d.key"
            (chosen)="choose(d)"
          />
        }
        @for (r of spent(); track r.key; let i = $index) {
          <p class="offer__none" [id]="group + '-none-' + i"><mat-icon aria-hidden="true">block</mat-icon>Sem usos de {{ r.name }}.</p>
        }
      </fieldset>
    }
  `,
  styles: `
    :host {
      display: block;
    }

    .offer {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-2);
      min-width: 0;
      margin: 0;
      padding: 0;
      border: 0;
    }

    .offer__legend {
      padding: 0 0 var(--mr-space-2);
      font-size: var(--mr-font-15);
      font-weight: 700;
      line-height: 20px;
    }

    .offer__none {
      display: flex;
      align-items: center;
      gap: var(--mr-space-2);
      margin: 0;
      font-size: var(--mr-font-14);
      line-height: 19px;
      color: var(--mr-ink-muted);

      .mat-icon {
        flex: none;
        width: 18px;
        height: 18px;
        font-size: var(--mr-font-18);
      }
    }
  `,
})
export class RollDieOffer {
  /** The dice that fit this roll (`rollDiceFor`). */
  readonly dice = input<readonly RollDie[]>([]);
  /** The key of the die ticked; empty for none. */
  readonly picked = model('');
  /** The uses left of the pool each die spends from, by the die's key (a weapon attack's `maneuver_dice`). */
  readonly uses = input<Readonly<Record<string, DieUses>>>({});
  /** Two dice or more are one choice among them, "Nenhum" first, instead of boxes that untick each other. */
  readonly exclusive = input(false);
  /** A visible title for the group; without it the group is named for screen readers only. */
  readonly legend = input('');

  protected readonly group = `roll-die-offer-${nextId++}`;
  protected readonly radios = computed(() => this.exclusive() && this.dice().length > 1);
  /** The pools with no use left, once each by the pool's key ("Reserva de Golpes"). */
  protected readonly spent = computed(() => {
    const pools = new Map<string, string>();
    for (const d of this.dice()) {
      const u = this.uses()[d.key];
      if (u && u.left <= 0) {
        pools.set(u.resource || d.key, u.resourceNamePt);
      }
    }
    return [...pools].map(([key, name]) => ({ key, name }));
  });

  protected cost(d: RollDie): string {
    const u = this.uses()[d.key];
    return u ? attackDieCost(d, u) : rollDieCost(d);
  }

  /** The id of the "Sem usos de ..." line that says why the die is off. */
  protected noneId(d: RollDie): string {
    const u = this.uses()[d.key];
    return `${this.group}-none-${this.spent().findIndex((r) => r.key === (u?.resource || d.key))}`;
  }

  protected empty(d: RollDie): boolean {
    const u = this.uses()[d.key];
    return !!u && u.left <= 0;
  }

  protected choose(d: RollDie): void {
    this.picked.set(this.radios() || this.picked() !== d.key ? d.key : '');
  }
}
