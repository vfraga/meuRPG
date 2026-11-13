import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
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
import { NgTemplateOutlet } from '@angular/common';
import { MatIconModule } from '@angular/material/icon';

import {
  AreaPlacement,
  type Combatant,
  type PreviewSpellAreaResponse,
  type TargetInReach,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { type AreaPick, samePick } from '../../core/combat/area-flow';
import {
  announcement,
  beforeHint,
  centerLabel,
  mapLabelOf,
  outOfRangeText,
  placedHint,
} from '../../core/combat/area-text';
import type { Square } from '../../core/combat/combat-grid';
import { BURNING_PATTERN, zonePattern } from '../../core/combat/zone-pattern';
import {
  type AreaShape,
  type Direction,
  beyondRange,
  directionWord,
  cellsPath,
  outlinePath,
  rangeFt,
  toward,
} from '../../core/combat/spell-area';
import type { MapLayers } from '../../core/maps/layers';
import type { Vision } from '../../core/maps/vision';
import { type WallChoice, wallHint, wallZoneView } from '../../core/combat/spell-wall';
import { metersText } from '../../core/units';
import { CombatMap, type CombatMapImage } from '../combat-map/combat-map';
import { ZoneLayer } from '../combat-map/zone-layer/zone-layer';
import { ZoneSwatch } from '../combat-map/zone-layer/zone-swatch';
import { CombatantToken } from '../combatant-token/combatant-token';
import { MapLayersLegend } from '../map-layers/map-layers-legend';
import { AreaAnnouncer } from './area-announcer';
import {
  BIG_MAP_COLUMNS,
  ZOOM_CELLS,
  areaLine,
  clampSquare,
  outlineOf,
  edgeMarkOf,
  squareUnder,
  arrowPick,
  beyondReach,
} from './area-moves';
import { AreaPointer } from './area-pointer';
import { CenterList } from './center-list';
import { type CenterOption, centerOptions } from './center-options';
import { WallDrag } from './wall-drag';

let nextId = 0;

/**
 * Step 1 of an area spell on the map (PM-02a states 2 and 2b, PM-02b states 5 to 7, PM-02d state 11): the battle map with
 * the spell's template over it.
 *
 * - **Placing:** the template follows the pointer, and a finger 44 px above it; releasing (or a click) places the point:
 *   the template fills, the diamond marks the origin and a dashed line measures it from the caster. A second tap on the same
 *   point confirms. A cone, a line and a cube take the nearest of eight directions from the caster instead.
 * - **The server's word:** once a preview answers for the placed point (`preview`), its squares replace the outline and its
 *   point is the effective one; when a wall moved it, the tap is drawn as an ✕ and the origin where the server put it.
 * - **Range:** the squares beyond a point spell's range are dimmed. A player's tap there places nothing and says why; the
 *   master is not held to it, and the dimming is only a hint for him.
 * - **Keyboard:** the map is a focusable `role="application"`. Arrows move the point a square (Shift: five) or turn the
 *   direction 45°; Enter places, and on a placed point confirms; Escape cancels; C opens "Centrar em…" ("Apontar para…"), the
 *   listbox of the creatures the caster sees, the caster and "Um quadrado do mapa (use as setas)". Each move is announced
 *   politely after a 300 ms pause; a count of creatures only after a preview, never a name.
 *
 * Presentational: it reports places and confirmations; the page asks the server.
 */
@Component({
  selector: 'app-area-map',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    CombatMap,
    CombatantToken,
    MapLayersLegend,
    MatIconModule,
    NgTemplateOutlet,
    ZoneLayer,
    ZoneSwatch,
  ],
  templateUrl: './area-map.html',
  styleUrl: './area-map.scss',
})
export class AreaMap {
  private readonly injector = inject(Injector);

