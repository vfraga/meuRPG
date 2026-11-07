import {
  Component,
  computed,
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

let nextWhy = 0;

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
          <button #safe matButton="outlined" type="button" (click)="asking.set(false)">Voltar</button>
          <button matButton="outlined" type="button" class="ask__go" (click)="confirm()">Passar o turno</button>
        </div>
      </div>
    } @else if (passing()) {
      <div class="ask" role="alertdialog" aria-labelledby="idle-title" (keydown.escape)="passing.set(false)">
        <p class="ask__title" id="idle-title">Passar a vez de {{ idle() }}, que ainda não agiu?</p>
        <div class="ask__buttons">
          <button #safe matButton="outlined" type="button" (click)="passing.set(false)">Cancelar</button>
          <button matButton="filled" type="button" class="ask__pass" (click)="pass()">Passar a vez</button>
        </div>
      </div>
    } @else {
      <button
        [matButton]="quiet() ? 'outlined' : 'filled'"
        type="button"
        class="next"
        [class.next--off]="off()"
        [disabled]="busy() || off()"
        disabledInteractive
        [attr.aria-describedby]="off() && reason() ? whyId : null"
        (click)="press()"
      >
        <mat-icon aria-hidden="true">skip_next</mat-icon>{{ label() }}
      </button>
      @if (off() && reason()) {
        <p class="why" [id]="whyId"><mat-icon aria-hidden="true">lock</mat-icon>{{ reason() }}</p>
      }
      @if (legendary()) {
        <p class="why" data-testid="legendary-left"><mat-icon aria-hidden="true">crown</mat-icon>{{ legendary() }}</p>
      }
    }
  `,
  styles: `
    :host {
      display: block;
    }

    .next {
      --mat-button-filled-container-height: 52px;
      --mat-button-outlined-container-height: 52px;
      --mat-button-outlined-label-text-color: var(--mr-ink);
      width: 100%;
    }

    // Why the turn cannot pass ("Responda ao pedido abaixo para seguir.") and what legendary actions are left, under the button, at its right edge.
    .why {
      display: flex;
      align-items: center;
      justify-content: flex-end;
      gap: 4px;
      margin: 6px 0 0;
      font-size: var(--mr-font-13);
      line-height: 17px;
      color: var(--mr-ink-muted);

      .mat-icon {
        flex: none;
        width: 14px;
        height: 14px;
        font-size: var(--mr-font-14);
      }
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
      font-size: var(--mr-font-20);
      line-height: 25px;
    }

    .ask__text {
      margin: 0;
      font-size: var(--mr-font-15);
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

        &:not(.ask__pass) {
          background: var(--mr-surface);
        }

        @media (min-width: 768px) {
          --mat-button-outlined-container-height: 44px;
        }
      }

      .ask__pass {
        --mat-button-filled-container-height: 48px;
        flex: 1 1 0;
        min-width: max-content;
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
  /** The button's words: "Próximo turno", or "Encerrar o turno de Pensantus" while the master acts for a character (C8-01). */
  readonly label = input('Próximo turno');
  /** An opportunity attack waits for an answer: the turn cannot pass yet. */
  readonly waiting = input(false);
  /** Why it cannot pass, written under the button and linked to it ("Responda ao pedido abaixo para seguir."), or `''`. */
  readonly why = input('');
  /** "Restam 2 ações lendárias: Dragão vermelho adulto.": what a legendary creature still has to spend, under the button, or `''`. */
  readonly legendary = input('');
  /** A joint turn: each member ends its part from the Turno panel, so the button stays in its place, off, and says so. */
  readonly joint = input(false);
  /** Another panel has the page's one filled button (the opportunity offer): this one is outlined while it is open. */
  readonly quiet = input(false);
  protected readonly off = computed(() => this.waiting() || this.joint());
  protected readonly reason = computed(() =>
    this.joint() ? 'Cada um encerra a parte no painel Turno' : this.why(),
  );
  protected readonly whyId = `next-why-${nextWhy++}`;
  /** Whose turn it is and the round: the question belongs to one turn. */
  readonly turn = input('');
  /** `true` when the master passes the turn although a damage waits. */
  readonly next = output<boolean>();

  /** The player character on turn who has not acted yet ("Toren"), or `''`: passing their turn is asked first. */
  readonly idle = input('');
  protected readonly asking = signal(false);
  protected readonly passing = signal(false);
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
      this.idle();
      this.asking.set(false);
      this.passing.set(false);
    });
  }

  protected press(): void {
    if (this.off()) {
      return;
    }
    if (this.idle() && !this.pendingNote()) {
      this.passing.set(true);
      afterNextRender(() => this.safe()?.nativeElement.focus(), { injector: this.injector });
      return;
    }
    if (!this.pendingNote()) {
      this.next.emit(false);
      return;
    }
    this.asking.set(true);
    afterNextRender(() => this.safe()?.nativeElement.focus(), { injector: this.injector });
  }

  protected pass(): void {
    this.passing.set(false);
    if (!this.waiting() && !this.busy()) {
      this.next.emit(false);
    }
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
