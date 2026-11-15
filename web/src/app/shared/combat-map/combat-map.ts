import { Component, computed, input, output, signal, viewChild, ElementRef } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';
import { CreatureSize } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  DFT_PER_SQUARE,
  type Square,
  squareAt,
  squareCenter,
  stepSquare,
} from '../../core/combat/combat-grid';
import type { DoorSquare, MapLayers } from '../../core/maps/layers';
import { combatantInitial, isPlayer } from '../../core/combat/combat-view';
import { isCreature } from '../../core/combat/creature-names';
import { CombatantToken } from '../combatant-token/combatant-token';
import type { Vision } from '../../core/maps/vision';
import { FogBase } from '../fog-map/fog-base';
import { DoorPicks } from '../map-layers/door-picks';
import { MapLayersOverlay } from '../map-layers/map-layers';
import { gridPath, watchUnit } from './measure-unit';
import { type PickMark, type PickNote, PickLayer } from './pick-layer';
import { costAbove } from './cost-above';
import { MapPairs } from './map-pairs';
import { type TokenDrop, TokenDrag } from './token-drag';
import { SaddleIcon } from '../saddle-icon/saddle-icon';

export type { PickMark, PickNote } from './pick-layer';

/** The map's picture: its URL and size (the frame is reserved from it). */
export interface CombatMapImage {
  readonly url: string;
  readonly width: number;
  readonly height: number;
}

/** What the server said a combatant reaches (`GetMoveOptions`): the square it
 * starts from, the radius of the circle in tenths of a foot (the movement it has
 * left, or a jump's limit) and the squares it can go to. The map only draws it. */
export interface Reach {
  readonly origin: Square;
  readonly leftDft: number;
  readonly squares: readonly Square[];
}

/** A square the person chose; `refused` when the server said it cannot be
 * moved to, and `label` the cost ("2,1 m") written beside it. */
export interface Chosen {
  readonly square: Square;
  readonly refused: boolean;
  readonly label?: string;
  /** A second label under the square ("Cai fora do alcance"): the cost then goes above it. */
  readonly note?: string;
}

/** A reactor whose reach a pending opportunity offer is about, and the square
 * where the mover left it: drawn for whoever answers (E9-13). */
export interface OfferMark {
  readonly reactor: Square;
  readonly left: Square | null;
}

/** Where a creature the mover drags ends up (W7-X): the square the server worked out, and the creature's initial. */
export interface DraggedMark {
  readonly square: Square;
  readonly initial: string;
}

export type { TokenDrop } from './token-drag';

/**
 * The battle map (RN-21; E6-02, E6-04, E6-05, E6-10, E6-11): the map's image,
 * the grid on top of it and a token on each combatant's square. Presentational:
 * it never calls the API; the page saves what it reports.
 *
 * - **Who is drawn:** the combatants that have a square. A player never gets a
 *   hidden one from the server; the master's hidden ones are drawn dashed.
 * - **Fog:** a player's map with the fog on is `app-fog-base` (the tiles, the layers and the
 *   shading of what they see) instead of the image; the combatants are what the server sends them.
 * - **Layers:** the walls, the difficult terrain and the cover, drawn
 *   by `app-map-layers` over the image, and the doors, one mark per kind; with `doorPicks` the master taps a door.
 * - **Reach (E6-10, docs/design/README.md):** the squares the server says a combatant
 *   can go to, tinted, inside a dashed circle of the movement left. A square the
 *   circle holds that is not tinted gets no mark of its own.
 * - **Offers (E9-13):** a dashed outline on the square of the reactor of a pending
 *   opportunity attack and a label on the square the mover left.
 * - **Moving:** the master drags any token (`masterMoves`); a player drags
 *   their own inside the reach (`ownMoveId`). With `pickSquares`, a click on
 *   a square (or an arrow key) chooses it (`choose`) and Enter confirms
 *   (`confirm`), the phone's "Mover" page. The map is one focus stop; the
 *   arrows move the choice one square.
 * - **Size:** it fills its container; with `cellPx` each square has that side
 *   and the container scrolls (the "Mover" page zooms in).
 */
