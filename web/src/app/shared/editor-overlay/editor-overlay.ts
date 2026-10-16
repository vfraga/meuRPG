import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import type { MapPoint } from '../../../gen/meurpg/maps/v1/maps_pb';
import type { MapLayers } from '../../core/maps/layers';
import { type Square, squareAt } from '../../core/combat/combat-grid';
import { SQUARE_FT } from '../../core/units';
import { type BrushCursor, MapLayersOverlay } from '../map-layers/map-layers';
import { MapPins } from '../map-pins/map-pins';

/** The reach of a light point on the map: its radii in feet, and the square it stands on. */
export interface LightReach {
  readonly xBp: number;
  readonly yBp: number;
  readonly brightFt: number;
  readonly dimFt: number;
}

/**
 * Everything the master's editor draws over the map's picture (E9-01, E9-02), in one layer that fills the stage:
 * the grid lines, the painted layers (`app-map-layers`, with the light glyphs and the brush cursor), the trap,
 * treasure and light marks (`app-map-pins`) and the reach circles of the selected light. It is the surface that
 * sets `--cols` and `--rows` for the layers. Presentational and decorative (`aria-hidden`): the lists beside the
 * map and the legend under it say the same in words.
 *
 * **The reach is the radii, not the lit squares:** no read gives the master which squares a light reaches (the
 * server's vision is the players'), so the circles show how far the light goes, in squares, and nothing more;
 * the walls that cut it are the fog's business, shown in "Ver como".
 */
@Component({
  selector: 'app-editor-overlay',
  imports: [MapLayersOverlay, MapPins],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (columns() > 0) {
      <svg class="grid" aria-hidden="true" preserveAspectRatio="none" [attr.viewBox]="'0 0 ' + columns() + ' ' + rows()">
        <path [class.rules]="factor() > 1" [attr.d]="gridPath()" />
        @if (factor() > 1) {
          <path class="drawn" [attr.d]="drawnPath()" />
        }
      </svg>
      <app-map-layers [layers]="layers()" [lightGlyphs]="true" [cursor]="cursor()" />
    }
    @if (highlight(); as h) {
      <svg class="room" aria-hidden="true" preserveAspectRatio="none" [attr.viewBox]="'0 0 ' + columns() + ' ' + rows()">
        <rect [attr.x]="h.x" [attr.y]="h.y" [attr.width]="h.width" [attr.height]="h.height" />
      </svg>
    }
    @if (reach(); as r) {
      @if (r.center) {
        <svg class="reach" aria-hidden="true" preserveAspectRatio="none" [attr.viewBox]="'0 0 ' + columns() + ' ' + rows()">
 @if (r.dim > r.bright) {
            <circle class="reach__halo" [attr.cx]="r.center.x" [attr.cy]="r.center.y" [attr.r]="r.dim" />
            <circle class="reach__dim" [attr.cx]="r.center.x" [attr.cy]="r.center.y" [attr.r]="r.dim" />
          }
          @if (r.bright > 0) {
            <circle class="reach__halo" [attr.cx]="r.center.x" [attr.cy]="r.center.y" [attr.r]="r.bright" />
            <circle class="reach__bright" [attr.cx]="r.center.x" [attr.cy]="r.center.y" [attr.r]="r.bright" />
          }
        </svg>
      }
    }
    <app-map-pins [points]="points()" [columns]="columns()" [rows]="rows()" [isMaster]="true" [faded]="faded()" [selectedId]="selectedId()" />
  `,
  styleUrl: './editor-overlay.scss',
  host: {
    'aria-hidden': 'true',
    '[style.--cols]': 'columns() || 1',
    '[style.--rows]': 'rows() || 1',
  },
})
export class EditorOverlay {
  readonly columns = input(0);
  readonly rows = input(0);
  readonly layers = input.required<MapLayers>();
  readonly points = input<readonly MapPoint[]>([]);
  readonly cursor = input<BrushCursor | null>(null);
  readonly lightReach = input<LightReach | null>(null);
  /** Painting: the marks stand back at 40 %. */
  readonly faded = input(false);
  readonly selectedId = input<string | null>(null);
  /** The calibration (RN-25): how many squares of 1,5 m each square of the drawing is worth. Above 1, the drawing's own
   * lines are drawn solid, every `factor` squares, over the rules' grid, which turns dotted. */
  readonly factor = input(1);
  /** A room chosen in a generated dungeon's list: its floor, in squares, outlined with the solid 3 px accent (docs/design.md). */
  readonly highlight = input<{ x: number; y: number; width: number; height: number } | null>(null);

  protected readonly gridPath = computed(() => {
    const parts: string[] = [];
    for (let c = 0; c <= this.columns(); c++) {
      parts.push(`M${c} 0V${this.rows()}`);
    }
    for (let r = 0; r <= this.rows(); r++) {
      parts.push(`M0 ${r}H${this.columns()}`);
    }
    return parts.join('');
  });

  /** The drawing's lines: every `factor` squares of the rules' grid, edges included. */
  protected readonly drawnPath = computed(() => {
    const f = Math.max(1, this.factor());
    const parts: string[] = [];
    for (let c = 0; c <= this.columns(); c += f) {
      parts.push(`M${c} 0V${this.rows()}`);
    }
    for (let r = 0; r <= this.rows(); r += f) {
      parts.push(`M0 ${r}H${this.columns()}`);
    }
    return parts.join('');
  });

  /** The circles, in squares: the radius is the feet over 5 (a square is 5 ft), from the middle of the light's square. */
  protected readonly reach = computed(() => {
    const light = this.lightReach();
    const cols = this.columns();
    const rows = this.rows();
    if (!light || cols === 0 || rows === 0) {
      return null;
    }
    const square: Square = squareAt(light.xBp / 10000, light.yBp / 10000, cols, rows);
    return {
      center: { x: square.col + 0.5, y: square.row + 0.5 },
      bright: light.brightFt / SQUARE_FT,
      dim: (light.brightFt + light.dimFt) / SQUARE_FT,
    };
  });
}
