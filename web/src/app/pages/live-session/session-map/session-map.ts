import { Component, computed, inject, input, output, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { RouterLink } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import type { Map as MapMessage, MapPoint } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { describeConnectError } from '../../../core/connect/connect-errors';
import type { DoorSquare } from '../../../core/maps/layers';
import { wallUnderDoor } from '../../../core/maps/door-paint';
import { lightKeyName } from '../../../core/maps/carried-light';
import type { FogView } from '../../../core/maps/fog-view';
import { mapErrorMessage } from '../../../core/maps/map-errors';
import { MapReveals } from '../../../core/maps/map-reveals';
import { MapState, tokenKey } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { MoveSaves } from '../../../core/maps/move-saves';
import { SceneClient } from '../../../core/play/scene-client';
import { sceneErrorMessage } from '../../../core/play/scene-errors';
import type { SceneState } from '../../../core/play/scene-state';
import { isPinKind } from '../../../core/traps/trap-text';
import { FogMap } from '../../../shared/fog-map/fog-map';
import type { ViewAsPerson } from '../../../shared/fog-map/view-as-list';
import { ViewAsMapView } from '../../../shared/fog-map/view-as-map';
import { MapPointsList } from '../../../shared/map-lists/map-points-list';
import { MapPins } from '../../../shared/map-pins/map-pins';
import { MapPinsLegend } from '../../../shared/map-pins/map-pins-legend';
import { MapLegend } from '../../../shared/map-view/map-legend/map-legend';
import { MapMove, MapView } from '../../../shared/map-view/map-view';
import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import { type DoorSheetData, openDoorSheet } from '../door-sheet/door-sheet';
import { FogPlayerTools } from '../fog-tools/fog-player-tools';
import { LiveSessionSource } from '../live-session.types';

/**
 * The map half of the session page (MR-012, E5-02 to E5-06): the session's
 * current map, the same for everyone at the table.
 *
 * - **Player:** a still preview (zoomed in on the party on a phone, the
 *   whole map on a computer) with only the revealed points and the visible
 *   tokens; the whole preview and "Ver mapa" open the full map. Without a
 *   map, "O mestre ainda não escolheu um mapa." The page reads the map
 *   again on `map_changed`; if the server answers `not_found`, the player
 *   lost sight of it and gets the empty state again.
 * - **Fog of war (MR-036, E9-03):** on a map with the fog on, the player gets the
 *   fog map in place (`app-fog-map`: the tiles, the shading, the legend, zoom) and
 *   the row "Luz que você carrega"; the master keeps his whole map and gets "Ver
 *   como" (the list, and the map as one player sees it) and "Luz dos personagens".
 * - **Master:** the "Mapa atual" select (`SetCurrentMap`, which also
 *   reveals a hidden map), the map with hidden things dashed, tokens you
 *   drag (`PlaceMapToken`) on a computer, "Abrir mapa" for the editor, and
 *   "Pontos do mapa" with "Revelar aos jogadores" and "Esconder".
 */
@Component({
  selector: 'app-session-map',
  imports: [
    FogMap,
    FogPlayerTools,
    MapLegend,
    MapPins,
    MapPinsLegend,
    MapPointsList,
    MapView,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    RouterLink,
    ViewAsMapView,
  ],
  templateUrl: './session-map.html',
  styleUrl: './session-map.scss',
})
export class SessionMap {
  private readonly api = inject(MapsClient);
  /** Saves each token's moves one at a time (see `MoveSaves`). */
  private readonly moves = new MoveSaves();
  private readonly source = inject(LiveSessionSource);
  private readonly sceneApi = inject(SceneClient);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);

  readonly campaignId = input.required<string>();
  readonly state = input.required<MapState>();
  /** The session's current map, or `null` while there is none. */
  readonly mapId = input<string | null>(null);
  /** The master's copy speaks to the one who chooses the map. */
  readonly isMaster = input(false);
  /** The campaign's maps, for the master's select. */
  readonly maps = input<readonly MapMessage[]>([]);
  /** The session's RP scene, for "Abrir cena" on a scene point (master). */
  readonly scene = input<SceneState | null>(null);
  /** The player may search this map for traps (they have a character on it and the map has a grid):
   * "Procurar armadilhas" sits beside "Ver mapa" (MR-035, E9-08). */
  readonly searchable = input(false);
  /** The player's own character, for the off-map notice and the light they carry. */
  readonly characterName = input('');
  readonly ownCharacterId = input('');
  /** The character's senses ("Visão no escuro: 18 m"), for the card "Você vê". */
  readonly senses = input<readonly string[]>([]);
  /** What the viewer sees of the map with the fog on (the page reads it again on `vision_changed`). */
  readonly fog = input<FogView | null>(null);
  /** The master's "Ver como" list: the living player characters, and the one he looks as (`null`: "Todos"). */
  readonly people = input<readonly ViewAsPerson[]>([]);
  readonly viewAs = input<string | null>(null);
  /** Goes up when what a player sees may have changed (the master's stream hears nothing of it). */
  readonly tick = input(0);
  /** The player looks through their familiar's eyes ("Nanquim"): the card says whose view it is. */
  readonly seeingFamiliar = input<string | null>(null);
  readonly seeing = input(false);
  readonly creaturesTick = input(0);
  /** The master chose another map (or none): the page shows it. */
  readonly currentChanged = output<string | null>();
  /** "Procurar armadilhas": the page opens the search sheet. */
  readonly searchTraps = output<void>();
  /** "Ver como": the line about the view, and the ways back to "Todos". */
  readonly viewNoteChange = output<string>();
  readonly viewAsGone = output<void>();
  readonly viewAsBack = output<void>();

  protected readonly phone = mediaQuery(PHONE_QUERY);
  protected readonly error = signal<string | null>(null);
  protected readonly busy = signal(false);
  protected readonly sceneError = signal<string | null>(null);
  protected readonly reveals = new MapReveals(
    inject(MapsClient),
    () => this.state(),
    () => this.campaignId(),
  );

  protected readonly map = computed(() => this.state().map());
  protected readonly status = computed(() => this.state().status());
  /** The points the old markers draw: traps, treasures and lights have their own marks, panels and legend. */
  protected readonly markerPoints = computed(() =>
    this.state()
      .points()
      .filter((p) => !isPinKind(p.kind)),
  );
  protected readonly image = computed(() => {
    const image = this.map()?.image;
    return image ? { url: image.url, width: image.width, height: image.height } : null;
  });
  protected readonly openHiddenNote = computed(() => {
    const chosen = this.maps().find((m) => m.id === this.mapId());
    return chosen !== undefined && !chosen.revealed;
  });

  protected readonly pendingScene = signal<string | null>(null);

  // ---- the fog of war (MR-036) ----
  protected readonly fogOn = computed(() => this.map()?.fogEnabled === true);
  /** The player's own token: the carried light is read from it. */
  protected readonly ownToken = computed(
    () =>
      this.state()
        .tokens()
        .find((t) => t.mine && !t.creatureId) ?? null,
  );
  protected readonly viewAsPerson = computed(
    () => this.people().find((p) => p.id === this.viewAs()) ?? null,
  );
  protected readonly viewer = computed(() => ({
    name: this.ownToken()?.name ?? (this.characterName() || 'Seu personagem'),
    own: true,
  }));
  /** The master reads the whole picture. */
  protected readonly masterImage = computed(() => this.image());
  protected readonly carriedName = computed(() =>
    lightKeyName(this.ownToken()?.carriedLight ?? '').toLocaleLowerCase('pt-BR'),
  );
  protected readonly seeingName = computed(() =>
    this.seeing() ? (this.seeingFamiliar() ?? 'familiar') : null,
  );

  /** The master taps a door of the map: its sheet opens (open, close, lock, or reveal a secret door). The map reads itself again on the stream. */
  protected openDoor(door: DoorSquare): void {
    const mapId = this.map()?.id;
    if (!mapId) {
      return;
    }
    const data: DoorSheetData = {
      campaignId: this.campaignId(),
      mapId,
      door,
      wallSquares: wallUnderDoor(
        this.fog()?.layers().walls ?? [],
        this.fog()?.layers().columns ?? 0,
        this.fog()?.layers().rows ?? 0,
        this.map()?.squareFactor ?? 1,
        door,
      ),
    };
    openDoorSheet(this.dialog, this.bottomSheet, data).subscribe();
  }

  protected async openScene(point: MapPoint): Promise<void> {
    const state = this.scene();
    if (!state || this.pendingScene() !== null) {
      return;
    }
    this.pendingScene.set(point.id);
    this.sceneError.set(null);
    try {
      state.openedHere(await this.sceneApi.open(this.campaignId(), point.id));
    } catch (err) {
      this.sceneError.set(sceneErrorMessage(err, 'abrir a cena'));
    } finally {
      this.pendingScene.set(null);
    }
  }

  protected async choose(select: HTMLSelectElement): Promise<void> {
    const mapId = select.value === '' ? null : select.value;
    this.busy.set(true);
    this.error.set(null);
    try {
      const current = await this.source.setCurrentMap(this.campaignId(), mapId);
      this.currentChanged.emit(current);
    } catch (err) {
      select.value = this.mapId() ?? '';
      this.error.set(
        ConnectError.from(err).code === Code.FailedPrecondition
          ? 'A sessão acabou: o mapa só muda durante a sessão.'
          : describeConnectError(err, {
              [Code.NotFound]: 'Esse mapa não existe mais. Recarregue a página.',
              [Code.PermissionDenied]: 'Só o mestre da campanha escolhe o mapa.',
            }),
      );
    } finally {
      this.busy.set(false);
    }
  }

  protected async onMoved(move: MapMove): Promise<void> {
    const state = this.state();
    const mapId = state.map()?.id;
    const before = state.tokens().find((t) => tokenKey(t) === move.id);
    if (move.kind !== 'token' || !mapId || !before) {
      return;
    }
    this.error.set(null);
    state.upsertToken({ ...before, xBp: move.xBp, yBp: move.yBp });
    await this.moves.move(
      `${mapId}/${move.id}`,
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
          this.error.set(mapErrorMessage(err, 'mover o token'));
        },
      },
    );
  }

  protected retry(): void {
    void this.state().refresh();
  }
}