@Component({
  selector: 'app-combat-map',
  imports: [CombatantToken, DoorPicks, FogBase, MapLayersOverlay, MatIconModule, SaddleIcon],
  templateUrl: './combat-map.html',
  styleUrl: './combat-map.scss',
})
export class CombatMap {
  readonly image = input.required<CombatMapImage>();
  readonly mapName = input('');
  readonly columns = input.required<number>();
  readonly rows = input.required<number>();
  readonly combatants = input<readonly Combatant[]>([]);
  readonly isMaster = input(false);
  readonly currentId = input('');
  readonly reach = input<Reach | null>(null);
  readonly layers = input<MapLayers | null>(null);
  /** A player's map with the fog of war on (MR-036, E9-03): what they see, drawn with the tiles the server made for them in place of the image. */
  readonly fog = input<Vision | null>(null);
  readonly offers = input<readonly OfferMark[]>([]);
  readonly chosen = input<Chosen | null>(null);
  /** Where the creature the mover drags ends up if the mover goes to the chosen square: a dashed token (W7-X). */
  readonly dragged = input<DraggedMark | null>(null);
  /** One square marked without a token (E6-02: "O quadrado marcado tem
   * 1,5 m"), to judge the grid's size by. */
  readonly mark = input<Square | null>(null);
  readonly masterMoves = input(false);
  /** The master's "Movimento forçado" is on: while a token is dragged, its square of origin is marked and a dashed line runs from it. */
  readonly forcedDrag = input(false);
  readonly ownMoveId = input<string | null>(null);
  readonly pickSquares = input(false);
  /** The player chooses a target on the map (C7-04): the creatures offered are ringed buttons, the ones that cannot be chosen dimmed. */
  readonly pick = input<readonly PickMark[] | null>(null);
  /** Why the creature the player tapped cannot be the target, as a chip over the map. */
  readonly pickNote = input<PickNote | null>(null);
  /** Side of one square in pixels; `null` fits the map to its container. */
  readonly cellPx = input<number | null>(null);
  /** The master's map: each door is a button that opens its sheet (E10-05 10), over the layers and under the tokens. */
  readonly doorPicks = input(false);
  /** The "Vez" word above the one on turn: left out where the page is about one mover (the "Mover" page). */
  readonly showTurn = input(true);

  /** A creature offered as a target was tapped (`pick`). */
  readonly pickTap = output<string>();
  /** A square was chosen (a click, or an arrow key) in `pickSquares` mode. */
  readonly choose = output<Square>();
  /** Enter on the chosen square in `pickSquares` mode. */
  readonly confirm = output<void>();
  /** The master tapped a door (`doorPicks`). */
  readonly doorPick = output<DoorSquare>();
  /** A token was dropped on a square (a drag, or Enter after the arrows). */
  readonly tokenDrop = output<TokenDrop>();
  /** The places of the fog map whose tile has arrived. */
  readonly fogSettled = output<ReadonlySet<string>>();
  /** Whether the first load of the fog map is still going (a tile that arrives later is not a load). */
  readonly fogLoadingChange = output<boolean>();

  private readonly surface = viewChild.required<ElementRef<HTMLElement>>('surface');

  /** The token under the pointer while it is dragged, and where. */
  private readonly dragger = new TokenDrag({
    surface: () => this.surface().nativeElement,
    shown: () => this.shown(),
    movable: (c) => this.movable(c),
    grab: (id) => this.picked.set(id),
    squareUnder: (e) => this.squareUnder(e),
    drop: (d) => this.tokenDrop.emit(d),
  });
  protected readonly drag = this.dragger.drag;
  /** The token the keyboard moves (the master picks one with a click). */
  private readonly picked = signal<string | null>(null);
  /** The square the arrow keys moved to, outside `pickSquares` mode. */
  protected readonly cursor = signal<Square | null>(null);

