import {
  afterNextRender,
  ChangeDetectionStrategy,
  Component,
  computed,
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
import { isOptionalReaction, playerChoosingText } from '../../../../core/combat/reaction-autopass';
import { otherDeviceText, playerWaitText } from '../../../../core/combat/readied-wait';
import { ReleaseWatch } from '../../../../core/combat/release-watch';
import { moveOffText } from '../../../../core/combat/release-text';
import { readiedRefusalText } from '../../../../core/combat/readied-refusal';
import type { AimNeed } from '../../../../core/combat/readied-aim';
import type { ReactionAnswer } from '../../../../core/combat/combat-client-types';
import {
  NO_PICK,
  type ReleasePick,
  type ReleasePlan,
  aimedUse,
  releaseComplete,
  releaseMapNotice,
} from '../../../../core/combat/readied-release';
import { readiedGoneText, readiedPrompt } from '../../../../core/combat/reaction-readied';
import { ActionKey } from '../../../../core/connect/idempotency';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { ReadiedAimList } from '../readied-aim/readied-aim';

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
  /**
   * "Soltar no mapa" (a held spell that needs the map) and "Mover agora" (a held move): the sheet closes and this opens the map for
   * the release, with the plan the options gave for the spell (`null` for a move). Absent where the map cannot open.
   */
  readonly toMap?: (plan: ReleasePlan | null) => void;
}

/**
 * "O mestre disse que o gatilho aconteceu: ‘Quando o zumbi passar pela porta’. Atacar com a Espada longa agora?" (#582, boards
 * 582-06 to 582-08): the `alertdialog` of a readied action's window, opened by itself for its readier and answered with
 * `AnswerReaction`. "Deixar passar" comes first and has the focus, with no ring until a key moves it; an attack picks its target from the server's list (the one the
 * player named is chosen when it is within reach); another action has one button. A held spell splits its darts or ticks its creatures
 * with the cast screens' own list (#857); a spell that needs the map and a move go there (`toMap`, "Soltar no mapa" and "Mover agora").
 * The master's screen passes the window after 30 s, or after the one wait of 120 s once the player is choosing or on the map, so the
 * footer counts the seconds; if the window closed meanwhile it says so and only closes. The server decides every refusal.
 */
