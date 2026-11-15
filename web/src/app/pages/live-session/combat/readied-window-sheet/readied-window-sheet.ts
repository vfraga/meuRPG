import {
  afterNextRender,
  ChangeDetectionStrategy,
  Component,
  computed,
  DestroyRef,
  effect,
  ElementRef,
  inject,
  Injector,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { ReactionWindow, ReadiedAction } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatantState, ReadiedKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../../../../core/format/article';
import { CombatClient } from '../../../../core/combat/combat-client';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ownCombatant } from '../../../../core/combat/combat-view';
import { playerAutoPassText, playerSecondsLeft } from '../../../../core/combat/reaction-autopass';
import { readiedRefusalText } from '../../../../core/combat/readied-refusal';
import type { AimNeed } from '../../../../core/combat/readied-aim';
import { readiedGoneText, readiedPrompt } from '../../../../core/combat/reaction-readied';
import { ActionKey } from '../../../../core/connect/idempotency';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { ReadiedAimList } from '../readied-aim/readied-aim';

/** How often the prompt's countdown redraws, in ms. */
const CLOCK_TICK_MS = 1000;
/** What the page hands the prompt of a readied action's window. */
export interface ReadiedWindowSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly window: ReactionWindow;
  readonly round: number;
  /** The row the window answers (`Combatant.readied` of its reactor); `undefined` when the caller cannot read it. */
  readonly readied: ReadiedAction | undefined;
  readonly reactorLabel: string;
  readonly state: CombatState;
}

/**
 * "O mestre disse que o gatilho aconteceu: ‘Quando o zumbi passar pela porta’. Atacar com a Espada longa agora?" (#582, boards
 * 582-06 to 582-08): the `alertdialog` of a readied action's window, opened by itself for its readier and answered with
 * `AnswerReaction`. "Deixar passar" comes first and has the focus; an attack picks its target from the server's list (the one the
 * player named is chosen when it is within reach); another action has one button. A move and a spell cannot be released from here
 * yet (`releasable`): the prompt says so and only lets them pass. The master's screen passes the window after 30 s, so the footer
 * counts the seconds; if the window closed meanwhile it says so and only closes. The server decides every refusal.
 */
@Component({
  selector: 'app-readied-window-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, ReadiedAimList, SheetFrame],
  template: `
    <app-sheet-frame
      icon="alarm"
      [title]="view.title"
      [subtitle]="view.subtitle"
      [phone]="inSheet"
      [closable]="false"
      [focusableBody]="true"
    >
      <div class="body">
        @if (error()) {
          <div class="mr-notice mr-notice--danger" role="alert">
            <mat-icon aria-hidden="true">error</mat-icon>
            <p>{{ error() }}</p>
          </div>
        }
        @if (gone()) {
          <p class="what" role="status">{{ goneText() }}</p>
        } @else {
          @if (useHeld()) {
            <p class="what" role="status">
              A sua jogada espera as reações de outros. Assim que responderem, ela acontece.
            </p>
          }
          <p class="what">
            <b>{{ view.lead }}</b> {{ view.quote }}. {{ view.question }}
          </p>
          @if (aims) {
            <app-readied-aim
              [campaignId]="data.campaignId"
              [encounterId]="data.encounterId"
              [readied]="data.readied!"
              [encounter]="data.state.encounter()"
              [reactorId]="data.window.reactorId"
              [(need)]="need"
              [(target)]="target"
            />
          }
          <p class="small">{{ view.note }}</p>
          @if (view.limit) {
            <p class="missing" id="readied-limit">
              <mat-icon aria-hidden="true">block</mat-icon>{{ view.limit }}
            </p>
          }
        }
      </div>
      <div foot>
        @if (gone()) {
          <button matButton="outlined" type="button" class="foot__one" data-initial-focus (click)="sheet.close()">
            Fechar
          </button>
        } @else {
          <p class="small foot__clock" role="status">{{ autoPass() }}</p>
          <div class="pair">
            <button
              matButton="outlined"
              type="button"
              class="pair__btn"
              data-initial-focus
              [disabled]="busy()"
              (click)="answer(false)"
            >
              {{ view.declineLabel }}
            </button>
            @if (view.releasable) {
              <button
                matButton="filled"
                type="button"
                class="pair__btn"
                [disabled]="!ready() || busy()"
                disabledInteractive
                (click)="answer(true)"
              >
                {{ busy() ? 'Usando…' : useLabel() }}
              </button>
            }
          </div>
        }
      </div>
    </app-sheet-frame>
  `,
  styleUrls: [
    '../resource-pick/resource-sheet.scss',
    '../feature-window-sheet/feature-window-sheet.scss',
  ],
  // The board (582-06) keeps the two answers side by side at 390 px; the shared pair stacks them under 380 px of room.
  styles: `
    .pair {
      grid-template-columns: 1fr 1fr;
    }
    .pair:has(.pair__btn:only-child) {
      grid-template-columns: 1fr;
    }
    .pair__btn {
      white-space: normal;
      line-height: 1.2;
    }
  `,
})
export class ReadiedWindowSheet {
  private readonly api = inject(CombatClient);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly sheet = injectSheet<ReadiedWindowSheetData, void>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly view = readiedPrompt(
    this.data.readied,
    this.data.round,
    this.data.reactorLabel,
  );
  /** An attack or a spell of the window is aimed at a creature of the list. */
  protected readonly aims =
    this.data.readied?.kind === ReadiedKind.ATTACK || this.data.readied?.kind === ReadiedKind.SPELL;
  protected readonly need = signal<AimNeed>('loading');
  protected readonly target = signal<string | null>(null);

