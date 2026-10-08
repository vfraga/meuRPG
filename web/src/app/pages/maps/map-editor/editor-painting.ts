import {
  DestroyRef,
  type Signal,
  computed,
  effect,
  inject,
  signal,
  untracked,
} from '@angular/core';

import { MapLayer, type Map as MapMessage } from '../../../../gen/meurpg/maps/v1/maps_pb';
import type { Square } from '../../../core/combat/combat-grid';
import { type DoorKind, type MapLayers, hasPainted } from '../../../core/maps/layers';
import { editorErrorMessage } from '../../../core/maps/map-errors';
import { MapsClient } from '../../../core/maps/maps-client';
import {
  type DoorPlan,
  NO_GAP_TEXT,
  hidesWhoStands,
  planDoorBlock,
} from '../../../core/maps/door-paint';
import { PaintSession } from '../../../core/maps/paint-session';
import {
  DEFAULT_SETTINGS,
  DOOR_LABEL,
  type PaintSettings,
  brushSquares,
  strokeOf,
} from '../../../core/maps/paint-tools';
import { type LayerVisibility } from '../layers-panel/layers-panel';
import type { EditorMode } from '../editor-bar/editor-bar';
import type { Stroke } from '../paint-surface/paint-surface';

/** A creature standing on a square of the map (a token), for the question before a door goes where it stands. */
export interface Occupant extends Square {
  readonly name: string;
}

/** A door stroke waiting for the master's answer: the squares and what each would do, and who stands on them. */
interface DoorAsk {
  readonly plans: readonly {
    readonly square: Square;
    readonly squares: readonly Square[];
    readonly plan: DoorPlan;
  }[];
  readonly names: readonly string[];
  /** "Tirar a porta": the wall comes back over them, not a door. */
  readonly erase: boolean;
}

const ALL_VISIBLE: LayerVisibility = {
  terrain: true,
  wall: true,
  cover: true,
  light: true,
  door: true,
};

/**
 * Everything the map editor does when the master paints (MR-034), apart from the screen: the tool and the brush, which layers are on,
 * the painted layers and the queue that saves the strokes (`PaintSession`), whether painting is on and why not, the cursor, and the
 * question "Há traços sem salvar" when he leaves with strokes the server did not take. `MapEditor` owns an instance and the template
 * reads it; nothing here touches the DOM. Create it where `inject()` works (a field of the component): it reads the maps client and
 * registers the effects that read the layers when a map opens, when a refusal dropped strokes, and the flush when the editor goes away.
 */
export class EditorPainting {
  private readonly api = inject(MapsClient);

  readonly settings = signal<PaintSettings>(DEFAULT_SETTINGS);
  readonly visible = signal<LayerVisibility>(ALL_VISIBLE);
  /** The painted layers, the queue that saves the strokes and whether painting is on. */
  readonly session = new PaintSession(this.api);
  readonly painted = this.session.layers;
  readonly queue = this.session.queue;
  /** The question "Há traços sem salvar" while the master leaves the page with strokes the server did not take. */
  readonly leaving = signal(false);
  /** The square under the brush (the pointer's, or the keyboard's). */
  readonly hover = signal<Square | null>(null);
  /** The "Porta" tool: why a tap did nothing ("uma porta precisa de chão dos dois lados"), and the question before a door goes where
   * someone stands (`doorWarning` holds who). Both are shown in the "Porta" panel. */
  readonly doorRefusal = signal('');
  private readonly doorAsk = signal<DoorAsk | null>(null);
  readonly doorWarning = computed(() => this.doorAsk()?.names ?? null);
  /** The question is about "Tirar a porta" (the wall comes back over them). */
  readonly doorWarningErase = computed(() => this.doorAsk()?.erase ?? false);