  protected readonly shown = computed(() =>
    this.combatants().filter((c) => c.placed && (this.isMaster() || !c.hidden)),
  );
  protected readonly pair = new MapPairs(this.shown, this.currentId);
  /** The squares each token covers, for the "Vez" word: it goes where it covers none (a Large creature is drawn on 2 × 2). */
  private readonly occupied = computed(() => {
    const cells = new Set<string>();
    for (const c of this.shown()) {
      const at = this.anchor(c);
      const n = this.span(c);
      for (let dc = 0; dc < n; dc++) {
        for (let dr = 0; dr < n; dr++) {
          cells.add(`${at.col + dc},${at.row + dr}`);
        }
      }
    }
    return cells;
  });
  private readonly surfaceEl = viewChild<ElementRef<HTMLElement>>('surface');
  protected readonly layer = new PickLayer(
    () => this.pick(),
    () => this.pickNote(),
    () => this.shown(),
    () => ({ columns: this.columns(), rows: this.rows() }),
    watchUnit(
      () => this.surfaceEl()?.nativeElement,
      () => this.columns(),
    ),
    (c) => this.span(c),
  );
  protected readonly aspect = computed(() => `${this.image().width} / ${this.image().height}`);
  protected readonly widthPx = computed(() => {
    const cell = this.cellPx();
    return cell ? cell * this.columns() : null;
  });
  protected readonly gridPath = computed(() => gridPath(this.columns(), this.rows()));
  /** The squares the reach tints (the server's list). */
  protected readonly reachCells = computed<readonly Square[]>(() => this.reach()?.squares ?? []);
  /** The circle of the movement left, in squares (a square straight is 50 tenths of a foot). */
  protected readonly ringRadius = computed(() => {
    const reach = this.reach();
    return reach ? reach.leftDft / DFT_PER_SQUARE : 0;
  });
  /** The square of each reactor, outlined: the server does not say how far its reach goes, so none is drawn. */
  protected readonly reactorBoxes = computed(() =>
    this.offers().map((o) => ({ col: o.reactor.col, row: o.reactor.row, left: o.left })),
  );

  /** The frame on the chosen square (a tap, or the arrow keys' cursor). */
  protected readonly frame = computed<Chosen | null>(() => {
    const chosen = this.chosen();
    if (chosen) {
      return chosen;
    }
    const cursor = this.cursor();
    return cursor ? { square: cursor, refused: false } : null;
  });
  protected readonly framedToken = computed(() =>
    this.tokenOf(
      this.pickSquares() ? (this.ownMoveId() ?? '') : (this.picked() ?? this.ownMoveId() ?? ''),
    ),
  );

  /** The dashed line from the token to the chosen square (E6-10). */
  protected readonly path = computed(() => {
    const frame = this.frame();
    const token = this.pickSquares() ? this.originToken() : this.framedToken();
    if (!frame || !token) {
      return null;
    }
    const from = squareCenter(token, this.columns(), this.rows());
    const to = squareCenter(frame.square, this.columns(), this.rows());
    return { x1: from.x, y1: from.y, x2: to.x, y2: to.y };
  });

  /** A forced drag in progress: the square the token left, and the line from it to where it is now. */
  protected readonly forcedTrace = computed(() => {
    const drag = this.drag();
    const token =
      drag && this.forcedDrag()
        ? this.combatants().find((c) => c.id === drag.id && c.placed)
        : undefined;
    if (!drag || !token || (drag.col === token.col && drag.row === token.row)) {
      return null;
    }
    const from = squareCenter({ col: token.col, row: token.row }, this.columns(), this.rows());
    const to = squareCenter(drag, this.columns(), this.rows());
    return {
      origin: { col: token.col, row: token.row },
      x1: from.x,
      y1: from.y,
      x2: to.x,
      y2: to.y,
    };
  });

  protected readonly listed = this.pair.listed;

  protected readonly canMove = computed(
    () => this.masterMoves() || this.ownMoveId() !== null || this.pickSquares(),
  );

  /** Whether the cost pill of the chosen square goes above it: below by default, but never over a door (it would hide the mark), and above at the bottom edge. */
  protected costAbove(at: Square): boolean {
    return costAbove(at, this.rows(), this.layers()?.doors ?? []);
  }

  protected initial(c: Combatant): string {
    return combatantInitial(c.label);
  }

