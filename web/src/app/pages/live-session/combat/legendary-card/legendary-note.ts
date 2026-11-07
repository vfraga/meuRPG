import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant, Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { legendaryRecovered } from '../../../../core/combat/legendary-card';

/** A line cut at its count ("3 de 3") so the count reads bold: `[text, bold]` pieces. */
function pieces(line: string): readonly (readonly [string, boolean])[] {
  return line
    .split(/(\d+ de \d+)/)
    .filter((part) => part !== '')
    .map((part) => [part, /^\d+ de \d+$/.test(part)] as const);
}

/**
 * What the master's card of a legendary creature says on the creature's own turn: the actions
 * came back and the legendary card is gone, so the card of the NPC tells what it has and that legendary actions are
 * spent on another creature's turn, never on its own. Nothing for a creature without legendary actions or off turn.
 */
@Component({
  selector: 'app-legendary-note',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule],
  template: `
    @if (note(); as n) {
      <div class="note" data-testid="legendary-recovered">
        <p class="note__line">
          <mat-icon aria-hidden="true">crown</mat-icon>
          <span>@for (p of actions(); track $index) { @if (p[1]) { <b>{{ p[0] }}</b> } @else { {{ p[0] }} } }</span>
        </p>
        @if (n.resistance) {
          <p class="note__line">
            <mat-icon aria-hidden="true">shield</mat-icon>
            <span>@for (p of resistance(); track $index) { @if (p[1]) { <b>{{ p[0] }}</b> } @else { {{ p[0] }} } }</span>
          </p>
        }
        <p class="note__text">{{ n.note }}</p>
      </div>
    }
  `,
  styles: `
    :host {
      display: block;
    }

    .note {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-2);
    }

    p {
      margin: 0;
    }

    .note__line {
      display: flex;
      align-items: flex-start;
      gap: var(--mr-space-2);
      font-size: var(--mr-font-16);
      line-height: 22px;

      .mat-icon {
        flex: none;
        width: 20px;
        height: 20px;
        margin-top: 1px;
        font-size: var(--mr-font-20);
      }
    }

    .note__text {
      font-size: var(--mr-font-14);
      line-height: 20px;
      color: var(--mr-ink-muted);
    }
  `,
})
export class LegendaryNote {
  readonly encounter = input.required<Encounter>();
  readonly subject = input.required<Combatant>();

  protected readonly note = computed(() => legendaryRecovered(this.encounter(), this.subject()));
  protected readonly actions = computed(() => pieces(this.note()?.actions ?? ''));
  protected readonly resistance = computed(() => pieces(this.note()?.resistance ?? ''));
}
