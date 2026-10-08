import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  effect,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

/**
 * The master's "Próximo turno" (E6-11, E6-12): the one filled button of the
 * screen, 52px. With a damage still to roll or apply it asks in place, "Há
 * dano sem aplicar. Passar o turno mesmo assim?", the focus on the safe
 * "Voltar"; "Passar o turno" sends the call with the discard flag. It stands
 * in the combat bar on a laptop and at the end of the turn card on a phone.
 */
@Component({
  selector: 'app-next-turn',
  imports: [MatButtonModule, MatIconModule],
  template: `
    @if (asking()) {
      <div class="ask" role="alertdialog" aria-labelledby="next-title">
        <p class="ask__title" id="next-title">Há dano sem aplicar. Passar o turno mesmo assim?</p>
        <p class="ask__text">O dano que ficou esperando é descartado.</p>
        <div class="ask__buttons">
          <button #safe mat-stroked-button type="button" (click)="asking.set(false)">Voltar</button>
          <button mat-stroked-button type="button" class="ask__go" (click)="confirm()">Passar o turno</button>
        </div>
      </div>
    } @else {
      <button mat-flat-button type="button" class="next" [class.next--off]="waiting()" [disabled]="busy() || waiting()" disabledInteractive (click)="press()">
        <mat-icon aria-hidden="true">skip_next</mat-icon>Próximo turno
      </button>
    }
  `,
  styles: `
    :host {
      display: block;
    }

    .next {
      --mat-button-filled-container-height: 52px;
      width: 100%;
    }

    // Waiting for an opportunity attack's answer: the app's dashed, off button.
    .next--off {
      --mat-button-filled-disabled-container-color: var(--mr-surface);
      --mat-button-filled-disabled-label-text-color: var(--mr-ink-muted);
      border: 1px dashed var(--mr-control-line);
    }

    .ask {
      display: flex;
      flex-direction: column;
      gap: 6px;
      box-sizing: border-box;
      padding: 12px 14px;
      border: 1px solid var(--mr-warning-line);
      border-radius: var(--mr-radius-md);
      background: var(--mr-warning-surface);
      color: var(--mr-warning-ink);
    }

    .ask__title {
      margin: 0;
      font-family: var(--mr-font-display);
      font-weight: 700;
      font-size: 20px;
      line-height: 25px;
    }

    .ask__text {
      margin: 0;
      font-size: 15px;
      line-height: 20px;
    }

    // Side by side while they fit, else two full rows of the same height.
    .ask__buttons {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      margin-top: 4px;

      button {
        --mat-button-outlined-container-height: 48px;
        --mat-button-outlined-label-text-color: var(--mr-ink);
        --mat-button-outlined-outline-color: var(--mr-control-line);
        flex: 1 1 0;
        min-width: max-content;
        white-space: nowrap;
        background: var(--mr-surface);

        @media (min-width: 768px) {
          --mat-button-outlined-container-height: 44px;
        }
      }

      .ask__go {
        --mat-button-outlined-label-text-color: var(--mr-danger-ink);
        --mat-button-outlined-outline-color: var(--mr-danger-ink);
      }
    }
  `,
})
export class NextTurn {
  private readonly injector = inject(Injector);

  /** What the turn still owes ("Falta aplicar 5 de dano"), or `null`. */
  readonly pendingNote = input<string | null>(null);
  readonly busy = input(false);
  /** An opportunity attack waits for an answer: the turn cannot pass yet. */
  readonly waiting = input(false);
  /** Whose turn it is and the round: the question belongs to one turn. */
  readonly turn = input('');
  /** `true` when the master passes the turn although a damage waits. */
  readonly next = output<boolean>();

  protected readonly asking = signal(false);
  private readonly safe = viewChild('safe', { read: ElementRef<HTMLButtonElement> });

  constructor() {
    // The question is about the damage and the turn it was asked on: once any of
    // them changes (applied elsewhere, the turn moved, an answer is pending) it
    // is closed, and the master asks again if they still want to.
    effect(() => {
      this.pendingNote();
      this.turn();
      this.waiting();
      this.busy();
      this.asking.set(false);
    });
  }

  protected press(): void {
    if (this.waiting()) {
      return;
    }
    if (!this.pendingNote()) {
      this.next.emit(false);
      return;
    }
    this.asking.set(true);
    afterNextRender(() => this.safe()?.nativeElement.focus(), { injector: this.injector });
  }

  protected confirm(): void {
    if (!this.asking() || !this.pendingNote() || this.waiting() || this.busy()) {
      this.asking.set(false);
      return;
    }
    this.asking.set(false);
    this.next.emit(true);
  }
}