  /** An NPC is the rounded square; a player's creature is round with a dashed outline (docs/design/README.md). */
  protected npc(c: Combatant): boolean {
    return !isPlayer(c) && !isCreature(c);
  }

  protected creature(c: Combatant): boolean {
    return isCreature(c);
  }

  /** A tap on a creature's button while choosing a target: the square under the pointer decides (see `tappedCreature`). */
  protected tapPick(event: MouseEvent, id: string): void {
    const boxes = this.shown().map((c) => ({ id: c.id, ...this.anchor(c), span: this.span(c) }));
    this.layer.tapped(event, id, boxes, (hit) => this.pickTap.emit(hit));
  }

  /** How many squares a side of the token is drawn on: Large is 2, Huge 3, Gargantuan 4 (the picture only: the server still counts one square). */
  protected span(c: Combatant): number {
    const n = Math.max(
      1,
      Math.min(c.size >= CreatureSize.LARGE ? c.size - 2 : 1, this.columns(), this.rows()),
    );
    if (n === 1) {
      return 1;
    }
    // Two big creatures side by side (two wolves) would be drawn over each other: where another token sits on the squares
    // it would take, it is drawn on its own square.
    const at = this.place(c);
    const col = Math.max(0, Math.min(at.col, this.columns() - n));
    const row = Math.max(0, Math.min(at.row, this.rows() - n));
    const others = this.shown().filter((o) => o.id !== c.id);
    const blocked = others.some((o) => {
      const p = this.place(o);
      return p.col >= col && p.col < col + n && p.row >= row && p.row < row + n;
    });
    return blocked ? 1 : n;
  }

  /** The top-left square of the drawing, pulled back so a big token stays on the map. */
  private anchor(c: Combatant): Square {
    const at = this.place(c);
    const n = this.span(c);
    return {
      col: Math.max(0, Math.min(at.col, this.columns() - n)),
      row: Math.max(0, Math.min(at.row, this.rows() - n)),
    };
  }

  /** The centre of the drawing, in percent of the map. */
  protected centerX(c: Combatant): number {
    return ((this.anchor(c).col + this.span(c) / 2) / this.columns()) * 100;
  }

  protected centerY(c: Combatant): number {
    return ((this.anchor(c).row + this.span(c) / 2) / this.rows()) * 100;
  }

  /**
   * Where the "Vez" word of the token on turn goes, so it is read as its own token's: the pill is wider than a square, so a side
   * counts as free only when the squares along it, one more at each end, hold no other token (a token diagonally by is "beside"
   * it). Above first, then below, then right, then left; when every side is taken, the side with the fewest neighbours (above on a
   * tie), and the garnet ring carries the turn.
   */
  protected turnSide(c: Combatant): 'above' | 'below' | 'right' | 'left' {
    return pillSide(this.anchor(c), this.span(c), this.occupied(), this.columns(), this.rows());
  }

  protected place(c: Combatant): Square {
    const drag = this.drag();
    return drag && drag.id === c.id ? drag : { col: c.col, row: c.row };
  }

  protected left(s: Square): number {
    return squareCenter(s, this.columns(), this.rows()).x;
  }

  protected top(s: Square): number {
    return squareCenter(s, this.columns(), this.rows()).y;
  }

  protected movable(c: Combatant): boolean {
    return (
      this.masterMoves() || c.id === this.ownMoveId() || this.pair.carries(c, this.ownMoveId())
    );
  }

  private tokenOf(id: string): Square | null {
    const c = this.shown().find((x) => x.id === id);
    return c ? { col: c.col, row: c.row } : null;
  }

  /** The square the reach starts from (the mover's own token). */
  private originToken(): Square | null {
    return this.reach()?.origin ?? this.tokenOf(this.ownMoveId() ?? '');
  }

  // ---- pointer ----

  private squareUnder(event: PointerEvent | MouseEvent): Square {
    const rect = this.surface().nativeElement.getBoundingClientRect();
    return squareAt(
      (event.clientX - rect.left) / rect.width,
      (event.clientY - rect.top) / rect.height,
      this.columns(),
      this.rows(),
    );
  }