  readonly columns = computed(() => this.map()?.gridColumns ?? 0);
  readonly rows = computed(() => this.map()?.gridRows ?? 0);
  /** How many rules' squares a drawing square is wide: a door is painted and erased over the whole block. */
  private readonly factor = computed(() => Math.max(1, this.map()?.squareFactor ?? 1));
  readonly hasGrid = computed(() => this.columns() > 0);
  /** Painting is on with a grid and the painted layers read once: before that a stroke would be drawn over an empty map. */
  readonly canPaint = computed(() => this.hasGrid() && this.session.readiness() === 'ready');
  readonly painting = computed(() => this.mode() === 'paint' && this.canPaint());
  /** Why the paint tools cannot act, written next to them. */
  readonly why = computed(() => {
    if (!this.hasGrid()) {
      return 'Defina a grade para pintar e ligar a névoa.';
    }
    switch (this.session.readiness()) {
      case 'loading':
        return 'Lendo o que já está pintado...';
      case 'failed':
        return 'Não deu para ler o que já está pintado.';
      default:
        return '';
    }
  });
  /** What the server said when it refused a batch, by the typed reason. */
  readonly saveProblem = computed(() => {
    const err = this.queue.failure();
    return err === null ? '' : editorErrorMessage(err, 'paint', 'salvar o que você pintou');
  });
  /** The layers the map shows now: the painted ones the master left on. */
  readonly shown = computed<MapLayers>(() => {
    const l = this.painted.layers();
    const v = this.visible();
    if (v.terrain && v.wall && v.cover && v.light && v.door) {
      return l;
    }
    return {
      ...l,
      terrain: v.terrain ? l.terrain : [],
      walls: v.wall ? l.walls : [],
      half: v.cover ? l.half : [],
      threeQuarters: v.cover ? l.threeQuarters : [],
      light: v.light ? l.light : undefined,
      doors: v.door ? l.doors : undefined,
    };
  });
  /** Something is painted or seen: a new grid or a new image would erase it. */
  readonly erases = computed(
    () => hasPainted(this.painted.layers()) || (this.map()?.fogEnabled ?? false),
  );
  /** Which painted light levels are on the map: the legend names only those. */
  readonly lightLevels = computed(() => {
    const l = this.shown().light;
    return {
      bright: (l?.bright.length ?? 0) > 0,
      dim: (l?.dim.length ?? 0) > 0,
      dark: (l?.dark.length ?? 0) > 0,
    };
  });
  /** The squares the next stroke would cover, for the outline on the map. */
  readonly cursor = computed(() => {
    const at = this.hover();
    if (!at || !this.painting()) {
      return null;
    }
    const s = this.settings();
    if (s.tool === 'door') {
      // One drawing square (its whole block on a calibrated map) at a time, and the cursor says what the tap would do.
      const f = this.factor();
      const col = Math.floor(at.col / f) * f;
      const row = Math.floor(at.row / f) * f;
      return {
        col,
        row,
        w: Math.min(f, this.columns() - col),
        h: Math.min(f, this.rows() - row),
        erase: s.erase,
        door: this.doorCursor(at, s),
      };
    }
    const reach = s.brush === 3 ? 1 : 0;
    const col = Math.max(0, at.col - reach);
    const row = Math.max(0, at.row - reach);
    return {
      col,
      row,
      w: Math.min(this.columns() - 1, at.col + reach) - col + 1,
      h: Math.min(this.rows() - 1, at.row + reach) - row + 1,
      erase: s.erase,
    };
  });

  private leaveResolve: ((leave: boolean) => void) | null = null;

  /**
   * @param campaignId the campaign's ID
   * @param map the map on the screen
   * @param mode the editor's mode
   * @param flagsStale called when a refusal says the page's flags (a combat runs, or not) may be out of date
   */
  constructor(
    private readonly campaignId: Signal<string>,
    private readonly map: Signal<MapMessage | null | undefined>,
    private readonly mode: Signal<EditorMode>,
    private readonly flagsStale: () => void,
    /** The creatures on the map, by square: a door painted over one asks first (RN-26). */
    private readonly occupants: Signal<readonly Occupant[]> = signal([]),
  ) {
    // The painted layers: read when a map opens and when its grid or its layers change under the editor (a new grid or image clears
    // them, and drops the strokes that waited), never while strokes wait (the copy on the screen is the newer).
    effect(() => {
      const map = this.map();
      if (!map) {
        return;
      }
      untracked(
        () =>
          void this.session.open(this.campaignId(), map.id, map.gridColumns, map.layersRevision),
      );
    });
    // A refusal dropped strokes: read what the server has. One that says a combat runs, or that the map changed, makes the flags stale.
    effect(() => {
      this.queue.refused();
      const map = untracked(() => this.map());
      if (map) {
        untracked(() =>
          this.session.syncAfterRefusal(
            this.campaignId(),
            map.id,
            map.gridColumns,
            map.layersRevision,
          ),
        );
      }
      untracked(() => {
        if (this.queue.failure() !== null && !this.queue.retryable()) {
          this.flagsStale();
        }
      });
    });
    // What waits is sent when the editor goes away.
    inject(DestroyRef).onDestroy(() => void this.queue.flush());
  }

  /** Leaving the paint mode: what waits goes out, and the cursor goes away. */
  stop(): void {
    void this.queue.flush();
    this.hover.set(null);
    this.doorAsk.set(null);
    this.doorRefusal.set('');
  }

  /** The brush's stroke: the squares the brush covers on each centre; painted at once, saved in batches, each to the map it was made on. */
  stroke(stroke: Stroke, mapId: string): void {
    const cols = this.columns();
    const rows = this.rows();
    if (cols === 0 || this.session.readiness() !== 'ready') {
      return;
    }
    const target = { campaignId: this.campaignId(), mapId };
    const s = { ...this.settings(), erase: this.settings().erase || stroke.erase };
    if (s.tool === 'door') {
      this.doorStroke(stroke.centers, s.erase ? 0 : s.door, mapId);
      return;
    }
    const { layer, value } = strokeOf(s);
    const squares = stroke.centers.flatMap((c) => brushSquares(c, s.brush, cols, rows));
    const changed = this.painted.paint(layer, value, squares);
    if (changed.length > 0) {
      this.queue.add(target, layer, value, changed);
    }
  }

