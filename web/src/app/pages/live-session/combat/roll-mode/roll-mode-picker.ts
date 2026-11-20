import { ChangeDetectionStrategy, Component, computed, input, model, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  type AdvantageSource,
  type RollModeRequest,
  RollMode,
  RollModeRequestStatus,
} from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import {
  type Approval,
  ROLL_MODES,
  isBetter,
  modeStatus,
  modeWord,
  orNormal,
} from '../../../../core/combat/roll-mode';

import { RollModeSources } from './roll-mode-sources';

let nextId = 0;

/**
 * How the d20 is rolled: what the server suggests, the circumstances behind it
 * (each tagged "Vantagem" or "Desvantagem" in words) and the three modes as a
 * radio group, the suggestion marked. Any change from the suggestion asks for its
 * reason. A player may pick Desvantagem freely; a mode better than the suggestion
 * is asked of the master ("Pedir ao mestre", then "Aguardando o mestre…" with a
 * cancel, then the master's decision), and the master picks any. The page sends
 * the request and rolls; this only draws the state and holds the choice.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-roll-mode-picker',
  imports: [MatButtonModule, MatIconModule, RollModeSources],
  templateUrl: './roll-mode-picker.html',
  styleUrl: './roll-mode-picker.scss',
})
export class RollModePicker {
  readonly suggested = input<RollMode>(RollMode.NORMAL);
  readonly sources = input<readonly AdvantageSource[]>([]);
  /** The master sets any mode; a player asks him for a better one, or cannot (a spell's attack). */
  readonly approval = input<Approval>('request');
  /** The request of this attack, as the combat shows it now. */
  readonly request = input<RollModeRequest | null>(null);
  /** The chosen mode, two-way. */
  readonly mode = model<RollMode>(RollMode.NORMAL);
  readonly reason = model('');
  readonly busy = input(false);
  /** Nothing is preselected: the targets of a cast suggest different modes. */
  readonly mixed = input(false);
  /** The Inspiração is ticked: the segment only reads `mode` (every cell dashed, none can be picked), says `word` beside the title
   * ("pela Inspiração") and `line` under it, how to choose again. No reason is asked and no request is made while it stands. */
  readonly lock = input<{
    readonly mode: RollMode;
    readonly word: string;
    readonly line: string;
  } | null>(null);

  readonly ask = output<void>();
  readonly cancelAsk = output<void>();

  protected readonly id = `roll-mode-${nextId++}`;
  protected readonly modes = ROLL_MODES;
  protected readonly status = computed(() =>
    modeStatus(this.approval(), this.suggested(), this.mode(), this.reason(), this.request()),
  );
  protected readonly pending = computed(
    () => this.request()?.status === RollModeRequestStatus.PENDING,
  );
  protected readonly answered = computed(() => {
    const r = this.request();
    return r?.status === RollModeRequestStatus.ANSWERED ? r : null;
  });
  protected readonly changed = computed(
    () => !this.mixed() && orNormal(this.mode()) !== orNormal(this.suggested()),
  );
  /** The reason field: any change from the suggestion, until the master has answered. */
  protected readonly needsReason = computed(
    () => this.changed() && !this.answered() && this.status() !== 'blocked',
  );
  protected readonly reasonLength = computed(() => this.reason().trim().length);

  protected readonly Mode = RollMode;
  protected readonly suggestedMode = computed(() => orNormal(this.suggested()));
  protected readonly better = computed(
    () => !this.mixed() && isBetter(this.mode(), this.suggested()),
  );

  protected word = modeWord;

  protected pick(mode: RollMode): void {
    if (!this.pending() && !this.answered() && !this.lock()) {
      this.mode.set(mode);
    }
  }

  protected isOff(mode: RollMode): boolean {
    return (
      !!this.lock() || this.pending() || !!this.answered() || (this.busy() && mode !== this.mode())
    );
  }

  protected onReason(event: Event): void {
    this.reason.set((event.target as HTMLInputElement).value);
  }

  protected checked(mode: RollMode): boolean {
    const lock = this.lock();
    if (lock) {
      return orNormal(lock.mode) === mode;
    }
    return this.answered()
      ? this.answered()!.decidedMode === mode
      : this.mode() === mode && !this.mixed();
  }
}
