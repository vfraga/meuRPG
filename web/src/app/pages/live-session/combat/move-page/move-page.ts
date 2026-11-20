import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  afterNextRender,
  computed,
  effect,
  input,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  type Encounter,
  type GetMoveOptionsResponse,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { JumpLimits } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import type { Vision } from '../../../../core/maps/vision';
import { type Square, stepSquare } from '../../../../core/combat/combat-grid';
import {
  HEIGHT_STEP_DFT,
  type JumpMode,
  lineLengthDft,
  limitFor,
  maxHeight,
  stepHeight,
} from '../../../../core/combat/jump-plan';
import { indexOptions, trapQuestion, verdictFor } from '../../../../core/combat/move-plan';
import { ownCombatant, roundLabel } from '../../../../core/combat/combat-view';
import { type MapLayers, NO_LAYERS } from '../../../../core/maps/layers';
import { metersFixed } from '../../../../core/units';
import { tieNumbers } from '../../../../core/format/text';
import {
  CombatMap,
  type CombatMapImage,
  type Reach,
} from '../../../../shared/combat-map/combat-map';
import { LivePill } from '../../../../shared/live-pill/live-pill';
import { MapLayersLegend } from '../../../../shared/map-layers/map-layers-legend';
import { mediaQuery } from '../../../../shared/map-view/media-query';
import type { MoveReleaseView } from '../../../../core/combat/move-release';
import { ReleaseBandView } from '../release-band/release-band';
import { JumpPanel } from './jump-panel';
import { MoveAdjust } from './move-adjust';
import {
  type JumpScene,
  type ScrollBox,
  centreScroll,
  canMoveOf,
  errorShown,
  frameOf,
  goLabelOf,
  jumpSummary,
  keepInViewScroll,
  KINDS,
  cappedMovement,
  goIconOf,
  type JumpRequest,
  jumpRequestOf,
  MODES,
  leadOf,
  trapNameOf,
  reachOf,
  walkSummary,
} from './move-page-view';
import { MoveDrag } from './move-drag';
import { MoveStatus, type MoveSummary } from './move-status';
import { Segmented } from '../../../../shared/segmented/segmented';

/** Sides of a square on this page, in pixels: the map is zoomed in so a thumb hits one (E9-05: 30 px). */
const ZOOM_PX = { far: 22, near: 30, closer: 40, closest: 52 } as const;
const ZOOMS = [ZOOM_PX.far, ZOOM_PX.near, ZOOM_PX.closer, ZOOM_PX.closest] as const;
const DEFAULT_ZOOM = 1;

export type { JumpRequest } from './move-page-view';