  /** The cursor of the door tool: the kind it would paint and the words, or "Aqui não dá" (no preview) where a tap would be refused. */
  private doorCursor(at: Square, s: PaintSettings): { state: DoorKind | null; label: string } {
    const read = (layer: MapLayer, col: number, row: number) => this.painted.value(layer, col, row);
    const kind = s.erase ? 0 : s.door;
    if (
      !planDoorBlock(read, this.columns(), this.rows(), this.factor(), at.col, at.row, kind).plan.ok
    ) {
      return { state: null, label: 'Aqui não dá' };
    }
    return { state: s.door, label: this.doorCursorLabel(at, s) };
  }

  /** What the door tool's tap on `at` would say in the cursor's label: "Fechada", or "Fechada → Trancada" on a door, or "Tirar a porta". */
  private doorCursorLabel(at: Square, s: PaintSettings): string {
    const here = this.painted.value(MapLayer.DOORS, at.col, at.row) as DoorKind | 0;
    if (s.erase) {
      return here === 0 ? 'Sem porta' : 'Tirar a porta';
    }
    return here === 0 || here === s.door
      ? DOOR_LABEL[s.door]
      : `${DOOR_LABEL[here]} → ${DOOR_LABEL[s.door]}`;
  }

  /** A stroke of the door tool: each square gets what `planDoor` says; one that cannot take a door says why, and a blocking door where
   * someone stands waits for the master's answer. */
  private doorStroke(centers: readonly Square[], kind: DoorKind | 0, mapId: string): void {
    const read = (layer: MapLayer, col: number, row: number) => this.painted.value(layer, col, row);
    const plans = centers.map((square) => ({
      square,
      ...planDoorBlock(
        read,
        this.columns(),
        this.rows(),
        this.factor(),
        square.col,
        square.row,
        kind,
      ),
    }));
    const refused = plans.some((p) => !p.plan.ok);
    this.doorRefusal.set(refused && plans.every((p) => !p.plan.ok) ? NO_GAP_TEXT : '');
    this.doorAsk.set(null);
    // A closed, locked or secret door hides who stands there; so does the wall that comes back with "Tirar a porta".
    if (hidesWhoStands(kind) || kind === 0) {
      const names = plans
        .filter((p) => p.plan.ok && p.plan.writes.length > 0)
        .flatMap((p) =>
          this.occupants()
            .filter((o) => p.squares.some((q) => q.col === o.col && q.row === o.row))
            .map((o) => o.name),
        );
      if (names.length > 0) {
        this.doorAsk.set({ plans, names: [...new Set(names)], erase: kind === 0 });
        return;
      }
    }
    this.applyDoors(plans, mapId);
  }

  private applyDoors(plans: DoorAsk['plans'], mapId: string): void {
    const target = { campaignId: this.campaignId(), mapId };
    for (const { squares, plan } of plans) {
      if (!plan.ok) {
        continue;
      }
      for (const w of plan.writes) {
        const changed = this.painted.paint(w.layer, w.value, squares);
        if (changed.length > 0) {
          this.queue.add(target, w.layer, w.value, changed);
        }
      }
    }
  }

  /** "Pôr a porta" in the question about someone standing there. */
  confirmDoor(mapId: string): void {
    const ask = this.doorAsk();
    this.doorAsk.set(null);
    if (ask) {
      this.applyDoors(ask.plans, mapId);
      void this.queue.flush();
    }
  }

  cancelDoor(): void {
    this.doorAsk.set(null);
  }

  strokeEnd(): void {
    void this.queue.flush();
  }

  retrySave(): void {
    void this.queue.retry();
  }

  retryRead(): void {
    const map = this.map();
    if (map) {
      void this.session.open(this.campaignId(), map.id, map.gridColumns, map.layersRevision, true);
    }
  }

  /** For the route guard: true when it is fine to leave. What waits is sent first; if the server does not take it, the editor asks. */
  async confirmLeave(): Promise<boolean> {
    if (!this.queue.busy && this.queue.status() !== 'error') {
      return true;
    }
    await this.queue.flush();
    if (this.queue.status() === 'saved') {
      return true;
    }
    this.leaving.set(true);
    return new Promise<boolean>((resolve) => {
      this.leaveResolve = resolve;
    });
  }

  answerLeave(leave: boolean): void {
    this.leaving.set(false);
    const resolve = this.leaveResolve;
    this.leaveResolve = null;
    resolve?.(leave);
  }
}
