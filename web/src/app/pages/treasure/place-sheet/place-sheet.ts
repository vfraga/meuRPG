import { Component, ElementRef, computed, effect, inject, signal } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';

import type { XpMode } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { DungeonRoom } from '../../../../gen/meurpg/maps/v1/dungeons_pb';
import {
  type Map as MapMessage,
  type MapPoint,
  MapPointKind,
  MapPointSchema,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import { type Treasure, TreasureMode } from '../../../../gen/meurpg/maps/v1/treasure_pb';
import { newKey } from '../../../core/connect/idempotency';
import { type Square, squareAt } from '../../../core/combat/combat-grid';
import { DungeonsClient } from '../../../core/maps/dungeons-client';
import { type MapLayers, NO_LAYERS, decodeLayers } from '../../../core/maps/layers';
import { MapsClient } from '../../../core/maps/maps-client';
import { TreasureClient } from '../../../core/treasure/treasure-client';
import {
  NO_GRID_TEXT,
  type PlaceFailure,
  placeFailure,
} from '../../../core/treasure/treasure-errors';
import {
  coinRows,
  goldKinds,
  goldLine,
  groupItems,
  pieceRows,
  po,
} from '../../../core/treasure/treasure-format';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { MapLayersLegend } from '../../../shared/map-layers/map-layers-legend';
import { MapPinsLegend } from '../../../shared/map-pins/map-pins-legend';
import { MapView } from '../../../shared/map-view/map-view';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../shared/sheet/sheet-host';
import { EditorOverlay } from '../../maps/editor-overlay/editor-overlay';

/** What the page hands the sheet: the treasure to place and what the confirmation says about the gold. */
export interface PlaceSheetData {
  readonly campaignId: string;
  readonly treasure: Treasure;
  readonly xpMode: XpMode;
  readonly campaignName: string;
}

/** What the sheet answers: the point was made, or the master asked to generate the treasure again. */
export type PlaceSheetResult =
  | {
      readonly kind: 'placed';
      readonly point: MapPoint;
      readonly mapId: string;
      readonly mapName: string;
      /** "Sala 3" when the square is in a room of the generated dungeon, `''` otherwise. */
      readonly place: string;
      readonly goldPo: number;
      readonly itemCount: number;
    }
  | { readonly kind: 'again' };

type MapsState =
  { status: 'loading' } | { status: 'error' } | { status: 'ready'; maps: readonly MapMessage[] };
type DetailState =
  | { status: 'loading' }
  | { status: 'error' }
  | {
      status: 'ready';
      map: MapMessage;
      points: readonly MapPoint[];
      layers: MapLayers;
      rooms: readonly DungeonRoom[];
    };

const NAME_MAX = 80;
const SHIFT_STEP = 5;

/**
 * "Pôr no mapa" (MR-044, RN-09, RN-10, E10-10 states 4 and 6): the master chooses the map and the square, and `PlaceTreasure` makes a
 * hidden treasure point there with the treasure's gold. On a desktop the square is picked on the map as the master sees it (the
 * grid in the rules' squares, with the calibration drawn, the walls, doors and stairs in the map language, the points that are already
 * there, and the chosen square outlined with the hidden chest on it); the arrow keys move it too. On a phone a finger would hit
 * squares of 11 px, so the master picks a room of a generated dungeon, and a map without rooms puts the point in its middle.
 *
 * **One idempotency key per request:** the key is made when the sheet opens and again whenever the map, the square or the name
 * changes, so a retry after a lost answer (the same request) gets the same point back, and a different request is never refused for
 * reusing a key. A `content_version` that changed says "gere de novo" and offers it.
 */
@Component({
  selector: 'app-place-sheet',
  imports: [
    EditorOverlay,
    MapLayersLegend,
    MapPinsLegend,
    MapView,
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    SelectField,
    SheetFrame,
    TextField,
  ],
  templateUrl: './place-sheet.html',
  styleUrl: './place-sheet.scss',
})
export class PlaceSheet {
  private readonly treasureApi = inject(TreasureClient);
  private readonly mapsApi = inject(MapsClient);
  private readonly dungeons = inject(DungeonsClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly sheet = injectSheet<PlaceSheetData, PlaceSheetResult>();
  protected readonly data = this.sheet.data;
  protected readonly phone = this.sheet.inSheet;
  protected readonly nameMax = NAME_MAX;

  protected readonly mapsState = signal<MapsState>({ status: 'loading' });
  protected readonly detail = signal<DetailState>({ status: 'loading' });
  protected readonly mapId = signal('');
  protected readonly square = signal<Square | null>(null);
  protected readonly name = signal('');
  protected readonly busy = signal(false);
  protected readonly failure = signal<PlaceFailure | null>(null);

  /** One key for this request; replaced whenever the request changes. */
  private key = newKey();
  private detailGeneration = 0;

  protected readonly mapOptions = computed<SelectOption[]>(() => {
    const s = this.mapsState();
    return s.status === 'ready'
      ? s.maps.map((m) => ({
          value: m.id,
          label: m.gridColumns > 0 ? m.name : `${m.name} · sem grade`,
        }))
      : [];
  });
  protected readonly map = computed(() => {
    const d = this.detail();
    return d.status === 'ready' ? d.map : null;
  });
  protected readonly hasGrid = computed(() => (this.map()?.gridColumns ?? 0) > 0);
  protected readonly rooms = computed(() => {
    const d = this.detail();
    return d.status === 'ready' ? d.rooms : [];
  });
  protected readonly image = computed(() => {
    const m = this.map();
    return m?.image ? { url: m.image.url, width: m.image.width, height: m.image.height } : null;
  });
  /** The map's real points and the hidden chest of the treasure on the chosen square. */
  protected readonly points = computed<readonly MapPoint[]>(() => {
    const d = this.detail();
    const sq = this.square();
    if (d.status !== 'ready' || !sq || d.map.gridColumns <= 0) {
      return d.status === 'ready' ? d.points : [];
    }
    const preview = create(MapPointSchema, {
      id: 'treasure-preview',
      name: 'Tesouro',
      kind: MapPointKind.TREASURE,
      xBp: Math.round(((sq.col + 0.5) / d.map.gridColumns) * 10000),
      yBp: Math.round(((sq.row + 0.5) / d.map.gridRows) * 10000),
      revealed: false,
    });
    return [...d.points, preview];
  });
  protected readonly layers = computed(() => {
    const d = this.detail();
    return d.status === 'ready' ? d.layers : NO_LAYERS;
  });
  protected readonly stairKinds = computed(() => {
    const pts = this.points();
    return { up: pts.some((p) => p.stairs === 1), down: pts.some((p) => p.stairs === 2) };
  });
  /** The solid outline (MAP-LANGUAGE-E10): the room of the chosen square, or the square itself where there is no room; the hidden chest marks the square. */
  protected readonly outline = computed(() => {
    const sq = this.square();
    const f = this.room()?.floor;
    return f
      ? { x: f.x, y: f.y, width: f.width, height: f.height }
      : sq
        ? { x: sq.col, y: sq.row, width: 1, height: 1 }
        : null;
  });
  /** The room the chosen square is in ("Sala 3"), `null` when it is in none. */
  protected readonly room = computed(() => {
    const sq = this.square();
    return sq
      ? (this.rooms().find(
          (r) =>
            r.floor &&
            sq.col >= r.floor.x &&
            sq.col < r.floor.x + r.floor.width &&
            sq.row >= r.floor.y &&
            sq.row < r.floor.y + r.floor.height,
        ) ?? null)
      : null;
  });
  protected readonly placeText = computed(() => {
    const r = this.room();
    return r ? `Sala ${r.id}` : '';
  });
  protected readonly modeName = computed(() =>
    this.data.treasure.mode === TreasureMode.HOARD ? 'Tesouro de covil' : 'Tesouro individual',
  );
  protected readonly kindsText = computed(() =>
    goldKinds(this.data.treasure.mode === TreasureMode.HOARD),
  );
  protected readonly goldText = computed(() => po(this.data.treasure.goldPo));
  protected readonly itemCount = computed(() => this.data.treasure.items.length);
  protected readonly inside = computed(() => {
    const t = this.data.treasure;
    const parts = [
      ...coinRows(t.coins).map((c) => c.count),
      ...pieceRows(t.gems).map((g) => g.title),
      ...pieceRows(t.art).map((a) => a.title),
      ...groupItems(t.items).map((i) => i.title),
    ];
    return parts.join(', ');
  });
  protected readonly goldSentence = computed(() =>
    goldLine(this.data.xpMode, this.data.campaignName),
  );
  protected readonly canPlace = computed(
    () =>
      this.detail().status === 'ready' && this.hasGrid() && this.square() !== null && !this.busy(),
  );
  protected readonly noGridText = NO_GRID_TEXT;
  /** What a screen reader hears about the chosen square (never drawn: the artboard says no coordinates on screen). */
  protected readonly squareSpeech = computed(() => {
    const sq = this.square();
    return sq
      ? `Quadrado escolhido: coluna ${sq.col + 1}, linha ${sq.row + 1}${this.placeText() ? `, na ${this.placeText()}` : ''}.`
      : '';
  });

  constructor() {
    // While PlaceTreasure runs the sheet stays: closing would lose the answer and a second try would be a new request.
    effect(() => this.sheet.lock(this.busy()));
    void this.loadMaps();
  }

  protected async loadMaps(): Promise<void> {
    this.mapsState.set({ status: 'loading' });
    try {
      const maps = await this.mapsApi.list(this.data.campaignId);
      this.mapsState.set({ status: 'ready', maps });
      // The map of the session, or the newest one with a grid (the server lists the newest last or first: take the session's, then a grid).
      const first = maps.find((m) => m.current) ?? maps.find((m) => m.gridColumns > 0) ?? maps[0];
      if (first) {
        await this.chooseMap(first.id);
      }
    } catch {
      this.mapsState.set({ status: 'error' });
    }
  }

  protected async chooseMap(id: string, keepFailure = false): Promise<void> {
    const generation = ++this.detailGeneration;
    this.mapId.set(id);
    this.square.set(null);
    if (!keepFailure) {
      this.failure.set(null);
    }
    this.key = newKey();
    this.detail.set({ status: 'loading' });
    try {
      const got = await this.mapsApi.get(this.data.campaignId, id);
      const map = got.map;
      if (!map) {
        throw new Error('GetMap answered without a map');
      }
      // The layers and the rooms only dress the map: a failed read still lets the master pick a square.
      const [layers, rooms] = await Promise.all([
        map.gridColumns > 0
          ? this.mapsApi.layers(this.data.campaignId, id).then(
              (l) => decodeLayers(l),
              () => NO_LAYERS,
            )
          : Promise.resolve(NO_LAYERS),
        map.generatedDungeon
          ? this.dungeons.rooms(this.data.campaignId, id).then(
              (r) => r.rooms,
              () => [] as DungeonRoom[],
            )
          : Promise.resolve([] as DungeonRoom[]),
      ]);
      if (generation !== this.detailGeneration) {
        return;
      }
      this.detail.set({ status: 'ready', map, points: got.points, layers, rooms });
      this.square.set(this.startSquare(map, rooms));
    } catch {
      if (generation === this.detailGeneration) {
        this.detail.set({ status: 'error' });
      }
    }
  }

  /** The middle of the first room of a generated dungeon, or of the map. */
  private startSquare(map: MapMessage, rooms: readonly DungeonRoom[]): Square | null {
    if (map.gridColumns <= 0 || map.gridRows <= 0) {
      return null;
    }
    const first = rooms[0];
    return first
      ? { col: first.centerCol, row: first.centerRow }
      : { col: Math.floor(map.gridColumns / 2), row: Math.floor(map.gridRows / 2) };
  }

  protected pickRoom(room: DungeonRoom): void {
    this.setSquare({ col: room.centerCol, row: room.centerRow });
  }

  protected roomChosen(room: DungeonRoom): boolean {
    return this.room()?.id === room.id;
  }

  protected onMapClick(at: { xBp: number; yBp: number }): void {
    const m = this.map();
    if (m && m.gridColumns > 0) {
      this.setSquare(squareAt(at.xBp / 10000, at.yBp / 10000, m.gridColumns, m.gridRows));
    }
  }

  protected onKeydown(event: KeyboardEvent): void {
    const m = this.map();
    const sq = this.square();
    if (!m || !sq || m.gridColumns <= 0) {
      return;
    }
    const step = event.shiftKey ? SHIFT_STEP : 1;
    const by: Record<string, [number, number]> = {
      ArrowLeft: [-step, 0],
      ArrowRight: [step, 0],
      ArrowUp: [0, -step],
      ArrowDown: [0, step],
    };
    const move = by[event.key];
    if (!move) {
      return;
    }
    event.preventDefault();
    this.setSquare({
      col: Math.min(m.gridColumns - 1, Math.max(0, sq.col + move[0])),
      row: Math.min(m.gridRows - 1, Math.max(0, sq.row + move[1])),
    });
  }

  private setSquare(sq: Square): void {
    const cur = this.square();
    if (!cur || cur.col !== sq.col || cur.row !== sq.row) {
      this.key = newKey();
    }
    this.square.set(sq);
    this.failure.set(null);
  }

  protected setName(value: string): void {
    this.name.set(value);
    this.key = newKey();
  }

  protected async place(): Promise<void> {
    const sq = this.square();
    const m = this.map();
    if (!sq || !m || !this.canPlace()) {
      return;
    }
    this.busy.set(true);
    this.failure.set(null);
    const t = this.data.treasure;
    try {
      const res = await this.treasureApi.place({
        campaignId: this.data.campaignId,
        mapId: m.id,
        mode: t.mode,
        partyLevel: t.partyLevel,
        seed: t.seed,
        contentVersion: t.contentVersion,
        column: sq.col,
        row: sq.row,
        name: this.name().trim(),
        idempotencyKey: this.key,
      });
      if (!res.point) {
        throw new Error('PlaceTreasure answered without a point');
      }
      this.sheet.close({
        kind: 'placed',
        point: res.point,
        mapId: m.id,
        mapName: m.name,
        place: this.placeText(),
        goldPo: res.treasure?.goldPo ?? t.goldPo,
        itemCount: res.treasure?.items.length ?? t.items.length,
      });
    } catch (err) {
      this.failure.set(placeFailure(err));
      this.host.nativeElement.querySelector<HTMLElement>('.notice')?.focus?.();
      if (ConnectError.from(err, Code.Unavailable).code === Code.InvalidArgument) {
        // The square is outside the grid the server has now: the map changed under the dialog, so read it again and redraw.
        void this.chooseMap(m.id, true);
      }
    } finally {
      this.busy.set(false);
    }
  }

  protected generateAgain(): void {
    this.sheet.close({ kind: 'again' });
  }

  protected cancel(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close();
  }
}
