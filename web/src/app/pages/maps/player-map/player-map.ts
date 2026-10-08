import {
  Component,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';
import { Router, RouterLink } from '@angular/router';

import { type Map as MapMessage, MapPointKind } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { FogView } from '../../../core/maps/fog-view';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { ChestIcon } from '../../../shared/chest-icon/chest-icon';
import { FogMap } from '../../../shared/fog-map/fog-map';
import { MapPinsLegend } from '../../../shared/map-pins/map-pins-legend';
import { mapTokenInitial } from '../../../core/maps/token-initial';
import { MapPins } from '../../../shared/map-pins/map-pins';
import { MapLegend } from '../../../shared/map-view/map-legend/map-legend';
import { pointKindIcon, pointKindLabel } from '../../../shared/map-view/map-labels';
import { visiblePoints } from '../../../shared/map-view/map-geometry';
import { MapSelection, MapView } from '../../../shared/map-view/map-view';
import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import { PointSheet } from '../../../shared/point-sheet/point-sheet';

/**
 * A map as a player reads it (E5-25, E5-26, MR-009): the revealed points
 * and the visible tokens, and nothing else; the server filters, and the
 * map view filters again.
 *
 * - Tapping a point opens its sheet: a non-modal bottom sheet on a phone,
 *   the first block of the side column from a tablet up. On open, focus
 *   goes to the sheet's title; "Fechar", Esc or a tap on the empty map
 *   close it, and focus goes back to the marker.
 * - A Submapa point whose target the player may see offers "Abrir <mapa>"
 *   in the sheet (never a direct jump).
 * - Inside a submap, a breadcrumb shows the path (`parent_maps`); "Mapas
 *   revelados" lists every map the player may open, with `aria-current` on
 *   this one; "Pontos deste mapa" is the way to the points without the
 *   pointer.
 */
@Component({
  selector: 'app-player-map',
  imports: [
    ChestIcon,
    FogMap,
    MapLegend,
    MapPins,
    MapPinsLegend,
    MapView,
    MatIconModule,
    PointSheet,
    RouterLink,
  ],
  templateUrl: './player-map.html',
  styleUrl: './player-map.scss',
})
export class PlayerMap {
  private readonly router = inject(Router);
  private readonly api = inject(MapsClient);
  /** What the player sees of this map when it has the fog on (MR-036): the same drawing as the session's. */
  protected readonly fog = new FogView(
    (mapId, as) => this.api.vision(this.campaignId(), mapId, as ?? ''),
    (mapId, as) => this.api.layers(this.campaignId(), mapId, as ?? ''),
    () => true,
  );

  readonly campaignId = input.required<string>();
  readonly state = input.required<MapState>();
  /** The maps the player may open (ListMaps). */
  readonly maps = input<readonly MapMessage[]>([]);
  /** Opened from the session page: the back link says so. */
  readonly fromSession = input(false);

  protected readonly phone = mediaQuery(PHONE_QUERY);
  protected readonly view = viewChild(MapView);
  protected readonly selection = signal<MapSelection | null>(null);

  protected readonly initialOf = mapTokenInitial;
  protected readonly Treasure = MapPointKind.TREASURE;
  protected readonly icon = pointKindIcon;
  protected readonly kind = pointKindLabel;

  protected readonly map = computed(() => this.state().map());
  protected readonly image = computed(() => {
    const image = this.map()?.image;
    return image ? { url: image.url, width: image.width, height: image.height } : null;
  });
  protected readonly fogOn = computed(() => this.map()?.fogEnabled === true);
  protected readonly viewer = computed(() => ({
    name:
      this.state()
        .tokens()
        .find((t) => t.mine && !t.creatureId)?.name ?? 'Seu personagem',
    own: true,
  }));
  protected readonly parent = computed(() => this.map()?.parentMaps[0] ?? null);
  /** The points this player may see: the server sends no other, and this is the second lock (RN-10) for the pins, the legends, the list and the sheet. */
  protected readonly points = computed(() => visiblePoints(this.state().points(), false));
  protected readonly selected = computed(() => {
    const s = this.selection();
    return s ? (this.points().find((p) => p.id === s.id) ?? null) : null;
  });
  protected readonly backLink = computed(() =>
    this.fromSession()
      ? { path: ['/campaigns', this.campaignId(), 'session'], label: 'Voltar para a sessão' }
      : { path: ['/campaigns', this.campaignId()], label: 'Voltar para a campanha' },
  );

  constructor() {
    effect(() => {
      const id = this.fogOn() ? (this.map()?.id ?? null) : null;
      untracked(() => void this.fog.open(id));
    });
  }

  protected mapSub(m: MapMessage): string {
    if (m.id === this.map()?.id) {
      return 'Você está aqui';
    }
    if (m.current) {
      return 'Mapa atual da sessão';
    }
    return m.parentMaps.length > 0 ? `Submapa de ${m.parentMaps[0].name}` : 'Mapa principal';
  }

  protected openPoint(id: string): void {
    this.selection.set({ kind: 'point', id });
  }

  protected closeSheet(): void {
    const s = this.selection();
    this.selection.set(null);
    if (s) {
      // After the sheet is gone, focus goes back to the marker.
      queueMicrotask(() => this.view()?.focusItem('point', s.id));
    }
  }

  /** "Abrir <mapa>": the sheet's button; the next page loads the map. */
  protected openMap(mapId: string): void {
    void this.router.navigate(['/campaigns', this.campaignId(), 'maps', mapId], {
      queryParamsHandling: 'preserve',
    });
  }
}