  /** "Atacar o Zumbi 1" once a creature is chosen; "Atacar" before it. */
  protected readonly useLabel = computed(() => {
    const id = this.target();
    const label = this.data.state.encounter()?.combatants.find((c) => c.id === id)?.label;
    if (!label) {
      return this.view.useLabel;
    }
    const male = article(label) !== 'a';
    return this.data.readied?.kind === ReadiedKind.SPELL
      ? `Soltar ${male ? 'no' : 'na'} ${label}`
      : `${this.view.useLabel} ${male ? 'o' : 'a'} ${label}`;
  });
  /** The answer may be sent: an attack needs a creature chosen (or the one named at Ready, when the list could not be read). */
  protected readonly ready = computed(() => {
    switch (this.need()) {
      case 'required':
        return this.target() !== null;
      case 'optional':
        return true;
      case 'failed':
        return (this.data.readied?.targetCombatantId ?? '') !== '';
      default:
        return !this.aims;
    }
  });

  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  private readonly key = new ActionKey();

  /** The master's screen passes the window after 30 s without an answer, like any optional reaction. */
  private readonly openedAt = Date.now();
  private readonly clock = signal(this.openedAt);
  private readonly clockTimer = setInterval(() => this.clock.set(Date.now()), CLOCK_TICK_MS);
  protected readonly autoPass = computed(() =>
    playerAutoPassText(playerSecondsLeft(this.openedAt, this.clock())),
  );

  /** The window is no longer in the combat: the time ran out, or the master answered for the player. */
  protected readonly gone = computed(
    () =>
      !this.busy() &&
      !(this.data.state.encounter()?.reactionWindows ?? []).some(
        (w) => w.id === this.data.window.id,
      ),
  );
  /**
   * The use was sent and its roll was held for other windows: the window leaving the combat is then the use, not a time out. The
   * window's `answer_now` is the master's alone, so the readier is not told when the hold ends: both answers stay and the server
   * refuses them (`REACTION_PENDING`) for as long as it holds.
   */
  protected readonly useHeld = signal(false);
  /** Closes by itself when a held use went through. */
  protected readonly closeWhenDone = effect(() => {
    if (this.useHeld() && this.gone()) {
      untracked(() => this.sheet.close());
    }
  });
  /** When the window was seen gone: the countdown stops there, so the text does not change as the clock goes on. */
  private readonly goneAt = signal<number | null>(null);
  protected readonly markGone = effect(() => {
    if (this.gone() && untracked(this.goneAt) === null) {
      this.goneAt.set(Date.now());
    }
  });
  /** The window going away takes the focus to "Fechar", so it does not fall to the page behind. */
  protected readonly focusClose = effect(() => {
    if (this.gone()) {
      untracked(() => afterNextRender(() => this.focusInitial(), { injector: this.injector }));
    }
  });
  protected readonly goneText = computed(() => {
    const e = this.data.state.encounter();
    const own = e ? ownCombatant(e) : null;
    const at = this.goneAt();
    return readiedGoneText({
      reactionUsed: own?.reactionUsed ?? false,
      down: own?.state === CombatantState.DOWN,
      // This screen's own clock (the wire has no time for the window): it started when the prompt opened.
      timedOut: at !== null && playerSecondsLeft(this.openedAt, at) === 0,
    });
  });

  constructor() {
    inject(DestroyRef).onDestroy(() => clearInterval(this.clockTimer));
    // The safe answer has the focus, and its ring: the sheet opens for a keyboard-style answer.
    afterNextRender(() => this.focusInitial(), { injector: this.injector });
  }

  private focusInitial(): void {
    (
      this.host.nativeElement.querySelector('[data-initial-focus]') as HTMLButtonElement | null
    )?.focus({ focusVisible: true } as FocusOptions);
  }

  /** "Atacar o Zumbi 1" or "Deixar passar": the answer to the window. */
  protected async answer(use: boolean): Promise<void> {
    if (this.busy() || (use && !this.ready())) {
      return;
    }
    const target = this.target();
    const answer = !use
      ? { use }
      : this.aims
        ? { use, ...(target ? { creatureIds: [target] } : {}), die: { inApp: true as const } }
        : { use };
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.attacks.answerReaction(
        this.data.campaignId,
        this.data.encounterId,
        this.data.window.id,
        answer,
        this.key.keyFor({ window: this.data.window.id, answer }),
      );
      this.key.renew();
      this.data.state.apply(res.encounter);
      this.busy.set(false);
      if (use && res.result?.used === false) {
        // The roll waits for other reactions: the window stays open, and so does this prompt.
        this.useHeld.set(true);
        return;
      }
      this.sheet.close();
    } catch (err) {
      this.error.set(readiedRefusalText(err, this.data.readied, 'responder à reação'));
      this.busy.set(false);
    }
  }
}
