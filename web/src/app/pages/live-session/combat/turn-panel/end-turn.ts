import { MASTER_WAIT_NOTE_ID, isMasterWait } from '../../../../core/combat/reactions';
import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { endTurnIsPrimary } from '../../../../core/combat/combat-options';
import { cannotAct } from '../../../../core/combat/conditions';

let nextId = 0;

/**
 * "Encerrar turno" of the player (E6-06, E6-14, timeline.md decision 2): an
 * outline while the Ação or the Ação bônus is still available, the filled
 * button once both are used. With the Ação unused it asks in place, "Ainda
 * tem ação disponível. Encerrar mesmo?", the focus on the safe "Voltar". It
 * stands in the pinned bar on a phone and in the turn card on a laptop. A tap
 * that comes while the turn's options are being read again after a change
 * (`settling`: an action just spent) waits for them, so it never asks about an
 * action the server already counted.
 */
@Component({
  selector: 'app-end-turn',
  imports: [MatButtonModule, MatIconModule],
  template: `
    @if (asking()) {
      <div class="ask" role="alertdialog" aria-labelledby="end-ask">
        <p class="ask__text" id="end-ask">{{ question() }}</p>
        <button #safe matButton="outlined" type="button" class="ask__btn" (click)="asking.set(false)">Voltar</button>
        <button matButton="outlined" type="button" class="ask__btn ask__go" [disabled]="busy()" (click)="confirm()">
          {{ label() }}
        </button>
      </div>
    } @else {
      <button
        matButton="outlined"
        type="button"
        class="end"
        [class.end--filled]="primary()"
        [class.end--block]="block()"
        [class.end--off]="!!waiting()"
        [class.mr-button--off]="!!waiting()"
        [disabled]="busy() || !!waiting() || queued()"
        disabledInteractive
        [attr.aria-busy]="queued() || null"
        [attr.aria-describedby]="waiting() ? (masterWait() ? waitNoteId : id + '-why') : null"
        (click)="press()"
      >
        <mat-icon aria-hidden="true">flag</mat-icon>{{ label() }}
      </button>
      @if (waiting() && !masterWait()) {
        <!-- The reason the turn cannot end, read with the button (it stays reachable by Tab). -->
        <span class="mr-visually-hidden" [id]="id + '-why'">{{ waiting() }}: a vez continua quando responderem.</span>
      }
    }
  `,
  styles: `
    :host {
      display: block;
    }

    .end {
      --mat-button-outlined-container-height: 48px;
      --mat-button-outlined-label-text-color: var(--mr-ink);
    }

    // Waiting for an answer: the dashed, off button of the app, never a click that fails.
    .end--off {
      --mat-button-outlined-disabled-label-text-color: var(--mr-ink-muted);
      --mat-button-outlined-disabled-outline-color: var(--mr-control-line);
      border-style: dashed;
    }

    .end--block {
      width: 100%;
    }

    .end--filled {
      --mat-button-outlined-disabled-label-text-color: var(--mr-on-accent);
      --mat-button-outlined-container-color: var(--mr-accent);
      --mat-button-outlined-label-text-color: var(--mr-on-accent);
      --mat-button-outlined-outline-color: var(--mr-accent);
      background: var(--mr-accent);
    }

    .ask {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      gap: 10px;
    }

    .ask__text {
      flex: 1 1 100%;
      margin: 0;
      font-weight: 700;
    }

    .ask__btn {
      --mat-button-outlined-container-height: 48px;
      @media (min-width: 768px) {
        --mat-button-outlined-container-height: 44px;
      }
      --mat-button-outlined-label-text-color: var(--mr-ink);
      --mat-button-outlined-outline-color: var(--mr-control-line);
      flex: 1 1 0;
      min-width: max-content;
      white-space: nowrap;
    }

    .ask__go {
      --mat-button-outlined-label-text-color: var(--mr-accent-text);
    }
  `,
})
export class EndTurn {
  protected readonly id = `end-turn-${nextId++}`;
  private readonly injector = inject(Injector);

  readonly own = input.required<
    Pick<Combatant, 'actionUsed' | 'bonusActionUsed'> & Partial<Pick<Combatant, 'conditions'>>
  >();
  /** Extra Attack: the attacks left once the first spent the action. */
  readonly attacksLeft = input(0);
  readonly busy = input(false);
  /** The button's words: "Encerrar turno", or "Encerrar a vez do Nanquim" for a creature of the player's (E9-12). */
  readonly label = input('Encerrar turno');
  /** Full width (the phone's bar). */
  readonly block = input(false);
  /** An opportunity attack waits for an answer ("Esperando a reação do mestre"): the turn cannot end. */
  readonly waiting = input('');
  /** The wait is the master's: the turn card says it once, and the button points to it. */
  protected readonly masterWait = computed(() => isMasterWait(this.waiting()));
  protected readonly waitNoteId = MASTER_WAIT_NOTE_ID;
  /** The character is surprised (W7-X): the turn only passes ("Passar o turno"), with nothing to ask, as nothing is available. */
  readonly surprised = input(false);
  /**
   * The turn's options are being read again after a change of the combat (`TurnOptionsState.settling`):
   * `attacksLeft` may still be the one from before the action just spent.
   */
  readonly settling = input(false);

  readonly endTurn = output<void>();

  protected readonly asking = signal(false);
  /** A tap that came while the options were settling: it is decided as soon as they are fresh (a moment). */
  protected readonly queued = signal(false);
  protected readonly primary = computed(
    () =>
      this.surprised() ||
      cannotAct(this.own()) ||
      (endTurnIsPrimary(this.own()) && this.attacksLeft() === 0),
  );
  protected readonly question = computed(() => {
    const left = this.attacksLeft();
    return this.own().actionUsed && left > 0
      ? `Ainda tem ${left} ${left === 1 ? 'ataque' : 'ataques'} desta ação. Encerrar mesmo?`
      : 'Ainda tem ação disponível. Encerrar mesmo?';
  });
  private readonly safe = viewChild('safe', { read: ElementRef<HTMLButtonElement> });

  constructor() {
    effect(() => {
      if (this.queued() && !this.settling()) {
        untracked(() => {
          this.queued.set(false);
          this.decide();
        });
      }
    });
  }

  protected press(): void {
    if (this.settling()) {
      this.queued.set(true);
      return;
    }
    this.decide();
  }

  /** Ends the turn, or asks first when the action (or an attack of it) is still there to use. */
  private decide(): void {
    if (this.waiting()) {
      return;
    }
    if (
      this.surprised() ||
      cannotAct(this.own()) ||
      (this.own().actionUsed && this.attacksLeft() === 0)
    ) {
      this.endTurn.emit();
      return;
    }
    this.asking.set(true);
    afterNextRender(() => this.safe()?.nativeElement.focus(), { injector: this.injector });
  }

  protected confirm(): void {
    this.asking.set(false);
    this.endTurn.emit();
  }
}
