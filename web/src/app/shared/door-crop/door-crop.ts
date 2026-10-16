import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';

import { type MapLayers, NO_LAYERS, decodeLayers } from '../../core/maps/layers';
import { MapsClient } from '../../core/maps/maps-client';
import { MapLayersOverlay } from '../map-layers/map-layers';

/** The window around the door, in squares. */
const WINDOW_COLS = 7;
const WINDOW_ROWS = 5;

/**
 * The door a solved puzzle opened, on its map (E10-06 state 4): a window of 7 × 5 squares of the map around the door, with the
 * map's own layers drawn over the image (the door is now open, in the map's own door mark). It is the master's alone (the
 * player's page tells only the words the master wrote). Quiet on failure: without the picture the line of words above it says the
 * same, so a map that cannot be read leaves nothing behind.
 */
@Component({
  selector: 'app-door-crop',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MapLayersOverlay],
  template: `
    @if (view(); as v) {
      <figure class="dc">
        <div class="dc__window" role="img" [attr.aria-label]="label()">
          <div class="dc__stage" [style.--cols]="v.cols" [style.--rows]="v.rows" [style.width.%]="(v.cols / win.cols) * 100" [style.height.%]="(v.rows / win.rows) * 100" [style.left.%]="(-v.left / win.cols) * 100" [style.top.%]="(-v.top / win.rows) * 100">
            <img [src]="v.imageUrl" alt="" width="1" height="1" />
            <app-map-layers [layers]="v.layers" />
          </div>
        </div>
        <figcaption class="dc__caption">{{ v.mapName }} · coluna {{ col() + 1 }}, linha {{ row() + 1 }}</figcaption>
      </figure>
    }
  `,
  styleUrl: './door-crop.scss',
})
export class DoorCrop {
  private readonly maps = inject(MapsClient);
  protected readonly win = { cols: WINDOW_COLS, rows: WINDOW_ROWS };

  readonly campaignId = input.required<string>();
  readonly mapId = input.required<string>();
  readonly col = input.required<number>();
  readonly row = input.required<number>();

  protected readonly data = signal<{
    readonly cols: number;
    readonly rows: number;
    readonly imageUrl: string;
    readonly mapName: string;
    readonly layers: MapLayers;
  } | null>(null);

  protected readonly view = computed(() => {
    const d = this.data();
    if (!d || d.cols <= 0 || d.rows <= 0) {
      return null;
    }
    // The window slides to stay on the map: a door at an edge sits at the edge of the picture.
    const left = Math.max(
      0,
      Math.min(this.col() - Math.floor(WINDOW_COLS / 2), d.cols - WINDOW_COLS),
    );
    const top = Math.max(
      0,
      Math.min(this.row() - Math.floor(WINDOW_ROWS / 2), d.rows - WINDOW_ROWS),
    );
    return { ...d, left: Math.max(0, left), top: Math.max(0, top) };
  });
  protected readonly label = computed(
    () =>
      `A porta aberta no mapa ${this.data()?.mapName ?? ''}, coluna ${this.col() + 1}, linha ${this.row() + 1}`,
  );

  constructor() {
    // Another map (the form's select): read it again; an answer for the map before is dropped.
    effect(() => {
      const mapId = this.mapId();
      untracked(() => void this.load(mapId));
    });
  }

  private async load(mapId: string): Promise<void> {
    this.data.set(null);
    try {
      const [map, layers] = await Promise.all([
        this.maps.get(this.campaignId(), mapId),
        this.maps.layers(this.campaignId(), mapId),
      ]);
      const m = map.map;
      if (!m || mapId !== this.mapId()) {
        return;
      }
      this.data.set({
        cols: m.gridColumns,
        rows: m.gridRows,
        imageUrl: m.image?.url ?? '',
        mapName: m.name,
        layers: decodeLayers(layers) ?? NO_LAYERS,
      });
    } catch {
      // Quiet: the words say the same.
    }
  }
}
