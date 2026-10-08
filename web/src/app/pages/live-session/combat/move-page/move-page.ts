import {
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
  MoveRefusal,
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
import {
  afterText,
  costTitle,
  indexOptions,
  leftLine,
  provokeWarning,
  provokedBy,
  refusalText,
  trapQuestion,
  verdictFor,
} from '../../../../core/combat/move-plan';
import { ownCombatant, roundLabel } from '../../../../core/combat/combat-view';
import { type MapLayers, NO_LAYERS } from '../../../../core/maps/layers';
import { metersFixed, reachSquares } from '../../../../core/units';
import { tieNumbers } from '../../../../core/format/text';
import {
  CombatMap,
  type CombatMapImage,
  type Reach,
} from '../../../../shared/combat-map/combat-map';
import { LivePill } from '../../../../shared/live-pill/live-pill';
import { MapLayersLegend } from '../../../../shared/map-layers/map-layers-legend';
import { PHONE_QUERY, mediaQuery } from '../../../../shared/map-view/media-query';
import { JumpPanel } from './jump-panel';
import { MoveAdjust } from './move-adjust';
import { MoveStatus, type MoveSummary } from './move-status';
import { type Segment, Segmented } from './segmented';

/** Sides of a square on this page, in pixels: the map is zoomed in so a thumb hits one (E9-05: 30 px). */
const ZOOMS = [22, 30, 40, 52] as const;
const DEFAULT_ZOOM = 1;

/** What "Saltar para cá" asks for. */
export type JumpRequest =
  | { readonly kind: 'long'; readonly square: Square }
  | { readonly kind: 'high'; readonly heightDft: number };

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
  /** A square to start with: where the player dropped their token on the main map (the drop never moves, the page asks). */
  readonly start = input<Square | null>(null);

  /** "Mover para cá": move the combatant to this square. */
  readonly confirm = output<Square>();
  /** "Saltar para cá" / "Saltar 1,8 m para cima". */
  readonly jump = output<JumpRequest>();
  /** "Desengajar (gasta a ação)": takes the action here, so the warning goes. */
  readonly disengage = output<void>();
  /** "Cancelar", or the back arrow. */
  readonly back = output<void>();

  protected readonly chosen = signal<Square | null>(null);
  protected readonly mode = signal<'walk' | 'jump'>('walk');
  protected readonly kind = signal<JumpMode>('long');
  protected readonly height = signal(0);
  protected readonly zoom = signal(DEFAULT_ZOOM);
  /** The question about the known trap is open in the footer. */
  protected readonly trapAsk = signal(false);
  private readonly errorFor = signal<Square | null>(null);
  private readonly scroller = viewChild<ElementRef<HTMLElement>>('scroller');
  private readonly safe = viewChild('safe', { read: ElementRef<HTMLButtonElement> });
  private readonly goButton = viewChild('goButton', { read: ElementRef<HTMLButtonElement> });

  protected readonly phone = mediaQuery(PHONE_QUERY);
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
  protected readonly leftDft = computed(
    () => this.options()?.movementLeftDft ?? this.own()?.movementLeftDft ?? 0,
  );
  protected readonly totalDft = computed(() => this.own()?.speedDft ?? 0);
  protected readonly usedDft = computed(() => this.own()?.movementUsedDft ?? 0);
  protected readonly jumping = computed(() => this.mode() === 'jump' && !!this.jumps());
  protected readonly modes: readonly Segment<'walk' | 'jump'>[] = [
    { value: 'walk', label: 'Andar', icon: 'directions_walk' },
    { value: 'jump', label: 'Saltar', icon: 'north_east' },
  ];
  protected readonly kinds: readonly Segment<JumpMode>[] = [
    { value: 'long', label: 'Distância', icon: 'straighten' },
    { value: 'high', label: 'Altura', icon: 'arrow_upward' },
  ];

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

  /** The reach the map draws: the walk's squares, or the circle of the jump. */
  protected readonly reach = computed<Reach | null>(() => {
    if (this.readOnly()) {
      return null;
    }
    if (this.jumping()) {
      return this.kind() === 'long'
        ? { origin: this.origin(), leftDft: this.cap(), squares: [] }
        : null;
    }
    const options = this.options();
    return {
      origin: this.origin(),
      leftDft: this.leftDft(),
      squares: (options?.reachable ?? []).map((s) => ({ col: s.col, row: s.row })),
    };
  });

  /** The move in words, from the options' cost. */
  protected readonly summary = computed<MoveSummary>(() => {
    const none: MoveSummary = { kind: 'idle', title: '', detail: '', warning: '', trap: '' };
    if (this.jumping()) {
      return this.jumpSummary(none);
    }
    const to = this.chosen();
    const v = this.verdict();
    if (!to || !v) {
      return none;
    }
    if (v.kind === 'ok') {
      const names = provokedBy(v.square, this.encounter().combatants);
      return {
        kind: 'ok',
        title: costTitle(v.square.costDft),
        detail: afterText(this.leftDft(), v.square.costDft),
        warning: names.length > 0 ? provokeWarning(names) : '',
        trap: v.square.knownTrapName,
      };
    }
    if (v.kind === 'unknown') {
      return none;
    }
    const text = refusalText(v, this.leftDft());
    return text ? { kind: 'refused', ...text, warning: '', trap: '' } : none;
  });
  /** The frame on the chosen square: the cost beside it when the move can be made, the
   * refusal's title ("Sem caminho") when it cannot. */
  protected readonly frame = computed(() => {
    const square = this.chosen();
    if (!square || (this.jumping() && this.kind() === 'high')) {
      return null;
    }
    const s = this.summary();
    if (!this.jumping() && this.verdict()?.kind === 'unknown') {
      return { square, refused: false, label: undefined };
    }
    if (s.kind !== 'ok') {
      return {
        square,
        refused: true,
        label: s.title ? s.title.replace('Sem caminho reto', 'Sem caminho') : undefined,
      };
    }
    const v = this.verdict();
    const dft = this.jumping() ? this.jumpCost() : v?.kind === 'ok' ? v.square.costDft : 0;
    return { square, refused: false, label: metersFixed(dft / 10) };
  });
  protected readonly canMove = computed(() => {
    if (this.busy()) {
      return false;
    }
    if (this.jumping()) {
      return this.kind() === 'high'
        ? this.height() > 0
        : this.chosen() !== null && this.summary().kind === 'ok';
    }
    // Without the reach (unread, or the read failed) the server decides and says why not.
    const kind = this.verdict()?.kind;
    return kind === 'ok' || kind === 'unknown';
  });
  protected readonly visibleError = computed(() => {
    const at = this.errorFor();
    const to = this.chosen();
    return (
      this.serverError() !== '' &&
      ((!at && !to) || (!!at && !!to && at.col === to.col && at.row === to.row))
    );
  });
  protected readonly title = computed(
    () => `${this.jumping() ? 'Saltar' : 'Mover'} ${this.own()?.label ?? ''}`,
  );
  protected readonly lead = computed(() => {
    const ask = this.jumping()
      ? this.kind() === 'long'
        ? 'Toque no quadrado onde quer cair.'
        : ''
      : 'Toque num quadrado destacado.';
    const squares = this.jumping() ? null : reachSquares(this.leftDft() / 10);
    return [leftLine(this.leftDft(), this.totalDft(), this.usedDft(), squares), ask]
      .filter(Boolean)
      .join(' ');
  });
  protected readonly leftText = computed(() => metersFixed(this.leftDft() / 10));
  protected readonly limitText = computed(() => metersFixed(this.cap() / 10));
  protected readonly goIcon = computed(() =>
    !this.jumping() ? 'arrow_forward' : this.kind() === 'high' ? 'arrow_upward' : 'north_east',
  );
  protected readonly goLabel = computed(() => {
    if (!this.jumping()) {
      return 'Mover para cá';
    }
    return this.kind() === 'long'
      ? 'Saltar para cá'
      : `Saltar ${metersFixed(this.height() / 10)} para cima`;
  });
  /** What the footer's button says it is for when it cannot be pressed. */
  protected readonly trapName = computed(() => {
    if (this.jumping()) {
      // The walk's reads know the trap squares the character knows: a jump that lands on one asks too.
      const to = this.chosen();
      return this.kind() === 'long' && to
        ? (this.index().reachable.get(to.row * 1000 + to.col)?.knownTrapName ?? '')
        : '';
    }
    const v = this.verdict();
    return v?.kind === 'ok' ? v.square.knownTrapName : '';
  });
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

  /** Scrolls the map so the square is in the middle of the visible part. */
  private centreOn(sq: Square): void {
    const el = this.scroller()?.nativeElement;
    if (!el || this.wide()) {
      return;
    }
    const cell = this.cell();
    el.scrollLeft = Math.max(0, (sq.col + 0.5) * cell - el.clientWidth / 2);
    el.scrollTop = Math.max(0, (sq.row + 0.5) * cell - el.clientHeight / 2);
  }

  /** Brings the square into view when it is not, with a square and a half of margin for its label. */
  private keepInView(sq: Square): void {
    const el = this.scroller()?.nativeElement;
    if (!el || this.wide()) {
      return;
    }
    const cell = this.cell();
    const pad = cell * 1.5;
    const left = sq.col * cell;
    const top = sq.row * cell;
    if (left - pad < el.scrollLeft) {
      el.scrollLeft = Math.max(0, left - pad);
    } else if (left + cell + pad > el.scrollLeft + el.clientWidth) {
      el.scrollLeft = left + cell + pad - el.clientWidth;
    }
    if (top - pad < el.scrollTop) {
      el.scrollTop = Math.max(0, top - pad);
    } else if (top + cell + pad > el.scrollTop + el.clientHeight) {
      el.scrollTop = top + cell + pad - el.clientHeight;
    }
  }

  /** "Voltar" on the trap question: the focus goes back to the button that asked. */
  protected closeTrapAsk(): void {
    this.trapAsk.set(false);
    queueMicrotask(() => this.goButton()?.nativeElement.focus());
  }

  private jumpSummary(none: MoveSummary): MoveSummary {
    const left = this.leftDft();
    const refused = (title: string, detail: string): MoveSummary => ({
      kind: 'refused',
      title,
      detail,
      warning: '',
      trap: '',
    });
    if (this.kind() === 'high') {
      const h = this.height();
      return h > 0
        ? {
            kind: 'ok',
            title: `Saltar ${metersFixed(h / 10)}`,
            detail: `Você sobe sem mudar de quadrado, para agarrar uma borda. ${afterText(left, h)}`,
            warning: '',
            trap: '',
          }
        : refused(
            'Sem salto em altura',
            'Com essa Força e esse movimento, não dá para subir nem um passo de 0,3 m.',
          );
    }
    const to = this.chosen();
    if (!to) {
      return none;
    }
    if (to.col === this.origin().col && to.row === this.origin().row) {
      return refused('Você já está aqui', 'Toque no quadrado onde quer cair.');
    }
    // A jump cannot cross a wall or land on a creature: what the reads show (a wall in the options'
    // refusals, a creature on the square) is refused here, so the button never promises what the server refuses.
    const read = this.verdict();
    if (
      read?.kind === 'refused' &&
      (read.reason === MoveRefusal.WALL || read.reason === MoveRefusal.OCCUPIED)
    ) {
      return refused(
        read.reason === MoveRefusal.WALL ? 'Sem caminho' : 'Ocupado',
        read.reason === MoveRefusal.WALL
          ? 'Um salto não atravessa parede. Escolha outro quadrado.'
          : 'Há alguém nesse quadrado. Escolha onde cair.',
      );
    }
    if (this.encounter().combatants.some((c) => c.placed && c.col === to.col && c.row === to.row)) {
      return refused('Ocupado', 'Há alguém nesse quadrado. Escolha onde cair.');
    }
    const cost = this.jumpCost();
    if (cost > this.cap()) {
      return refused(
        'Longe demais',
        cost > this.limit()
          ? `Seu salto vai até ${metersFixed(this.limit() / 10)}. Escolha um quadrado dentro do círculo.`
          : `Você só tem ${metersFixed(left / 10)} de movimento. Escolha um quadrado dentro do círculo.`,
      );
    }
    const names = read?.kind === 'ok' ? provokedBy(read.square, this.encounter().combatants) : [];
    return {
      kind: 'ok',
      title: `Saltar ${metersFixed(cost / 10)}`,
      detail: `O terreno difícil no caminho não conta. ${afterText(left, cost)}`,
      warning: names.length > 0 ? provokeWarning(names) : '',
      trap: this.trapName() ? this.trapName() : '',
    };
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
  }

  protected setKind(kind: JumpMode): void {
    this.kind.set(kind);
    this.chosen.set(null);
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
    if (this.jumping()) {
      if (this.kind() === 'high') {
        this.jump.emit({ kind: 'high', heightDft: this.height() });
      } else if (to) {
        this.jump.emit({ kind: 'long', square: to });
      }
    } else if (to) {
      this.confirm.emit(to);
    }
  }
}
