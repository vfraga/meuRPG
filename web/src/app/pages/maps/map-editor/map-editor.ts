import {
  ChangeDetectionStrategy,
  Component,
  HostListener,
  DestroyRef,
  ElementRef,
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
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { CharacterKind } from '../../../../gen/meurpg/characters/v1/characters_pb';
import { MapPointKind, type TrapSpecSchema } from '../../../../gen/meurpg/maps/v1/maps_pb';
import type {
  Map as MapMessage,
  MapPoint,
  SceneAction,
  SceneClue,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import { TrapTargets, TrapTrigger } from '../../../../gen/meurpg/rules/v1/rules_pb';
import type { MessageInitShape } from '@bufbuild/protobuf';
import { TrapPresets } from '../../../core/traps/trap-presets';
import { LightPresets } from '../../../core/maps/light-presets';
import { editorErrorMessage, mapErrorMessage } from '../../../core/maps/map-errors';
import { DungeonInfo } from '../../../core/maps/dungeon-info';
import { DungeonsClient } from '../../../core/maps/dungeons-client';
import { MapState, tokenKey } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { MoveSaves } from '../../../core/maps/move-saves';
import { paintHint } from '../../../core/maps/paint-tools';
import { mapTokenInitial } from '../../../core/maps/token-initial';
import { RosterClient, RosterEntry } from '../../../core/maps/roster-client';
import type { CluePlayer } from '../../../core/maps/scene-clues';
import { ViewAsCounts } from '../../../core/maps/view-as';

import { ViewAsList, type ViewAsPerson } from '../../../shared/fog-map/view-as-list';
import { ViewAsMapView } from '../../../shared/fog-map/view-as-map';
import { MapAsk } from '../map-ask/map-ask';
import { MapLayersLegend } from '../../../shared/map-layers/map-layers-legend';
import { MapPinsLegend } from '../../../shared/map-pins/map-pins-legend';
import type { PickRow } from '../../../shared/person-pick/person-pick';
import { MapLegend } from '../../../shared/map-view/map-legend/map-legend';
import { BP_MAX } from '../../../shared/map-view/map-geometry';
import { MapMove, MapSelection, MapView } from '../../../shared/map-view/map-view';
import { CoverDegrees } from '../cover-degrees/cover-degrees';
import { EditorBar, type EditorMode } from '../editor-bar/editor-bar';
import { EditorOverlay, type LightReach } from '../editor-overlay/editor-overlay';
import { FogPanel } from '../fog-panel/fog-panel';
import { GridPanel } from '../grid-panel/grid-panel';
import { LayersPanel } from '../layers-panel/layers-panel';
import { PaintSurface } from '../paint-surface/paint-surface';
import { DoorPanel } from '../door-panel/door-panel';
import { DungeonImage } from '../dungeon-image/dungeon-image';
import { DungeonRooms, type RoomOutline } from '../dungeon-rooms/dungeon-rooms';
import { EditorPainting, type Occupant } from './editor-painting';
import { LightPointPanel } from '../point-kinds/light-point-panel';
import { TrapPointPanel } from '../point-kinds/trap-point-panel';
import { TreasurePointPanel } from '../point-kinds/treasure-point-panel';
import { PointList } from '../point-list/point-list';
import { PointPanel } from '../point-panel/point-panel';
import type { PointChanges } from '../../../core/maps/maps-client';
import { factorLabel } from '../../../core/maps/calibration';
import { newKey } from '../../../core/connect/idempotency';

/** The kinds the editor creates: the three it always had, and the new ones. */
function defaultName(kind: MapPointKind): string {
  switch (kind) {
    case MapPointKind.BATTLE:
      return 'Nova batalha';
    case MapPointKind.SUBMAP:
      return 'Novo submapa';
    case MapPointKind.LIGHT:
      return 'Nova luz';
    case MapPointKind.TRAP:
      return 'Nova armadilha';
    case MapPointKind.TREASURE:
      return 'Novo tesouro';
    default:
      return 'Nova cena';
  }
}

/** What each point panel offers the page: the shape `PointPanel` always had. */
interface PointPanelApi {
  changes(): PointChanges | null | undefined;
  discard(): void;
}

/**
 * The master's map editor on a computer (E5-23, E9-01, E9-02, MR-008, MR-034, MR-035, MR-036, MR-041): a bar, the map and a side
 * column, in two modes.
 *
 * - **Pontos** (the editor it always was): "Adicionar ponto" (Batalha, Submapa, Cena de RP, and now Luz, Armadilha and Tesouro),
 *   drag a point or a token, the arrows move the selected one, and the side panel edits it with "Salvar ponto" (a Luz, an
 *   Armadilha and a Tesouro have their own panels). With nothing selected the side column lists the points, and, when the fog
 *   is on, "Ver como": the map as one player gets it, closed by "Voltar à sua vista".
 * - **Pintar:** Terreno difícil, Parede, Cobertura (Meia, Três quartos), Luz (Claro, Penumbra, Escuro), "Apagar" and the brush
 *   (1×1, 3×3). A drag paints square by square, a click one, Shift erases; the arrows move the brush, Espaço paints, Esc leaves.
 *   Strokes show at once and go to the server in batches (`PaintQueue`), "Tudo salvo" says they arrived. Beside the map, "Camadas"
 *   and "Grade"; under it, "Névoa de guerra". Painting is allowed while a combat runs on the map; the grid and the image are not.
 *
 * Without a grid there is nothing to paint or to light: the paint tools say so and cannot act. The map's image says it is
 * visible to the players ("O que está desenhado na imagem, os jogadores veem."), on the page's header.
 */
@Component({
  selector: 'app-map-editor',
  imports: [
    CoverDegrees,
    DoorPanel,
    DungeonImage,
    DungeonRooms,
    EditorBar,
    EditorOverlay,
    FogPanel,
    GridPanel,
    LayersPanel,
    LightPointPanel,
    MapAsk,
    MapLayersLegend,
    MapLegend,
    MapPinsLegend,
    MapView,
    MatButtonModule,
    MatIconModule,
    PaintSurface,
    PointList,
    PointPanel,
    TrapPointPanel,
    TreasurePointPanel,
    ViewAsList,
    ViewAsMapView,
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './map-editor.html',
  styleUrl: './map-editor.scss',
})
export class MapEditor {
  private readonly api = inject(MapsClient);
  /** Saves each point's and token's moves one at a time (see `MoveSaves`). */
  private readonly moves = new MoveSaves();
  private readonly roster = inject(RosterClient);
  /** What the server says of a generated dungeon: its rooms list and whether the image is still the generator's (the master only; `null` on any other map). */
  protected readonly dungeon = new DungeonInfo(inject(DungeonsClient));
  /** The room outlined on the map (chosen in the rooms list). */
  protected readonly roomOutline = signal<RoomOutline | null>(null);
  private readonly lightPresets = inject(LightPresets);
  private readonly trapPresets = inject(TrapPresets);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly campaignId = input.required<string>();
  readonly state = input.required<MapState>();
  /** The campaign's maps, for "Leva para". */
  readonly maps = input<readonly { id: string; name: string }[]>([]);
  /** A combat that has not ended runs on this map: the grid and the image cannot change, painting can. */
  readonly combatRunning = input(false);
  /** The open session's number, when there is one: a treasure marked found counts in its summary. */
  readonly sessionNumber = input<number | null>(null);

  /** Whether a new grid or a new image would erase something (what is painted, or the fog's memory): the header asks first. */
  readonly erasesChange = output<boolean>();
  /** The page's flags may be out of date (a refusal said a combat runs, or does not): read them again. */
  readonly staleFlags = output<void>();

  protected readonly map = computed(() => this.state().map());
  /** What a square of the drawing is worth, for the legend of a calibrated map: "3 m". */
  protected readonly factorText = computed(() => factorLabel(this.map()?.squareFactor || 1));
  protected readonly view = viewChild(MapView);
  private readonly pointPanel = viewChild(PointPanel);
  private readonly lightPanel = viewChild(LightPointPanel);
  private readonly trapPanel = viewChild(TrapPointPanel);
  private readonly treasurePanel = viewChild(TreasurePointPanel);

  // ---- the point editor ----
  protected readonly mode = signal<EditorMode>('points');
  protected readonly placing = signal<MapPointKind | null>(null);
  protected readonly selection = signal<MapSelection | null>(null);
  protected readonly pending = signal<MapSelection | null>(null);
  /** The mode the master asked for while the point on the panel has unsaved changes: the same question as for another point. */
  protected readonly pendingMode = signal<EditorMode | null>(null);
  protected readonly dirty = signal(false);
  protected readonly saving = signal(false);
  protected readonly panelError = signal<string | null>(null);
  protected readonly message = signal('');
  protected readonly justCreated = signal(false);
  protected readonly everyone = signal<readonly RosterEntry[]>([]);
  protected readonly lightReach = signal<LightReach | null>(null);
  protected readonly lightNames = signal<ReadonlyMap<string, string>>(new Map());

  // ---- painting ----
  /** The tool, the layers, the queue that saves the strokes and the question on leaving (`EditorPainting`). */
  protected readonly paint = new EditorPainting(
    this.campaignId,
    this.map,
    this.mode,
    () => this.refreshFlags(),
    computed(() => this.occupants()),
  );

  /** Who stands on which square (the tokens), for the question before a door goes over them. */
  private readonly occupants = computed<readonly Occupant[]>(() => {
    const cols = this.map()?.gridColumns ?? 0;
    const rows = this.map()?.gridRows ?? 0;
    if (cols === 0 || rows === 0) {
      return [];
    }
    return this.state()
      .tokens()
      .map((t) => ({
        col: Math.min(cols - 1, Math.floor((t.xBp / BP_MAX) * cols)),
        row: Math.min(rows - 1, Math.floor((t.yBp / BP_MAX) * rows)),
        name: t.name,
      }));
  });

  // ---- "Ver como" ----
  protected readonly viewAs = signal<string | null>(null);
  protected readonly viewNote = signal('');
  protected readonly viewGone = signal('');
  protected readonly counts = new ViewAsCounts((mapId, characterId) =>
    this.api.vision(this.campaignId(), mapId, characterId),
  );
  private countsTimer: ReturnType<typeof setTimeout> | undefined;

  protected readonly mapId = computed(() => this.map()?.id ?? '');
  private readonly imageId = computed(() => this.map()?.image?.id ?? '');
  /** The server says (to the master only) whether the map is a generated dungeon: only then are its rooms asked for. */
  private readonly isDungeon = computed(() => this.map()?.generatedDungeon ?? false);
  protected readonly image = computed(() => {
    const image = this.state().map()?.image;
    return image ? { url: image.url, width: image.width, height: image.height } : null;
  });
  protected readonly selectedPoint = computed(() => {
    const s = this.selection();
    return s?.kind === 'point'
      ? (this.state()
          .points()
          .find((p) => p.id === s.id) ?? null)
      : null;
  });
  protected readonly selectedToken = computed(() => {
    const s = this.selection();
    return s?.kind === 'token'
      ? (this.state()
          .tokens()
          .find((t) => tokenKey(t) === s.id) ?? null)
      : null;
  });
  protected readonly pendingName = computed(() => {
    const s = this.selection();
    return (
      this.state()
        .points()
        .find((p) => p.id === s?.id)?.name ?? ''
    );
  });
  /** The player characters, to say who has each clue ("Todos", "Só Brisa"). */
  protected readonly players = computed<readonly CluePlayer[]>(() =>
    this.everyone()
      .filter((c) => c.kind === CharacterKind.PLAYER && c.playerUserId !== '')
      .map((c) => ({ id: c.id, name: c.name, playerName: c.playerName ?? '' })),
  );
  /** Who found a treasure: the living player characters, with their class and player. */
  protected readonly finders = computed<readonly PickRow[]>(() =>
    this.everyone()
      .filter((c) => c.kind === CharacterKind.PLAYER && c.playerUserId !== '')
      .map((c) => ({
        id: c.id,
        name: c.name,
        sub: [c.classSummary, c.playerName].filter(Boolean).join(' · '),
      })),
  );
  protected readonly viewAsPeople = computed<readonly ViewAsPerson[]>(() =>
    this.players().map((p) => ({ id: p.id, name: p.name, sub: p.playerName })),
  );
  protected readonly viewAsPerson = computed(
    () => this.viewAsPeople().find((p) => p.id === this.viewAs()) ?? null,
  );
  protected readonly available = computed(() => {
    const onMap = new Set(
      this.state()
        .tokens()
        .map((t) => t.characterId),
    );
    return this.everyone().filter((c) => !onMap.has(c.id));
  });
  protected readonly hint = computed(() => {
    if (this.mode() === 'paint') {
      return this.paint.canPaint() ? paintHint(this.paint.settings()) : null;
    }
    return this.placing() === null
      ? 'Arraste para mover. As setas movem o item escolhido.'
      : 'Clique no mapa para pôr o ponto.';
  });
  protected readonly Trap = MapPointKind.TRAP;
  protected readonly Treasure = MapPointKind.TREASURE;
  protected readonly Light = MapPointKind.LIGHT;

  protected readonly initialOf = mapTokenInitial;

  constructor() {
    afterNextRender(() => {
      this.roster.list(this.campaignId()).then(
        (list) => this.everyone.set(list),
        () => undefined,
      );
      this.lightPresets.list(this.campaignId()).then(
        (list) => this.lightNames.set(new Map(list.map((o) => [o.key, o.name]))),
        () => undefined,
      );
    });
    inject(DestroyRef).onDestroy(() => clearTimeout(this.countsTimer));

    // A generated dungeon's rooms: read when a map opens and when its image changes ("Gerada pelo app" is about the image).
    effect(() => {
      const id = this.mapId();
      void this.imageId();
      const dungeon = this.isDungeon();
      untracked(() => {
        if (id === '' || !dungeon) {
          this.dungeon.clear();
          return;
        }
        void this.dungeon.load(this.campaignId(), id);
      });
    });
    effect(() => {
      void this.mapId();
      untracked(() => this.roomOutline.set(null));
    });

    // The light's rings go with the selection of a light.
    effect(() => {
      if (this.selectedPoint()?.kind !== MapPointKind.LIGHT) {
        untracked(() => this.lightReach.set(null));
      }
    });
    effect(() => this.erasesChange.emit(this.paint.erases()));

    // "Ver como" counts ("22 quadrados vistos"): read when the fog is on and the characters are known.
    effect(() => {
      const map = this.map();
      const ids = this.viewAsPeople().map((p) => p.id);
      if (!map?.fogEnabled || ids.length === 0) {
        return;
      }
      untracked(() => {
        clearTimeout(this.countsTimer);
        this.countsTimer = setTimeout(() => void this.counts.read(map.id, ids), 250);
      });
    });
    // The fog went off: nothing to look as.
    effect(() => {
      if (this.map()?.fogEnabled === false && this.viewAs() !== null) {
        untracked(() => this.setViewAs(null));
      }
    });
  }

  // ---- the mode ----

  protected setMode(mode: EditorMode): void {
    if (mode === this.mode()) {
      return;
    }
    if (mode === 'paint' && this.dirtyBlocks()) {
      this.pendingMode.set('paint');
      return;
    }
    this.applyMode(mode);
  }

  private applyMode(mode: EditorMode): void {
    this.pendingMode.set(null);
    if (mode === 'paint') {
      this.placing.set(null);
      this.selection.set(null);
      this.pending.set(null);
      this.setViewAs(null);
    } else {
      this.paint.stop();
    }
    this.mode.set(mode);
  }

  /** Esc leaves the paint surface: the focus goes back to the chosen tool. */
  protected leaveSurface(): void {
    this.host.nativeElement
      .querySelector<HTMLElement>('app-editor-bar [aria-pressed="true"]')
      ?.focus();
  }

  protected refreshFlags(): void {
    this.staleFlags.emit();
  }

  /** For the route guard: true when it is fine to leave (see `EditorPainting.confirmLeave`). */
  confirmLeave(): Promise<boolean> {
    return this.paint.confirmLeave();
  }

  /** The tab closes with strokes unsaved: the browser's own question. */
  @HostListener('window:beforeunload', ['$event'])
  protected onBeforeUnload(event: BeforeUnloadEvent): void {
    if (this.paint.queue.busy) {
      event.preventDefault();
    }
  }

  // ---- toolbar ----

  protected toggleKind(kind: MapPointKind): void {
    this.placing.update((current) => (current === kind ? null : kind));
  }

  protected async addToken(entry: RosterEntry): Promise<void> {
    const mapId = this.mapId();
    if (!mapId) {
      return;
    }
    const at = this.view()?.centerBp() ?? { xBp: 5000, yBp: 5000 };
    try {
      const token = await this.api.placeToken(this.campaignId(), mapId, entry.id, at.xBp, at.yBp);
      this.state().upsertToken(token);
      this.selection.set({ kind: 'token', id: token.characterId });
      this.message.set(`${entry.name} está no mapa.`);
    } catch (err) {
      this.message.set(mapErrorMessage(err, 'pôr o token'));
    }
  }

  // ---- the map ----

  protected onEmptyClick(at: { xBp: number; yBp: number }): void {
    const kind = this.placing();
    if (kind === null) {
      this.requestSelect(null);
      return;
    }
    void this.createPoint(kind, at);
  }

  private async createPoint(kind: MapPointKind, at: { xBp: number; yBp: number }): Promise<void> {
    const mapId = this.mapId();
    if (!mapId) {
      return;
    }
    if (this.dirtyBlocks()) {
      this.message.set('Salve ou descarte as mudanças do ponto escolhido antes de pôr outro.');
      return;
    }
    this.placing.set(null);
    try {
      // One key per tap on the map: the point is made once, whatever the network does with the answer.
      const point = await this.api.createPoint(
        this.campaignId(),
        mapId,
        {
          kind,
          name: defaultName(kind),
          description: '',
          xBp: at.xBp,
          yBp: at.yBp,
          ...(kind === MapPointKind.LIGHT ? { light: await this.newLight() } : {}),
          ...(kind === MapPointKind.TRAP ? { trap: await this.newTrap() } : {}),
          ...(kind === MapPointKind.TREASURE ? { treasureValuePo: 0 } : {}),
        },
        newKey(),
      );
      this.state().upsertPoint(point);
      this.panelError.set(null);
      this.justCreated.set(true);
      this.selection.set({ kind: 'point', id: point.id });
      this.message.set('Ponto criado, escondido dos jogadores. Dê um nome a ele.');
    } catch (err) {
      this.message.set(editorErrorMessage(err, 'pointNew', 'criar o ponto'));
    }
  }

  /** A new light starts as the first source the server lists (the server fills the radii of a preset sent with none). */
  private async newLight(): Promise<{ presetKey: string }> {
    const first = (await this.lightPresets.list(this.campaignId()))[0];
    return { presetKey: first?.key ?? '' };
  }

  /** A new trap starts with the numbers of the first sample trap the server lists, with no effect yet: the form edits the rest. */
  private async newTrap(): Promise<MessageInitShape<typeof TrapSpecSchema>> {
    const first = (await this.trapPresets.list(this.campaignId())).presets[0];
    return {
      findDc: first?.findDc ?? 10,
      areaSize: 1,
      trigger: TrapTrigger.ENTER,
      effect: { targets: TrapTargets.AREA },
    };
  }

  /** Unsaved changes: the page asks before moving on. */
  private dirtyBlocks(): boolean {
    return this.dirty() && this.selection()?.kind === 'point';
  }

  protected requestSelect(next: MapSelection | null): void {
    const current = this.selection();
    if (current?.kind === next?.kind && current?.id === next?.id) {
      return;
    }
    if (this.dirtyBlocks()) {
      this.pending.set(next ?? { kind: 'point', id: '' });
      return;
    }
    this.moveSelection(next);
  }

  private moveSelection(next: MapSelection | null): void {
    this.pending.set(null);
    this.justCreated.set(false);
    this.panelError.set(null);
    this.lightReach.set(null);
    this.selection.set(next && next.id !== '' ? next : null);
  }

  protected async saveAndContinue(): Promise<void> {
    if (await this.save()) {
      this.continueAfterAsk();
    }
  }

  protected discardAndContinue(): void {
    this.activePanel()?.discard();
    this.dirty.set(false);
    this.continueAfterAsk();
  }

  /** What the master was going to do when the question came: another point, or the "Pintar" mode. */
  private continueAfterAsk(): void {
    const mode = this.pendingMode();
    const next = this.pending();
    this.pending.set(null);
    this.pendingMode.set(null);
    this.moveSelection(mode === null ? next : null);
    if (mode !== null) {
      this.applyMode(mode);
    }
  }

  protected keepEditing(): void {
    this.pending.set(null);
    this.pendingMode.set(null);
  }

  /** Whichever point panel is open: they all answer `changes()` and `discard()`. */
  private activePanel(): PointPanelApi | undefined {
    return this.pointPanel() ?? this.lightPanel() ?? this.trapPanel() ?? this.treasurePanel();
  }

  protected async onMoved(move: MapMove): Promise<void> {
    const state = this.state();
    const mapId = this.mapId();
    if (!mapId) {
      return;
    }
    if (move.kind === 'point') {
      const before = state.points().find((p) => p.id === move.id);
      if (!before) {
        return;
      }
      state.upsertPoint({ ...before, xBp: move.xBp, yBp: move.yBp });
      await this.moves.move(
        `${mapId}/point/${move.id}`,
        before,
        { xBp: move.xBp, yBp: move.yBp },
        {
          save: (to) =>
            this.api.updatePoint(this.campaignId(), mapId, move.id, { xBp: to.xBp, yBp: to.yBp }),
          failed: (saved, err) => {
            const now = state.points().find((p) => p.id === move.id);
            if (now) {
              state.upsertPoint({ ...now, xBp: saved.xBp, yBp: saved.yBp });
            }
            this.message.set(mapErrorMessage(err, 'mover o ponto'));
          },
        },
      );
      return;
    }
    const before = state.tokens().find((t) => tokenKey(t) === move.id);
    if (!before) {
      return;
    }
    state.upsertToken({ ...before, xBp: move.xBp, yBp: move.yBp });
    await this.moves.move(
      `${mapId}/token/${move.id}`,
      before,
      { xBp: move.xBp, yBp: move.yBp },
      {
        save: (to) =>
          before.creatureId
            ? this.api.placeToken(this.campaignId(), mapId, '', to.xBp, to.yBp, before.creatureId)
            : this.api.placeToken(this.campaignId(), mapId, move.id, to.xBp, to.yBp),
        failed: (saved, err) => {
          const now = state.tokens().find((t) => tokenKey(t) === move.id);
          if (now) {
            state.upsertToken({ ...now, xBp: saved.xBp, yBp: saved.yBp });
          }
          this.message.set(mapErrorMessage(err, 'mover o token'));
        },
      },
    );
  }

  // ---- the point panels ----

  protected async save(): Promise<boolean> {
    const point = this.selectedPoint();
    const mapId = this.mapId();
    const changes = this.activePanel()?.changes();
    if (!point || !mapId) {
      return false;
    }
    if (changes === undefined) {
      return false;
    }
    if (changes === null) {
      // Nothing changed, or the draft breaks a rule (the fields say which).
      return !this.dirty();
    }
    this.saving.set(true);
    this.panelError.set(null);
    try {
      const saved = await this.api.updatePoint(this.campaignId(), mapId, point.id, changes);
      this.setPoint(saved);
      this.message.set(`${saved.name} salvo.`);
      return true;
    } catch (err) {
      this.panelError.set(editorErrorMessage(err, 'point', 'salvar o ponto'));
      return false;
    } finally {
      this.saving.set(false);
    }
  }

  /** The scene actions saved on their own: the point carries the new list. */
  protected setSceneActions(point: MapPoint, actions: readonly SceneAction[]): void {
    const now =
      this.state()
        .points()
        .find((p) => p.id === point.id) ?? point;
    this.state().upsertPoint({ ...now, sceneActions: [...actions] });
  }

  /** "Mostrar a CD aos jogadores" saved on its own: the point carries it. */
  protected setShowDc(point: MapPoint, showDc: boolean): void {
    const now =
      this.state()
        .points()
        .find((p) => p.id === point.id) ?? point;
    this.state().upsertPoint({ ...now, showDc });
  }

  /** The clues saved on their own: the point carries the new list. */
  protected setClues(point: MapPoint, clues: readonly SceneClue[]): void {
    const now =
      this.state()
        .points()
        .find((p) => p.id === point.id) ?? point;
    this.state().upsertPoint({ ...now, clues: [...clues] });
  }

  /** A treasure marked or unmarked found (saved at once): the map carries the point the server answered. */
  protected setPoint(point: MapPoint): void {
    const state = this.state();
    // A move of this point still on its way: the answer was computed before it, so the screen keeps the place the master dragged to.
    const shown = state.points().find((p) => p.id === point.id);
    const ahead = shown && this.moves.isPending(`${this.mapId()}/point/${point.id}`);
    state.upsertPoint(ahead ? { ...point, xBp: shown.xBp, yBp: shown.yBp } : point);
  }

  protected async remove(): Promise<void> {
    const point = this.selectedPoint();
    const mapId = this.mapId();
    if (!point || !mapId) {
      return;
    }
    try {
      await this.api.deletePoint(this.campaignId(), mapId, point.id);
      this.state().removePoint(point.id);
      this.dirty.set(false);
      this.moveSelection(null);
      this.message.set(`${point.name} foi apagado.`);
    } catch (err) {
      this.panelError.set(editorErrorMessage(err, 'point', 'apagar o ponto'));
    }
  }

  // ---- the token panel ----

  protected async toggleTokenHidden(): Promise<void> {
    const token = this.selectedToken();
    const mapId = this.mapId();
    if (!token || !mapId) {
      return;
    }
    try {
      const saved = await this.api.setTokenHidden(
        this.campaignId(),
        mapId,
        token.characterId,
        !token.hidden,
      );
      this.state().upsertToken(saved);
    } catch (err) {
      this.message.set(mapErrorMessage(err, 'mudar o token'));
    }
  }

  protected async removeToken(): Promise<void> {
    const token = this.selectedToken();
    const mapId = this.mapId();
    if (!token || !mapId) {
      return;
    }
    try {
      await this.api.removeToken(this.campaignId(), mapId, token.characterId, token.creatureId);
      if (token.creatureId) {
        this.state().removeCreatureToken(token.creatureId);
      } else {
        this.state().removeToken(token.characterId);
      }
      this.selection.set(null);
      this.message.set(`${token.name} saiu do mapa.`);
    } catch (err) {
      this.message.set(mapErrorMessage(err, 'tirar o token'));
    }
  }

  // ---- the grid and the fog ----

  /** The map after a change of the grid or the fog: the page's state takes it, the layers read again by themselves. */
  protected onMapChanged(map: MapMessage): void {
    this.state().setMap(map);
  }

  /** "Redesenhar" sends the strokes that still wait first, so the new image has the walls just painted. */
  protected readonly beforeRedraw = async (): Promise<string | null> => {
    await this.paint.queue.flush();
    return this.paint.queue.status() === 'error'
      ? 'Há traços que o servidor ainda não recebeu. Espere o aviso “Tudo salvo” e tente de novo.'
      : null;
  };

  /** A room chosen in the list is outlined, and the map goes to it (and back to the whole map when it is let go). */
  protected onRoomOutline(room: RoomOutline | null): void {
    this.roomOutline.set(room);
    const m = this.map();
    if (!room || !m || m.gridColumns <= 0 || m.gridRows <= 0) {
      this.view()?.fit();
      return;
    }
    const scale = Math.min(3, Math.max(1.2, (0.55 * m.gridColumns) / room.width));
    this.view()?.focusOn(
      {
        xBp: ((room.x + room.width / 2) / m.gridColumns) * 10000,
        yBp: ((room.y + room.height / 2) / m.gridRows) * 10000,
      },
      scale,
    );
  }

  protected reloadDungeon(): void {
    void this.dungeon.load(this.campaignId(), this.mapId());
  }

  protected onForgotten(): void {
    if (this.map()?.fogEnabled) {
      void this.counts.read(
        this.mapId(),
        this.viewAsPeople().map((p) => p.id),
      );
    }
  }

  // ---- "Ver como" ----

  protected setViewAs(id: string | null): void {
    this.viewAs.set(id);
    this.viewNote.set('');
    this.viewGone.set('');
  }

  protected viewAsGone(): void {
    this.viewAs.set(null);
    this.viewNote.set('');
    this.viewGone.set('Esse personagem morreu ou saiu da campanha. Voltamos para “Todos”.');
  }
}