  readonly image = input.required<CombatMapImage>();
  readonly mapName = input('');
  readonly columns = input.required<number>();
  readonly rows = input.required<number>();
  readonly combatants = input<readonly Combatant[]>([]);
  readonly layers = input<MapLayers | null>(null);
  readonly fog = input<Vision | null>(null);
  /** The master: hidden creatures are drawn, and the range does not hold him. */
  readonly master = input(false);
  readonly casterId = input.required<string>();
  /** The spell's Portuguese name ("Bola de Fogo"). */
  readonly spell = input.required<string>();
  readonly placement = input.required<AreaPlacement>();
  readonly area = input.required<AreaShape>();
  /** How far a point may be from the caster, in feet; 0 for a shape that comes out of the caster. */
  readonly rangeFt = input(0);
  readonly placed = input<AreaPick | null>(null);
  readonly preview = input<PreviewSpellAreaResponse | null>(null);
  /** Step 2: the map only shows the area, and takes no tap. */
  readonly readOnly = input(false);
  /** The creatures the caster sees, with their distances (`SpellTargets.targets`): the list of "Centrar em…". */
  readonly seen = input<readonly TargetInReach[]>([]);
  /** A spell that lays a wall (Wall of Fire): the point is the start of a line or the center of a ring, and the server's wall and
   * burning side are drawn instead of the area's outline. A drag on a line also says its direction (`aimed`). */
  readonly wall = input<WallChoice | null>(null);
  /** The focus goes to the map when the step opens. */
  readonly autofocus = input(false);

  /** A point or a direction placed; `null` when a tap outside the range took the old one away. */
  readonly place = output<AreaPick | null>();
  /** A drag that lays a line: its first square and the way (one of eight) it runs. */
  readonly aimed = output<{ square: Square; direction: Direction }>();
  /** A second tap on the same place, or Enter on it. */
  readonly confirm = output<void>();
  /** Escape on the map: back to the step before. */
  readonly dismiss = output<void>();

  protected readonly id = `area-map-${nextId++}`;
  private readonly app = viewChild<ElementRef<HTMLElement>>('app');
  private readonly listbox = viewChild<ElementRef<HTMLElement>>('listbox');
  private readonly centerButton = viewChild<ElementRef<HTMLButtonElement>>('center');

  /** The template under the pointer, the finger or the arrow keys, not placed yet. */
  protected readonly hover = signal<AreaPick | null>(null);
  /** The zoom step of a big map (0 fits it to the screen). */
  protected readonly zoom = signal(0);
  protected readonly lastZoom = ZOOM_CELLS.length - 1;
  protected readonly zoomable = computed(
    () => !this.readOnly() && this.columns() > BIG_MAP_COLUMNS,
  );
  protected readonly cell = computed(() => (this.zoomable() ? ZOOM_CELLS[this.zoom()] : null));
  /** The hover came from the keyboard, so the hint follows it (a mouse that only passes over does not rewrite it). */
  private readonly hoverByKey = signal(false);
  private readonly pointer = new AreaPointer({
    readOnly: () => this.readOnly(),
    pickUnder: (event, lift) => this.pickAt(squareUnder(event, lift, this.columns(), this.rows())),
    hover: this.hover,
    setHover: (pick, byKey) => this.setHover(pick, byKey),
    tap: (pick) => this.tap(pick),
  });
  private readonly wallDrag = new WallDrag({
    readOnly: () => this.readOnly(),
    squareAt: (event) => squareUnder(event, 0, this.columns(), this.rows()),
    hover: (pick) => this.setHover(pick, false),
    tap: (pick) => this.tap(pick),
    aimed: (square, direction) => this.aim(square, direction),
  });
  /** A straight wall is laid by a drag; every other pick keeps the pointer of the area map. */
  protected readonly gesture = computed(() =>
    this.wall() && !this.wall()?.ring ? this.wallDrag : this.pointer,
  );
  /** Where the finger is, in percent of the map, while it drags: the dotted line to the template above it. */
  protected readonly finger = this.pointer.finger;
  /** "Fora do alcance." and why, after a tap outside the range. */
  protected readonly refusal = signal('');
  protected readonly list = new CenterList({
    readOnly: () => this.readOnly(),
    options: () => this.options(),
    choose: (option) => {
      if (option.square) {
        this.tap(this.pickAt(option.square));
      }
    },
    focusOf: (where) =>
      (where === 'list' ? this.listbox() : where === 'button' ? this.centerButton() : this.app())
        ?.nativeElement,
    injector: this.injector,
  });
  private readonly announcer = new AreaAnnouncer();
  /** What the live region says, a pause after the last move. */
  protected readonly spoken = this.announcer.spoken;

  protected readonly Placement = AreaPlacement;
  protected readonly isPoint = computed(() => this.placement() === AreaPlacement.POINT);
  protected readonly isDirection = computed(() => this.placement() === AreaPlacement.DIRECTION);

  protected readonly caster = computed<Square | null>(() => {
    const c = this.combatants().find((x) => x.id === this.casterId() && x.placed);
    return c ? { col: c.col, row: c.row } : null;
  });

