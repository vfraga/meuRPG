import { isMasterWait } from '../../../../core/combat/reactions';
import { Component, ElementRef, computed, inject, input, output, signal } from '@angular/core';
import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { moveLine } from '../../../../core/combat/move-line';
import { leftSentence, passNote } from '../../../../core/combat/joint-turn';
import type { MineTab } from '../../../../core/combat/mine';
import { keepClear } from '../../../../shared/scroll-clearance/scroll-clearance';
import { EconomyShapes, economyShapes } from '../../../../shared/economy-shapes/economy-shapes';
import { EndPart } from '../joint-turn/end-part';
import { MineTabs } from '../mine-tabs/mine-tabs';
import { EndTurn } from './end-turn';

/**
 * The bar pinned to the bottom of a player's own turn on a phone and a tablet (C4-03, C4-04): "Ainda tem" and the shapes of
 * what the turn still has (a circle for the Ação, a triangle for the Bônus, a square for the Reação, the walker with the metres
 * left), and "Encerrar turno", which is an outline until the Ação and the Bônus are spent (then it is the filled button). It sits
 * at the end of the page's column so that it sticks to the bottom of the screen while the page scrolls, right above the phone's
 * pinned nav (`--mr-session-nav`). "Mover" is the row under the map; from 1024px the shapes, "Mover" and "Encerrar turno" are
 * the turn bar at the top.
 */
@Component({
  selector: 'app-turn-bar',
  imports: [EconomyShapes, EndPart, EndTurn, MineTabs],
  template: `
    @if (tabs().length > 1) {
      <app-mine-tabs [tabs]="tabs()" [selected]="selected()" (select)="select.emit($event)" />
    }
    @if (own(); as me) {
      @if (joint(); as who) {
        @if (!asking()) {
          <p class="what">{{ note(who) }}</p>
        }
        <app-end-part [left]="partLeft()" [busy]="busy()" (endPart)="endTurn.emit()" (asked)="asking.set($event)" />
      } @else {
        <div class="row">
          @if (waiting() && !masterWait()) {
            <p class="what">{{ waiting() }}: a vez continua quando responderem.</p>
          } @else {
            <div class="left">
              <span class="left__lead">Ainda tem</span>
              @if (anyLeft()) {
                <app-economy-shapes variant="inline" [onlyLeft]="true" [shapes]="shapes()" label="Ainda tem" />
              } @else {
                <span class="left__none">Nada mais neste turno</span>
              }
            </div>
          }
          <app-end-turn class="row__end" [own]="me" [attacksLeft]="attacksLeft()" [settling]="optionsSettling()" [busy]="busy()" [waiting]="waiting()" [label]="surprised() ? 'Passar o turno' : 'Encerrar turno'" [surprised]="surprised()" (endTurn)="endTurn.emit()" />
        </div>
      }
    }
  `,
  styles: `
    :host {
      position: sticky;
      bottom: var(--mr-session-nav, 0px);
      z-index: 5;
      display: flex;
      flex-direction: column;
      gap: 8px;
      margin: 0 calc(var(--mr-gutter) * -1);
      padding: 8px var(--mr-gutter);
      border-top: 1px solid var(--mr-line);
      background: var(--mr-surface);

      @media (min-width: 1024px) {
        display: none;
      }
    }

    .what {
      margin: 0;
      font-size: var(--mr-font-14);
      color: var(--mr-ink-muted);

      b {
        color: var(--mr-ink);
      }
    }

    .left {
      display: flex;
      flex: 1 1 0;
      flex-direction: column;
      gap: 2px;
      min-width: 0;
    }

    .left__lead {
      font-size: var(--mr-font-13);
      line-height: 16px;
      color: var(--mr-ink-muted);
      // On the narrowest phones the shapes alone say it (the group is still named "Ainda tem"): the bar gives the page a line back.
      @media (max-width: 359.98px) {
        position: absolute;
        width: 1px;
        height: 1px;
        overflow: hidden;
        clip-path: inset(50%);
        white-space: nowrap;
      }
    }

    .left__none {
      font-size: var(--mr-font-13);
      line-height: 16px;
      font-weight: 700;
    }

    // One slim row: the movement left, when there is one, and "Encerrar turno" at the end. The
    // question "Encerrar mesmo?" takes the whole row while it is open.
    .row {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      gap: 8px 12px;

      .what {
        flex: 1 1 0;
        min-width: 0;
      }

      &:has([role='alertdialog']) .what {
        display: none;
      }
    }

    .row__end {
      flex: none;
      max-width: 100%;
      margin-left: auto;

      &:has([role='alertdialog']) {
        flex: 1 1 100%;
      }
    }
  `,
})
export class TurnBar {
  /** The player's own character while it acts; `null` when only the tabs are drawn (it is not its turn, or a creature's page is open). */
  readonly own = input<Combatant | null>(null);
  /** The tabs of what the player plays (MR-037, E9-12), drawn above when there is more than one. */
  readonly tabs = input<readonly MineTab[]>([]);
  readonly selected = input('');
  readonly busy = input(false);
  /** Extra Attack: the attacks that remain once the action is spent. */
  readonly attacksLeft = input(0);
  readonly attacksPerAction = input(1);
  /** The turn's options are being read again after a change: "Encerrar turno" waits for them (`EndTurn.settling`). */
  readonly optionsSettling = input(false);
  /** An opportunity attack waits for an answer: the list of what is left and the end button say so instead. */
  readonly waiting = input('');
  /** The wait is the master's: the turn card says it once, so the bar does not. */
  protected readonly masterWait = computed(() => isMasterWait(this.waiting()));
  /** The character is surprised (W7-X): the button only passes the turn. */
  readonly surprised = input(false);
  /** No map (RN-25): the movement is a number of meters, said as "Movimento 9,0 m". */
  readonly theatre = input(false);

  /** In a joint turn, who else must end their part (labels; "o mestre" is the hidden one's). */
  readonly joint = input<readonly string[] | null>(null);

  readonly endTurn = output<void>();
  readonly select = output<string>();

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  constructor() {
    // The bar covers the foot of the page: what is focused or scrolled into view stops above it.
    keepClear(() => this.host.nativeElement, 'bottom');
  }

  protected readonly asking = signal(false);
  protected readonly partLeft = computed(() => {
    const c = this.own();
    return c ? leftSentence(c) : '';
  });

  /** "O turno passa quando você e a Brisa encerrarem." */
  protected note(others: readonly string[]): string {
    return passNote(
      others.filter((o) => o !== 'o mestre'),
      others.includes('o mestre'),
    );
  }

  /** The shapes of what the turn still has: the Action, the Bonus action, the Reaction and, on a map, the metres left (C4-03). */
  protected readonly shapes = computed(() => {
    const c = this.own();
    if (!c) {
      return [];
    }
    const line = moveLine(c, false);
    return economyShapes(c, {
      attacksLeft: this.attacksLeft(),
      attacksPerAction: this.attacksPerAction(),
      moveText: line.canMove ? line.left : null,
    });
  });
  protected readonly anyLeft = computed(() => this.shapes().some((s) => !s.spent));
}
