import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import type { Map as MapMessage } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { MAX_COLUMNS, MIN_COLUMNS, gridRows } from '../../../core/combat/combat-grid';
import { MapBlockedReason } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { editorErrorMessage, mapBlockedReason } from '../../../core/maps/map-errors';
import { MapsClient } from '../../../core/maps/maps-client';
import { MapAsk } from '../../../shared/map-ask/map-ask';
import { CalibrateAsk } from './calibrate-ask';
import { factorLabel, maxDrawnColumns } from '../../../core/maps/calibration';

const MIN = MIN_COLUMNS;
const MAX = MAX_COLUMNS;
const DEFAULT = 20;

/**
 * "Grade" (E9-01 1, 4, 5, 6): how many squares the map has and the one way to change it. Without a grid it asks
 * for the columns and "Definir a grade" (nothing to paint or light before it exists). With one, "Mudar a grade"
 * opens the question in place: the new columns (the rows follow the image's proportions), what it erases when
 * something is painted or seen (the layers and the players' memory; points and tokens stay), "Voltar" and the one
 * filled button, "Apagar e mudar a grade". With nothing to lose it does not warn and the button just says "Mudar a
 * grade". While a combat runs on the map the change is off, with the reason in a line (the server refuses it too).
 */
@Component({
  selector: 'app-grid-panel',
  imports: [
    CalibrateAsk,
    MapAsk,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './grid-panel.html',
  styleUrl: './grid-panel.scss',
})
export class GridPanel {
  private readonly api = inject(MapsClient);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly map = input.required<MapMessage>();
  /** Something is painted or seen: a new grid would erase it. */
  readonly erases = input(false);
  readonly combatRunning = input(false);

  /** The map as the server has it after the change. */
  readonly changed = output<MapMessage>();
  /** The server refused because of what a combat does (or no longer does): the page's flag is stale. */
  readonly blocked = output<void>();

  protected readonly asking = signal(false);
  /** "Cada quadrado deste desenho vale" is open (RN-25). */
  protected readonly calibrating = signal(false);
  protected readonly typed = signal(String(DEFAULT));
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  private readonly opener = viewChild('opener', { read: ElementRef<HTMLButtonElement> });
  private readonly calibrator = viewChild('calibrator', { read: ElementRef<HTMLButtonElement> });
  private readonly field = viewChild('field', { read: ElementRef<HTMLInputElement> });

  protected readonly hasGrid = computed(() => this.map().gridColumns > 0);
  protected readonly image = computed(() => this.map().image);
  /** How many squares of 1,5 m a square of the drawing is worth (1 for a map never calibrated). */
  protected readonly factor = computed(() => Math.max(1, this.map().squareFactor));
  /** The columns of the DRAWING: what "Mudar a grade" changes (the rules' grid is these times the factor). */
  protected readonly drawnColumns = computed(
    () => this.map().drawnColumns || this.map().gridColumns,
  );
  protected readonly drawnRows = computed(
    () => this.map().drawnRows || Math.round(this.map().gridRows / this.factor()),
  );
  protected readonly calibrated = computed(() => this.factor() > 1);
  protected readonly factorText = computed(() => factorLabel(this.factor()));
  /** The most columns the drawing can have at this factor: the rules' grid stays within 200 columns. */
  protected readonly maxColumns = computed(() => Math.min(MAX, maxDrawnColumns(this.factor())));
  protected readonly columns = computed(() => {
    const t = this.typed().trim();
    const n = /^\d{1,3}$/.test(t) ? Number(t) : NaN;
    return n >= MIN && n <= this.maxColumns() ? n : null;
  });
  protected readonly rows = computed(() => {
    const image = this.image();
    const columns = this.columns();
    return image && columns !== null ? gridRows(columns, image.width, image.height) : null;
  });
  protected readonly invalid = computed(() => this.columns() === null);
  /** The size really changes: asking for the same grid would only erase for nothing. */
  protected readonly changes = computed(() => this.columns() !== this.drawnColumns());
  protected readonly min = MIN;
  protected readonly max = computed(() => this.maxColumns());

  /** Opens the question (a map with a grid), or just the field (without one). */
  protected start(): void {
    this.typed.set(String(this.hasGrid() ? this.drawnColumns() : DEFAULT));
    this.error.set('');
    this.asking.set(true);
  }

  /** Opens "Cada quadrado deste desenho vale". */
  protected calibrate(): void {
    this.asking.set(false);
    this.calibrating.set(true);
  }

  protected closeCalibration(): void {
    this.calibrating.set(false);
    afterNextRender(() => focusWithRing(this.calibrator()?.nativeElement), {
      injector: this.injector,
    });
  }

  protected onCalibrated(map: MapMessage): void {
    this.changed.emit(map);
    this.closeCalibration();
  }

  protected cancel(): void {
    this.asking.set(false);
    this.error.set('');
    afterNextRender(() => focusWithRing(this.opener()?.nativeElement), { injector: this.injector });
  }

  protected onType(event: Event): void {
    this.typed.set((event.target as HTMLInputElement).value);
  }

  protected async confirm(): Promise<void> {
    const columns = this.columns();
    if (columns === null || this.busy() || (this.hasGrid() && !this.changes())) {
      this.field()?.nativeElement.focus();
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      this.changed.emit(
        await this.api.setGrid(this.campaignId(), this.map().id, columns, this.factor()),
      );
      this.asking.set(false);
      afterNextRender(() => focusWithRing(this.opener()?.nativeElement), {
        injector: this.injector,
      });
    } catch (err) {
      this.error.set(editorErrorMessage(err, 'grid', 'mudar a grade'));
      if (mapBlockedReason(err) === MapBlockedReason.COMBAT_RUNNING) {
        this.blocked.emit();
      }
    } finally {
      this.busy.set(false);
    }
  }
}
