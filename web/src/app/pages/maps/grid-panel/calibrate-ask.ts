import {
  ChangeDetectionStrategy,
  Component,
  OnInit,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import { MapBlockedReason, type Map as MapMessage } from '../../../../gen/meurpg/maps/v1/maps_pb';
import type { DiceOption } from '../../../core/campaigns/dice-labels';
import {
  MAX_FACTOR,
  PRESET_FACTORS,
  effectOf,
  factorFromMeters,
  factorLabel,
  fits,
  metersField,
  rulesGrid,
} from '../../../core/maps/calibration';
import { editorErrorMessage, mapBlockedReason } from '../../../core/maps/map-errors';
import { MapsClient } from '../../../core/maps/maps-client';
import { DiceChoice } from '../../../shared/dice-choice/dice-choice';
import { MapAsk } from '../../../shared/map-ask/map-ask';

/** The radio value of "Outro". */
const OTHER = 0;

/**
 * "Cada quadrado deste desenho vale" (MR-025, RN-25; E10-03 state 5): the calibration of a map's grid, as an in-place
 * question of the "Grade" panel. The master says how many squares of 1,5 m each square of the drawing is worth (1,5 m,
 * 3 m, 4,5 m, 6 m, or another multiple of 1,5 m up to 30 m), and the question says what the rules' grid becomes. A larger
 * multiple of the old value keeps what was painted, scaled, so it is saved at once; any other change clears the layers
 * and what the players remember (`SetMapGrid`'s docs), and then, with something painted or seen, a second question,
 * "Mudar a grade?", asks first. The server refuses it while a combat runs on the map and says so by the typed reason.
 */
@Component({
  selector: 'app-calibrate-ask',
  imports: [DiceChoice, MapAsk, MatFormFieldModule, MatIconModule, MatInputModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './calibrate-ask.html',
  styleUrl: './calibrate-ask.scss',
})
export class CalibrateAsk implements OnInit {
  private readonly api = inject(MapsClient);

  readonly campaignId = input.required<string>();
  readonly map = input.required<MapMessage>();
  /** Something is painted or seen: clearing it needs the second question. */
  readonly erases = input(false);

  readonly changed = output<MapMessage>();
  readonly blocked = output<void>();
  readonly closed = output<void>();

  protected readonly oldFactor = computed(() => Math.max(1, this.map().squareFactor));
  protected readonly drawnColumns = computed(
    () => this.map().drawnColumns || this.map().gridColumns / this.oldFactor(),
  );
  protected readonly drawnRows = computed(
    () => this.map().drawnRows || this.map().gridRows / this.oldFactor(),
  );

  /** The choice starts on what the map has now: one of the four, or "Outro" with its value in the field. */
  protected readonly pick = signal<number>(OTHER);
  protected readonly other = signal('');
  protected readonly stage = signal<'pick' | 'confirm'>('pick');
  protected readonly busy = signal(false);
  protected readonly error = signal('');

  protected readonly options = computed<readonly DiceOption<number>[]>(() => [
    ...PRESET_FACTORS.map((f) => ({
      value: f,
      title: factorLabel(f),
      description:
        f === 1
          ? 'O desenho já está na escala das regras (5 pés).'
          : `Cada quadrado do desenho vira ${f} × ${f} quadrados de 1,5 m.`,
    })),
    {
      value: OTHER,
      title: 'Outro',
      description: `Um múltiplo de 1,5 m, até ${factorLabel(MAX_FACTOR)}.`,
    },
  ]);

  /** The factor the question stands on now, or `null` while "Outro" holds something that is not one. */
  protected readonly factor = computed<number | null>(() =>
    this.pick() === OTHER ? factorFromMeters(this.other()) : this.pick(),
  );
  protected readonly effect = computed(() => {
    const f = this.factor();
    return f === null ? null : effectOf(this.oldFactor(), f);
  });
  protected readonly grid = computed(() => {
    const f = this.factor();
    return f === null ? null : rulesGrid(this.drawnColumns(), this.drawnRows(), f);
  });
  protected readonly tooBig = computed(() => {
    const f = this.factor();
    return f !== null && !fits(this.drawnColumns(), this.drawnRows(), f);
  });
  protected readonly ready = computed(
    () => this.factor() !== null && this.effect() !== 'same' && !this.tooBig(),
  );
  protected readonly clears = computed(() => this.effect() === 'clears');
  protected readonly times = computed(() => {
    const f = this.factor();
    return f === null ? 0 : f / this.oldFactor();
  });
  /** The picture of the question: one square of the drawing, and the squares of 1,5 m it becomes (up to 6 × 6; larger is only said). */
  protected readonly cells = computed(() => {
    const f = this.factor();
    return f !== null && f >= 2 && f <= 6 ? Array.from({ length: f * f }) : [];
  });
  protected readonly currentLabel = computed(() => factorLabel(this.oldFactor()));
  protected readonly newLabel = computed(() => {
    const f = this.factor();
    return f === null ? '' : factorLabel(f);
  });

  ngOnInit(): void {
    const now = this.oldFactor();
    if (PRESET_FACTORS.includes(now)) {
      this.pick.set(now);
    } else {
      this.pick.set(OTHER);
      this.other.set(metersField(now));
    }
  }

  protected choose(value: number): void {
    this.pick.set(value);
    this.error.set('');
    if (value === OTHER && this.other() === '') {
      // "Outro" starts at a value the four cards do not already offer (5 × 1,5 m = 7,5 m, or the next one if that is the map's own).
      const first = PRESET_FACTORS[PRESET_FACTORS.length - 1] + 1;
      this.other.set(
        metersField(Math.min(MAX_FACTOR, this.oldFactor() === first ? first + 1 : first)),
      );
    }
  }

  protected onType(event: Event): void {
    this.other.set((event.target as HTMLInputElement).value);
    this.error.set('');
  }

  /** "Salvar a grade": a change that clears what was painted asks first; one that scales it, or has nothing to clear, saves. */
  protected async save(): Promise<void> {
    if (!this.ready() || this.busy()) {
      return;
    }
    if (this.clears() && this.erases() && this.stage() === 'pick') {
      this.stage.set('confirm');
      return;
    }
    await this.send();
  }

  protected back(): void {
    this.error.set('');
    if (this.stage() === 'confirm') {
      this.stage.set('pick');
    } else {
      this.closed.emit();
    }
  }

  private async send(): Promise<void> {
    const factor = this.factor();
    if (factor === null) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const map = this.map();
      this.changed.emit(
        await this.api.setGrid(this.campaignId(), map.id, this.drawnColumns(), factor),
      );
    } catch (err) {
      this.stage.set('pick');
      this.error.set(editorErrorMessage(err, 'grid', 'mudar a grade'));
      if (mapBlockedReason(err) === MapBlockedReason.COMBAT_RUNNING) {
        this.blocked.emit();
      }
    } finally {
      this.busy.set(false);
    }
  }
}
