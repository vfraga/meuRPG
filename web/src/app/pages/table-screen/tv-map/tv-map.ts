import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import type { TvMapVm } from '../../../core/play/table-screen-view';
import { decodeVision } from '../../../core/maps/vision';
import { isPinKind } from '../../../core/traps/trap-text';
import { FogMap } from '../../../shared/fog-map/fog-map';
import { MapPins } from '../../../shared/map-pins/map-pins';
import { MapPinsLegend } from '../../../shared/map-pins/map-pins-legend';
import { MapView } from '../../../shared/map-view/map-view';
import { TvFogCard } from '../tv-fog-card/tv-fog-card';

/** The viewer the tiles are made for: the master's tab asks for the group's view with `as=tv`. */
export const TV_VIEWER = 'tv';

/**
 * The map on the TV, as the server's projection of the group's view: the fog's tiles when the fog is on, the picture
 * as it is when it is off, and the card "O mapa fica na tela de cada um" when the fog hides it. It draws what it is
 * given and decides nothing.
 */
@Component({
  selector: 'app-tv-map',
  imports: [FogMap, MapPins, MapPinsLegend, MapView, TvFogCard],
  templateUrl: './tv-map.html',
  styleUrl: './tv-map.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class TvMap {
  readonly map = input.required<TvMapVm>();

  protected readonly viewer = TV_VIEWER;
  protected readonly data = computed(() => (this.map().hiddenByFog ? null : this.map().map));
  protected readonly info = computed(() => this.data()?.map ?? null);
  protected readonly vision = computed(() => {
    const vision = this.map().vision;
    return vision ? decodeVision(vision) : null;
  });
  /** The fog is on: the tiles and the group's vision. */
  protected readonly fog = computed(
    () => this.info()?.fogEnabled === true && this.vision() !== null,
  );
  protected readonly image = computed(() => {
    const image = this.info()?.image;
    return image ? { url: image.url, width: image.width, height: image.height } : null;
  });
  protected readonly points = computed(() => this.data()?.points ?? []);
  protected readonly markers = computed(() => this.points().filter((p) => !isPinKind(p.kind)));
  protected readonly tokens = computed(() => this.data()?.tokens ?? []);
}