  /** What the template shows: the hover, or what is placed. */
  protected readonly shown = computed(
    () => (this.readOnly() ? null : this.hover()) ?? this.placed(),
  );
  /** The template is the placed one (filled), not one that moves (dashed). */
  protected readonly filled = computed(() => {
    const placed = this.placed();
    return !!placed && samePick(this.shown(), placed);
  });

  /** The squares drawn: the server's, for the placed point it answered for; otherwise the browser's outline. */
  protected readonly squares = computed<readonly Square[]>(() => {
    const p = this.preview();
    if (this.wall()) {
      return [];
    }
    return p && this.filled()
      ? p.squares.map((q) => ({ col: q.col, row: q.row }))
      : outlineOf(this.shown(), this.caster(), this.area(), this.columns(), this.rows());
  });
  /** The wall the server laid for the placed start, as the zone the map draws (bricks and the burning side's arrows). */
  protected readonly wallZones = computed(() => {
    const wall = this.wall();
    // The last answer stays drawn while the next one is on its way, so a change does not flicker.
    const p = this.preview() ?? this.lastWall();
    return wall && p && this.placed() ? [wallZoneView(p, wall.side)] : [];
  });
  private readonly lastWall = signal<PreviewSpellAreaResponse | null>(null);
  protected readonly wallPattern = zonePattern('spell:wall-of-fire');
  protected readonly burningPattern = BURNING_PATTERN;
  protected readonly fillPath = computed(() => cellsPath(this.squares()));
  protected readonly edgePath = computed(() => outlinePath(this.squares()));

  /** The effective point of origin: the server's when it answered, else the point placed (or hovered). */
  protected readonly origin = computed<Square | null>(() => {
    // A wall's start stays on the placed square: a hover does not move it.
    const pick = this.wall() ? (this.placed() ?? this.shown()) : this.shown();
    if (pick?.kind !== 'point') {
      return null;
    }
    const o = this.preview()?.origin;
    return (this.wall() ? samePick(pick, this.placed()) : this.filled()) && o
      ? { col: o.col, row: o.row }
      : pick.square;
  });
  /** The point tapped when a wall moved it: drawn as an ✕ beside the real origin. */
  protected readonly tapped = computed<Square | null>(() => {
    const pick = this.placed();
    return this.filled() && this.preview()?.moved && pick?.kind === 'point' ? pick.square : null;
  });
  /** The diamond of a shape out of the caster: on the caster's side the area leaves from. */
  protected readonly edgeMark = computed(() =>
    edgeMarkOf(this.shown(), this.caster(), this.columns(), this.rows()),
  );
  /** The dashed line from the caster to the point, with its length ("7,5 m"). */
  protected readonly line = computed(() =>
    areaLine(this.caster(), this.origin(), this.columns(), this.rows()),
  );
  /** The squares beyond the range, dimmed (a point spell with a range). */
  protected readonly dimPath = computed(() => {
    const caster = this.caster();
    const range = this.rangeFt();
    return caster && this.isPoint() && range > 0
      ? cellsPath(beyondRange(caster, range, this.columns(), this.rows()))
      : '';
  });
  protected readonly rangeText = computed(() => metersText(this.rangeFt()));
  protected readonly hasHidden = computed(
    () => this.master() && this.combatants().some((c) => c.placed && c.hidden),
  );

  protected readonly hint = computed(() => {
    const pick = (this.hoverByKey() ? this.hover() : null) ?? this.placed();
    const wall = this.wall();
    if (wall) {
      return wallHint(wall, !!pick);
    }
    if (!pick) {
      return beforeHint(this.placement(), this.area());
    }
    return placedHint(
      this.placement(),
      this.distanceOf(pick),
      pick.kind === 'direction' ? pick.direction : null,
    );
  });
  protected readonly mapLabel = computed(() =>
    mapLabelOf(this.spell(), this.placement(), this.wall()),
  );
  protected readonly centerText = computed(() => centerLabel(this.placement()));

  /** "Centrar em…": the creatures the caster sees, nearest list order as the server gave it, then the caster and the arrows. */
  protected readonly options = computed<readonly CenterOption[]>(() =>
    centerOptions(this.combatants(), this.seen(), this.casterId(), this.isPoint()),
  );

  constructor() {
    afterNextRender(() => {
      if (this.autofocus()) {
        this.app()?.nativeElement.focus({ preventScroll: true });
      }
    });
    inject(DestroyRef).onDestroy(() => this.announcer.clear());
    // The count of creatures is said once the server answered for the place on the map.
    effect(() => {
      const p = this.preview();
      if (p && this.wall()) {
        this.lastWall.set(p);
      }
      if (p && untracked(() => this.filled())) {
        untracked(() => this.say(this.placed()));
      }
    });
  }