  protected onPointerDown(event: PointerEvent): void {
    this.dragger.down(event);
  }

  protected onPointerMove(event: PointerEvent): void {
    this.dragger.move(event);
  }

  protected onPointerUp(event: PointerEvent): void {
    this.dragger.up(event);
  }

  protected onClick(event: MouseEvent): void {
    if (!this.pickSquares() || (event.target as HTMLElement).closest('[data-id]')) {
      return;
    }
    this.choose.emit(this.squareUnder(event));
  }

  // ---- keyboard ----

  protected onKeydown(event: KeyboardEvent): void {
    // A door's own button (the master's sheet) keeps Enter and the arrows for itself.
    if ((event.target as HTMLElement).closest('[data-door]') || !this.canMove()) {
      return;
    }
    if (event.key === 'Enter') {
      event.preventDefault();
      this.onEnter();
    } else if (event.key === 'Escape') {
      this.cursor.set(null);
    } else if (event.key.startsWith('Arrow')) {
      event.preventDefault();
      this.onArrow(event.key);
    }
  }

  /** Enter: confirms the chosen square (`pickSquares`), or drops the token the keyboard moves where the cursor is. */
  private onEnter(): void {
    if (this.pickSquares()) {
      this.confirm.emit();
      return;
    }
    const cursor = this.cursor();
    const id = this.picked() ?? this.ownMoveId();
    if (cursor && id) {
      this.tokenDrop.emit({ id, ...cursor });
      this.cursor.set(null);
    }
  }

  /** An arrow: the choice (`pickSquares`) or the cursor goes one square, from where it is, else from the token the keyboard moves. */
  private onArrow(key: string): void {
    const base = this.pickSquares()
      ? (this.chosen()?.square ?? this.reach()?.origin ?? null)
      : (this.cursor() ?? this.tokenOf(this.picked() ?? this.ownMoveId() ?? this.currentId()));
    if (!base) {
      return;
    }
    const next = stepSquare(base, key, this.columns(), this.rows());
    if (this.pickSquares()) {
      this.choose.emit(next);
    } else {
      this.cursor.set(next);
    }
  }

  protected onBlur(): void {
    this.cursor.set(null);
  }
}

/** The side of a token (its top-left square and its side in squares) where the "Vez" word goes; see `CombatMap.turnSide`. */
export function pillSide(
  at: Square,
  n: number,
  taken: ReadonlySet<string>,
  columns: number,
  rows: number,
): 'above' | 'below' | 'right' | 'left' {
  const sides: { side: 'above' | 'below' | 'right' | 'left'; cells: [number, number][] }[] = [
    {
      side: 'above',
      cells: Array.from(
        { length: n + 2 },
        (_, i) => [at.col - 1 + i, at.row - 1] as [number, number],
      ),
    },
    {
      side: 'below',
      cells: Array.from(
        { length: n + 2 },
        (_, i) => [at.col - 1 + i, at.row + n] as [number, number],
      ),
    },
    {
      side: 'right',
      cells: Array.from(
        { length: n + 2 },
        (_, i) => [at.col + n, at.row - 1 + i] as [number, number],
      ),
    },
    {
      side: 'left',
      cells: Array.from(
        { length: n + 2 },
        (_, i) => [at.col - 1, at.row - 1 + i] as [number, number],
      ),
    },
  ];
  const own = (col: number, row: number) =>
    col >= at.col && col < at.col + n && row >= at.row && row < at.row + n;
  const score = (cells: [number, number][]) => {
    // The cell in the middle of the side (right against the token) is on the map or the pill has no room; the ends only count when a token is there.
    const middle = cells.slice(1, -1);
    if (middle.some(([col, row]) => col < 0 || row < 0 || col >= columns || row >= rows)) {
      return 99;
    }
    return cells.filter(([col, row]) => !own(col, row) && taken.has(`${col},${row}`)).length;
  };
  const scored = sides.map((s) => ({ side: s.side, score: score(s.cells) }));
  return scored.reduce((best, s) => (s.score < best.score ? s : best)).side;
}