@Component({
  selector: 'app-readied-window-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, ReadiedAimList, SheetFrame],
  templateUrl: './readied-window-sheet.html',
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
    .pair__btn--off {
      border: 1px dashed currentColor;
      background: transparent;
      color: var(--mr-ink-muted);
    }
    .legend {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 8px;
      font-size: 0.875rem;
      line-height: 19px;
    }
    .legend li {
      display: flex;
      gap: 8px;
      align-items: flex-start;
    }
    .legend mat-icon {
      flex: none;
      font-size: 18px;
      width: 18px;
      height: 18px;
      margin-top: 1px; /* with the 19px line: the middle of the icon on the middle of the capitals */
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
  /** A held spell's pick (the darts dealt, the creatures ticked) and what the options say it asks. */
  protected readonly release = signal<ReleasePick>(NO_PICK);
  protected readonly plan = signal<ReleasePlan | null>(null);

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
      case 'optional': {
        const plan = this.plan();
        return !plan || releaseComplete(plan, this.release());
      }
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

  protected readonly isMove = this.data.readied?.kind === ReadiedKind.MOVE;
  /**
   * The window as the combat has it: the master's screen passes it after 30 s, or after the one wait of 120 s once the player opened
   * the map (on this device or another: `rolling`). A held spell or move says what became of it once the window went away.
   */
  private readonly watch = new ReleaseWatch(() => this.data.state.encounter(), {
    windowId: this.data.window.id,
    readiedId: this.data.readied?.id ?? '',
    reactorId: this.data.window.reactorId,
    spellName: this.data.readied?.kind === ReadiedKind.SPELL ? this.data.readied.actionNamePt : '',
    move: this.isMove,
  });
  /**
   * This sheet told the server the player is choosing (the first touch on the list of creatures or darts, see `choosing`). It is the
   * same mark the map sets (`rolling`), so the one 120 s wait begins at whichever comes first.
   */
  private readonly choosingMarked = signal(false);
  /** The mark is set by another device (this sheet did not set it). */
  private readonly markedElsewhere = computed(
    () => this.watch.window()?.rolling === true && !this.choosingMarked(),
  );
  /** The player is on the map in another device: this sheet reads the wait as theirs, not as a prompt of its own. */
  protected readonly otherDevice = computed(() => this.markedElsewhere() && this.mapRelease());
  protected readonly autoPass = computed(() => {
    const seconds = this.watch.secondsLeft();
    if (this.otherDevice()) {
      return otherDeviceText(seconds);
    }
    // The mark also comes from choosing targets, on this device or another: the table waits the longer time for it.
    return this.choosingMarked() || this.markedElsewhere()
      ? playerChoosingText(seconds)
      : playerWaitText(seconds);
  });

  /** The window is no longer in the combat: the time ran out, or the master answered for the player. */
  protected readonly gone = computed(() => !this.busy() && this.watch.gone());
  protected readonly goneView = computed(() =>
    this.data.readied?.kind === ReadiedKind.SPELL || this.isMove ? this.watch.view() : null,
  );
  /** The release opens the map: a spell the options place on it, or a move. */
  protected readonly mapRelease = computed(
    () => !!this.data.toMap && (this.isMove || this.plan()?.onMap === true),
  );
  protected readonly mapNotice = computed(() =>
    this.mapRelease() && this.data.readied
      ? releaseMapNotice(this.plan(), this.data.readied.kind)
      : null,
  );
  /** A move with no speed to spend (a grapple, a speed of 0): "Mover agora" is off, and the line says why. */
  private readonly moveLeft = signal<number | null>(null);
  protected readonly moveOff = computed(() => this.isMove && this.moveLeft() === 0);
  protected readonly moveOffLine = computed(() => {
    const who = this.data.state
      .encounter()
      ?.combatants.find((c) => c.id === this.data.window.reactorId);
    return moveOffText(who?.conditions.includes('condition:grappled') ?? false);
  });
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
  /** The window going away takes the focus to "Fechar", so it does not fall to the page behind. */
  protected readonly focusClose = effect(() => {
    if (this.gone()) {
      untracked(() => afterNextRender(() => this.focusInitial(true), { injector: this.injector }));
    }
  });
  protected readonly goneText = computed(() => {
    const e = this.data.state.encounter();
    const own = e ? ownCombatant(e) : null;
    return readiedGoneText({
      reactionUsed: own?.reactionUsed ?? false,
      down: own?.state === CombatantState.DOWN,
      timedOut: this.watch.secondsLeft() === 0,
      afterMap: this.watch.waitedOnMap(),
    });
  });

  constructor() {
    if (this.isMove && this.data.toMap) {
      void this.readMoveLeft();
    }
    // The safe answer has the focus; the browser draws a ring only when the last input was a key.
    afterNextRender(() => this.focusInitial(), { injector: this.injector });
  }

  /** What the move can spend in this window: the server's own number, so a grapple or a speed of 0 shows before the map opens. */
  private async readMoveLeft(): Promise<void> {
    try {
      const res = await this.api.moves.moveOptions(
        this.data.campaignId,
        this.data.encounterId,
        this.data.window.reactorId,
      );
      this.moveLeft.set(res.movementLeftDft);
    } catch {
      // The map says it itself: the sheet keeps the button on.
    }
  }

  /** "Soltar no mapa" and "Mover agora": the sheet closes and the map opens for the release. */
  protected goToMap(): void {
    const toMap = this.data.toMap;
    if (!toMap || this.busy() || !this.ready() || this.moveOff()) {
      return;
    }
    this.sheet.close();
    toMap(this.plan());
  }

  /** The focus goes to the first answer, with no ring unless `ring` asks for it ("Fechar" of a gone window). */
  private focusInitial(ring = false): void {
    (
      this.host.nativeElement.querySelector('[data-initial-focus]') as HTMLButtonElement | null
    )?.focus(ring ? ({ focusVisible: true } as FocusOptions) : undefined);
  }

  /**
   * The player's first touch on the list of targets or darts: the master's clock would pass the window at 30 s in the middle of the
   * choice, so the server is told they are at it (`MarkReactionRolling`, as the dice step of the other reactions does) and the longer
   * wait counts from now. Best effort: if it fails the 30 s stay.
   */
  protected choosing(): void {
    if (this.choosingTold || !isOptionalReaction(this.data.window)) {
      return;
    }
    this.choosingTold = true;
    void this.tellChoosing();
  }

  private choosingTold = false;

  private async tellChoosing(): Promise<void> {
    try {
      const e = await this.api.attacks.markReactionRolling(
        this.data.campaignId,
        this.data.encounterId,
        this.data.window.id,
      );
      this.data.state.apply(e);
      this.choosingMarked.set(true);
    } catch {
      // The window may have been answered meanwhile; the stream shows the truth.
      this.choosingTold = false;
    }
  }

  /** "Atacar o Zumbi 1" or "Deixar passar": the answer to the window. */
  protected async answer(use: boolean): Promise<void> {
    if (this.busy() || (use && !this.ready())) {
      return;
    }
    const answer: ReactionAnswer | null = !use
      ? { use }
      : this.aims
        ? aimedUse(this.plan(), this.release(), this.target())
        : { use };
    if (!answer) {
      return;
    }
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
      this.error.set(readiedRefusalText(err, this.data.readied, 'responder à reação', this.plan()));
      this.busy.set(false);
    }
  }
}