  // ---- what each place means ----

  private distanceOf(pick: AreaPick): number {
    const caster = this.caster();
    return pick.kind === 'point' && caster ? rangeFt(caster, pick.square) : 0;
  }

  /** The pick a square makes: the square itself for a point, the direction toward it for a shape out of the caster. */
  private pickAt(square: Square): AreaPick | null {
    if (this.isPoint()) {
      return { kind: 'point', square };
    }
    const caster = this.caster();
    const direction = caster ? toward(caster, square) : null;
    return direction ? { kind: 'direction', direction } : null;
  }

  /** A tap (or Enter, or a pick in the list) on a place: places it, confirms it when it is the placed one, or refuses it. */
  private tap(pick: AreaPick | null): void {
    if (!pick || this.readOnly()) {
      return;
    }
    if (this.outOfRange(pick)) {
      this.place.emit(null);
      return;
    }
    this.refusal.set('');
    this.hover.set(null);
    if (samePick(pick, this.placed())) {
      this.confirm.emit();
      return;
    }
    this.place.emit(pick);
    this.say(pick);
  }

  /** A point beyond the range of a player's spell: says why and drops the old point. The master is not held to it. */
  private outOfRange(pick: AreaPick): boolean {
    const caster = this.caster();
    if (!caster || this.master() || !beyondReach(pick, caster, this.rangeFt())) {
      return false;
    }
    const text = outOfRangeText(
      this.spell(),
      this.rangeFt(),
      rangeFt(caster, (pick as { square: Square }).square),
    );
    this.refusal.set(text);
    this.hover.set(null);
    this.announcer.now(`Fora do alcance. ${text}`);
    return true;
  }

  /** The end of a drag on a line: the start and the way, unless the start is out of reach. */
  private aim(square: Square, direction: Direction): void {
    const pick: AreaPick = { kind: 'point', square };
    if (this.outOfRange(pick)) {
      this.place.emit(null);
      return;
    }
    this.refusal.set('');
    this.hover.set(null);
    this.aimed.emit({ square, direction });
    const caster = this.caster();
    this.announcer.say(
      `${announcement(this.placement(), caster ? rangeFt(caster, square) : 0, null, null)}, direção ${directionWord(direction)}`,
    );
  }

  // ---- pointer ----

  protected onPointerLeave(event: PointerEvent): void {
    this.pointer.leave(event);
  }

  protected onPointerCancel(): void {
    this.pointer.cancel();
    this.wallDrag.cancel();
  }

  // ---- keyboard ----

  protected onKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      this.hover.set(null);
      this.dismiss.emit();
      return;
    }
    if (this.readOnly()) {
      return;
    }
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      this.tap(this.hover() ?? this.placed());
      return;
    }
    if (event.key === 'c' || event.key === 'C') {
      event.preventDefault();
      this.list.show();
      return;
    }
    const next = arrowPick(
      event,
      this.hover() ?? this.placed(),
      this.caster(),
      this.isPoint(),
      this.columns(),
      this.rows(),
    );
    if (next) {
      event.preventDefault();
      this.setHover(next, true);
      this.say(next);
    }
  }

  protected zoomBy(delta: 1 | -1): void {
    this.zoom.update((z) => Math.min(this.lastZoom, Math.max(0, z + delta)));
  }

  private setHover(pick: AreaPick | null, byKey: boolean): void {
    this.hoverByKey.set(byKey);
    this.hover.set(pick);
  }

  /** "Ajustar o ponto, um quadrado por toque": moves the placed point one square, as a tap there. */
  protected nudge(dc: number, dr: number): void {
    const pick = this.placed();
    if (pick?.kind !== 'point') {
      return;
    }
    const square = clampSquare(
      { col: pick.square.col + dc, row: pick.square.row + dr },
      this.columns(),
      this.rows(),
    );
    this.tap({ kind: 'point', square });
  }

  // ---- the live region ----

  private say(pick: AreaPick | null): void {
    if (!pick) {
      return;
    }
    const p = this.preview();
    const seen = p && samePick(pick, this.placed()) ? p.targets.length : null;
    this.announcer.say(
      announcement(
        this.placement(),
        this.distanceOf(pick),
        pick.kind === 'direction' ? pick.direction : null,
        seen,
      ),
    );
  }

  // ---- drawing ----

  protected pct(n: number, of: number): number {
    return (n / of) * 100;
  }
}