/**
 * "Mover" and "Saltar" (E6-10, E9-05, E9-06, E9-13; RN-21, MR-034): a full page
 * where the map is the control. The reach comes from the server
 * (`GetMoveOptions`): the squares it can go to are tinted inside the dashed
 * circle of the movement left, each with its cost, and a square it refuses says
 * why (wall, enemy, taken, too costly) without naming what is in the way. The
 * browser only draws and words that: it never works out a cost, a wall or a
 * cover. The player taps a square (or moves the choice with the arrows below
 * the map, or the arrow keys), reads the move in a live region and confirms with
 * "Mover para cá", under the map, where the thumb is. A move that may provoke an
 * opportunity attack warns first, and so does one into a trap the character
 * knows. "Saltar" swaps the walk for a long jump (a square, inside the circle of
 * the server's limit) or a high one (a height, in 0,3 m steps).
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-move-page',
  imports: [
    CombatMap,
    JumpPanel,
    LivePill,
    MapLayersLegend,
    MatButtonModule,
    MatIconModule,
    MoveAdjust,
    MoveStatus,
    ReleaseBandView,
    Segmented,
  ],
  templateUrl: './move-page.html',
  styleUrl: './move-page.scss',
})
export class MovePage {
  readonly encounter = input.required<Encounter>();
  readonly image = input.required<CombatMapImage>();
  readonly mapName = input('');
  readonly sessionNumber = input(0);
  /** "Ver mapa": the same page without the reach and the buttons. */
  readonly readOnly = input(false);
  /** What the player sees of the map with the fog on: the page draws it instead of the image (MR-036). */
  readonly fog = input<Vision | null>(null);
  /** The master's "Abrir mapa": hidden combatants are drawn too. */
  readonly isMaster = input(false);
  readonly busy = input(false);
  /** A refusal from the server ("Longe demais…"), shown like a local one. */
  readonly serverError = input('');
  /** The last move stopped before a locked door (RN-26): the notice says why, and the map keeps drawing "Porta fechada". */
  readonly lockedDoor = input(false);
  /** Where the combatant can go (`GetMoveOptions`); `null` until it is read, or when it failed. */
  readonly options = input<GetMoveOptionsResponse | null>(null);
  readonly optionsFailed = input(false);
  /** The map's layers, once read. */
  readonly layers = input<MapLayers>(NO_LAYERS);
  /** How far and how high it can jump this turn; unset when the sheet has no Força. */
  readonly jumps = input<JumpLimits | undefined>(undefined);
  /** "Desengajar" is still possible: the action is free and it was not taken. */
  readonly canDisengage = input(false);
  /** Who moves: one of the player's creatures (E9-12); empty for the player's own character. */
  readonly moverId = input('');
  /** A readied move released off the readier's turn (#856): the band, the note, the master's footer and what became of the window. */
  readonly release = input<MoveReleaseView | null>(null);
  /** A square to start with: where the player dropped their token on the main map (the drop never moves, the page asks). */
  readonly start = input<Square | null>(null);

  /** "Mover para cá": move the combatant to this square. */
  readonly confirm = output<Square>();
  /** "Saltar para cá" / "Saltar 1,8 m para cima". */
  readonly jump = output<JumpRequest>();
  /** "Desengajar (gasta a ação)": takes the action here, so the warning goes. */
  readonly disengage = output<void>();
  /** What the page reads from the server (`GetMoveOptions`): the squares of a long jump (`runningStart` as the limits say), or `null` for a walk. */
  readonly readJump = output<{ readonly runningStart: boolean } | null>();
  /** "Cancelar", or the back arrow. */
  readonly back = output<void>();

  protected readonly chosen = signal<Square | null>(null);
  protected readonly mode = signal<'walk' | 'jump'>('walk');
  protected readonly kind = signal<JumpMode>('long');
  protected readonly height = signal(0);
  protected readonly zoom = signal(DEFAULT_ZOOM);
  /** Desengajar was chosen for the jump: the action is spent with the jump, not before it. */
  private readonly disengageChoice = signal(false);
  /** The question about the known trap is open in the footer. */
  protected readonly trapAsk = signal(false);
  private readonly errorFor = signal<Square | null>(null);
  private readonly scroller = viewChild<ElementRef<HTMLElement>>('scroller');
  private readonly safe = viewChild('safe', { read: ElementRef<HTMLButtonElement> });
  private readonly goButton = viewChild('goButton', { read: ElementRef<HTMLButtonElement> });

  /** From 1024px the map fits the left column and the controls sit beside it. */
  protected readonly wide = mediaQuery('(min-width: 1024px)');
  protected readonly cell = computed(() => ZOOMS[this.zoom()]);
  protected readonly round = computed(() => tieNumbers(roundLabel(this.encounter().round)));
  protected readonly sessionLabel = computed(() => tieNumbers(`Sessão ${this.sessionNumber()}`));
  /** The height of a high jump has no map: the page is one column, with the content where the eye starts. */
  protected readonly soloHeight = computed(() => this.jumping() && this.kind() === 'high');
  protected readonly own = computed(() => {
    const e = this.encounter();
    const id = this.moverId();
    return (id ? e.combatants.find((c) => c.id === id) : null) ?? ownCombatant(e);
  });
  protected readonly origin = computed<Square>(() => {
    const own = this.own();
    return own ? { col: own.col, row: own.row } : { col: 0, row: 0 };
  });
  /** The server's answer for this origin, but never more than the combatant's live movement: after a move the options
   * are the old ones until they are fetched again, and the sheet would keep saying the distance from before. */
  protected readonly leftDft = computed(() =>
    this.release()
      ? // The budget of a readied move is the server's own: the movement of the turn that ended is not what it spends.
        (this.options()?.movementLeftDft ?? 0)
      : cappedMovement(this.options()?.movementLeftDft, this.own()?.movementLeftDft),
  );
  protected readonly totalDft = computed(() =>
    this.release() ? this.leftDft() : (this.own()?.speedDft ?? 0),
  );
  protected readonly usedDft = computed(() =>
    this.release() ? 0 : (this.own()?.movementUsedDft ?? 0),
  );
  /** The window went away under the page: nothing can be moved, and "Voltar à sessão" is the one button. */
  protected readonly goneView = computed(() => this.release()?.gone ?? null);
  protected readonly jumping = computed(() => this.mode() === 'jump' && !!this.jumps());
  protected readonly modes = MODES;
  protected readonly kinds = KINDS;

  protected readonly index = computed(() => indexOptions(this.options()));
  protected readonly verdict = computed(() => {
    const to = this.chosen();
    return to ? verdictFor(this.index(), this.origin(), to) : null;
  });
  /** The limit of the jump that is picked (the server's, running or standing). */
  protected readonly limit = computed(() => {
    const jumps = this.jumps();
    return jumps ? limitFor(jumps, this.kind()) : 0;
  });
  /** What a jump can really do: the server's limit, and no more than the movement left (a jump costs its length). */
  protected readonly cap = computed(() => Math.min(this.limit(), this.leftDft()));
  protected readonly atMin = computed(
    () => this.height() <= Math.min(HEIGHT_STEP_DFT, maxHeight(this.cap())),
  );
  protected readonly atMax = computed(() => this.height() >= maxHeight(this.cap()));
  /** The line a long jump draws, to say what it costs (display only; the server decides). */
  protected readonly jumpCost = computed(() => {
    const to = this.chosen();
    return to ? lineLengthDft(this.origin(), to) : 0;
  });

  protected readonly drag = new MoveDrag({
    options: this.options,
    encounter: this.encounter,
    jumping: this.jumping,
    chosen: this.chosen,
    verdict: this.verdict,
    origin: this.origin,
    leftDft: this.leftDft,
  });

  /** The reach the map draws: the walk's squares, or the circle of the jump. */
  protected readonly reach = computed<Reach | null>(() =>
    reachOf(
      this.readOnly(),
      this.jumping(),
      this.kind(),
      this.origin(),
      this.cap(),
      this.leftDft(),
      this.options(),
    ),
  );

  /** The move in words, from the options' cost. */
  protected readonly summary = computed<MoveSummary>(() =>
    this.jumping()
      ? jumpSummary(this.jumpScene())
      : walkSummary(this.chosen(), this.verdict(), this.leftDft(), this.encounter().combatants),
  );
  private readonly jumpScene = computed<JumpScene>(() => ({
    kind: this.kind(),
    left: this.leftDft(),
    leftHere: this.own()?.movementLeftDft ?? 0,
    height: this.height(),
    chosen: this.chosen(),
    origin: this.origin(),
    verdict: this.verdict(),
    cap: this.cap(),
    limit: this.limit(),
    cost: this.jumpCost(),
    combatants: this.encounter().combatants,
    trapName: this.trapName(),
  }));
  /** The frame on the chosen square: the cost beside it when the move can be made, the
   * refusal's title ("Sem caminho") when it cannot. */
  protected readonly frame = computed(() =>
    frameOf(
      this.chosen(),
      this.summary(),
      this.jumping(),
      this.kind(),
      this.verdict(),
      this.jumpCost(),
    ),
  );
  /** A long jump that leaves an enemy's reach: Desengajar can be chosen and spent with it. */
  protected readonly jumpProvokes = computed(
    () =>
      this.jumping() &&
      this.kind() === 'long' &&
      this.summary().kind === 'ok' &&
      !!this.summary().warning,
  );
  protected readonly disengaging = computed(
    () => this.disengageChoice() && this.jumpProvokes() && this.canDisengage(),
  );
  protected readonly canMove = computed(
    () =>
      !this.goneView() &&
      canMoveOf(
        this.busy(),
        this.jumping(),
        this.kind(),
        this.height(),
        this.chosen(),
        this.summary().kind,
        this.verdict(),
      ),
  );
  protected readonly visibleError = computed(() =>
    errorShown(this.serverError(), this.errorFor(), this.chosen()),
  );
  protected readonly title = computed(
    () => `${this.jumping() ? 'Saltar' : 'Mover'} ${this.own()?.label ?? ''}`,
  );
  protected readonly lead = computed(() =>
    leadOf(this.jumping(), this.kind(), this.leftDft(), this.totalDft(), this.usedDft()),
  );
  protected readonly leftText = computed(() => metersFixed(this.leftDft() / 10));
  protected readonly limitText = computed(() => metersFixed(this.cap() / 10));
  protected readonly goIcon = computed(() => goIconOf(this.jumping(), this.kind()));
  protected readonly goLabel = computed(() =>
    goLabelOf(this.jumping(), this.kind(), this.disengaging(), this.height()),
  );
  /** What the footer's button says it is for when it cannot be pressed. */
  protected readonly trapName = computed(() =>
    trapNameOf(this.jumping(), this.kind(), this.chosen(), this.index(), this.verdict()),
  );
  protected readonly trapText = computed(() => trapQuestion(this.trapName()));

  constructor() {
    // The map is bigger than the frame it scrolls in: open with the token in the middle of it.
    afterNextRender(() => this.centreOn(this.origin()));
    // A chosen square never scrolls out of sight (or under the app bar).
    effect(() => {
      const to = this.chosen();
      if (to) {
        untracked(() => this.keepInView(to));
      }
    });
    // A drop on the main map starts here, on the square it was dropped on.
    effect(() => {
      const start = this.start();
      if (start) {
        untracked(() => this.chosen.set(start));
      }
    });
    // A refusal belongs to the square it came for.
    effect(() => {
      if (this.serverError()) {
        const at = untracked(() => this.chosen());
        this.errorFor.set(at);
      }
    });
    // A locked door stopped the move: the square chosen is behind us now, so the choice is cleared (and "Mover para cá" with it).
    effect(() => {
      if (this.lockedDoor()) {
        untracked(() => this.chosen.set(null));
      }
    });
    // A long jump asks the server for its own squares (and who they provoke); a walk or a high jump asks for the walk's.
    effect(() => {
      const long = this.jumping() && this.kind() === 'long';
      const running = this.jumps()?.runningStart ?? false;
      untracked(() => this.readJump.emit(long ? { runningStart: running } : null));
    });
    // The high jump starts at the most it can do (E9-06: 1,8 m).
    effect(() => {
      const max = maxHeight(this.cap());
      untracked(() => this.height.set(max));
    });
    // The question about a trap closes when another square is chosen, and
    // opens with the focus on the safe answer.
    effect(() => {
      this.chosen();
      untracked(() => this.trapAsk.set(false));
    });
    effect(() => {
      if (this.trapAsk()) {
        queueMicrotask(() => this.safe()?.nativeElement.focus());
      }
    });
  }

  /** Scrolls the map to where `target` says, given the part of it the scroller shows (nothing in the wide layout). */
  private scrollTo(target: (box: ScrollBox) => { left: number; top: number }): void {
    const el = this.scroller()?.nativeElement;
    if (!el || this.wide()) {
      return;
    }
    const to = target({
      left: el.scrollLeft,
      top: el.scrollTop,
      width: el.clientWidth,
      height: el.clientHeight,
    });
    el.scrollLeft = to.left;
    el.scrollTop = to.top;
  }

  /** Scrolls the map so the square is in the middle of the visible part. */
  private centreOn(sq: Square): void {
    this.scrollTo((box) => centreScroll(sq, this.cell(), box));
  }

  /** Brings the square into view when it is not. */
  private keepInView(sq: Square): void {
    this.scrollTo((box) => keepInViewScroll(sq, this.cell(), box));
  }

  /** "Voltar" on the trap question: the focus goes back to the button that asked. */
  protected closeTrapAsk(): void {
    this.trapAsk.set(false);
    queueMicrotask(() => this.goButton()?.nativeElement.focus());
  }

  protected choose(square: Square): void {
    if (this.jumping() && this.kind() === 'high') {
      return;
    }
    this.chosen.set(square);
  }

  protected nudge(key: string): void {
    const e = this.encounter();
    this.choose(stepSquare(this.chosen() ?? this.origin(), key, e.gridColumns, e.gridRows));
  }

  protected setMode(mode: 'walk' | 'jump'): void {
    this.mode.set(mode);
    this.chosen.set(null);
    this.disengageChoice.set(false);
  }

  /** "Desengajar (gasta a ação)": a walk takes the action at once; a jump chooses it, to be spent with the jump. */
  protected chooseDisengage(): void {
    if (this.jumping()) {
      this.disengageChoice.set(true);
      return;
    }
    this.disengage.emit();
  }

  protected takeBackDisengage(): void {
    this.disengageChoice.set(false);
  }

  protected setKind(kind: JumpMode): void {
    this.kind.set(kind);
    this.chosen.set(null);
    this.disengageChoice.set(false);
    this.height.set(maxHeight(Math.min(limitFor(this.jumps()!, kind), this.leftDft())));
  }

  protected step(direction: 1 | -1): void {
    this.height.update((h) => stepHeight(h, direction, this.cap()));
  }

  protected zoomBy(delta: 1 | -1): void {
    this.zoom.update((z) => Math.min(ZOOMS.length - 1, Math.max(0, z + delta)));
  }

  protected go(): void {
    if (!this.canMove()) {
      return;
    }
    // A square inside a trap the character knows asks first, for a jump too.
    if (this.trapName() && !this.trapAsk() && !(this.jumping() && this.kind() === 'high')) {
      this.trapAsk.set(true);
      return;
    }
    this.commit();
  }

  /** "Mover assim mesmo" (after the trap question), or the button itself. */
  protected commit(): void {
    if (!this.canMove()) {
      return;
    }
    const to = this.chosen();
    if (!this.jumping()) {
      if (to) {
        this.confirm.emit(to);
      }
      return;
    }
    const request = jumpRequestOf(this.kind(), this.height(), to, this.disengaging());
    if (request) {
      this.jump.emit(request);
    }
  }
}
