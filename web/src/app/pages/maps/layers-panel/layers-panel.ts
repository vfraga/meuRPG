import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { MapLayers } from '../../../core/maps/layers';
import { type PaintSaveStatus } from '../../../core/maps/paint-queue';
import { LIGHT_LABEL, type PaintTool, layerLines } from '../../../core/maps/paint-tools';
import { DoorMark } from '../../../shared/map-layers/door-mark';
import { RevealSwitch } from '../../../shared/reveal-switch/reveal-switch';

/** Which layers the master's map shows (a view setting of his drawing: it changes nothing for anyone). */
export type LayerVisibility = Readonly<Record<PaintTool, boolean>>;

/**
 * "Camadas" (E9-01 1, and "Portas" of Etapa 10): the five layers the master paints, each with how many squares it holds and a switch that
 * shows or hides it on his own map (his view only), the same drawings as the map's legend, and the "Tudo salvo"
 * tag that says whether the strokes reached the server ("Salvando…", or "Não salvou" with "Tentar de novo").
 * The light's three glyphs are named under its row. Presentational: the editor owns the layers and the queue.
 */
@Component({
  selector: 'app-layers-panel',
  imports: [DoorMark, MatButtonModule, MatIconModule, RevealSwitch],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './layers-panel.html',
  styleUrl: './layers-panel.scss',
})
export class LayersPanel {
  readonly layers = input.required<MapLayers>();
  readonly visible = input.required<LayerVisibility>();
  readonly status = input<PaintSaveStatus>('saved');
  /** Painting is on (a grid, and the layers read): without it there is nothing to save, and no "Tudo salvo". */
  readonly active = input(true);
  /** What the server said when it refused a batch, by the typed reason. */
  readonly problem = input('');
  /** Whether "Tentar de novo" can help (the server did not answer); a refusal is only said. */
  readonly retryable = input(false);

  readonly visibleChange = output<LayerVisibility>();
  readonly retry = output<void>();

  /** What a screen reader hears: "Tudo salvo" once a run of strokes has been saved, never "Salvando" for each batch. */
  protected readonly announced = signal('');
  private wasSaving = false;

  constructor() {
    effect(() => {
      const status = this.status();
      untracked(() => {
        if (status === 'saving') {
          this.wasSaving = true;
        } else if (status === 'saved' && this.wasSaving) {
          this.wasSaving = false;
          this.announced.set('Tudo salvo');
        } else {
          this.announced.set('');
        }
      });
    });
  }

  protected readonly lines = computed(() => layerLines(this.layers()));
  protected readonly lights = [3, 2, 1] as const;
  protected readonly lightLabel = LIGHT_LABEL;
  protected readonly lightIcon = { 3: 'light_mode', 2: 'contrast', 1: 'dark_mode' } as const;

  protected toggle(tool: PaintTool, on: boolean): void {
    this.visibleChange.emit({ ...this.visible(), [tool]: on });
  }
}
