import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  EffectDurationKind,
  type LastingEffect,
} from '../../../../gen/meurpg/play/v1/lasting_effects_pb';
import type { CombatState } from '../../../core/combat/combat-state';
import { ActionKey } from '../../../core/connect/idempotency';
import { EffectsClient } from '../../../core/effects/effects-client';
import { effectsErrorMessage } from '../../../core/effects/effects-errors';
import {
  type DurationMode,
  MAX_ROUNDS,
  durationSpec,
  modeOfKind,
  validRounds,
} from '../../../core/effects/effects-text';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../shared/sheet/sheet-host';

/** What the page hands "Mudar a duração". */
export interface DurationData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly state: CombatState;
  readonly effect: LastingEffect;
}

/**
 * "Mudar a duração de Imobilizar Pessoa em Goblin 2" (W7-E board 4b): three choices in radios, "Mais rodadas" (a number
 * from 1 to 600, counted from now), "Até o fim do turno de alguém" and "Até o mestre encerrar". The first line of the
 * dialog says how the effect ends today, in the server's words.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-duration-dialog',
  imports: [MatButtonModule, MatIconModule, SheetFrame, SelectField, TextField],
  templateUrl: './duration-dialog.html',
  styleUrls: ['./effects-sheet.scss'],
})
export class DurationDialog {
  private readonly api = inject(EffectsClient);
  private readonly sheet = injectSheet<DurationData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly effectNow = this.data.effect;

  protected readonly title = `Mudar a duração de ${this.effectNow.sourceNamePt} em ${this.effectNow.targetLabels.join(', ')}`;
  protected readonly mode = signal<DurationMode>(this.start());
  protected readonly rounds = signal(String(this.effectNow.roundsLeft ?? 1));
  protected readonly anchorId = signal(this.effectNow.targetIds[0] ?? '');
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  private readonly key = new ActionKey();

  protected readonly maxRounds = MAX_ROUNDS;
  protected readonly roundsNumber = computed(() => Number(this.rounds().trim() || NaN));
  protected readonly roundsIssues = computed(() =>
    this.mode() === 'rounds' && !validRounds(this.roundsNumber())
      ? [`Digite de 1 a ${MAX_ROUNDS} rodadas.`]
      : [],
  );
  protected readonly anchorOptions = computed<SelectOption[]>(() =>
    (this.data.state.encounter()?.combatants ?? []).map((c) => ({ value: c.id, label: c.label })),
  );
  protected readonly valid = computed(
    () => this.roundsIssues().length === 0 && (this.mode() !== 'turn' || !!this.anchorId()),
  );
  /** "Hoje resta 9. Acaba no turno de Orla, rodada 12." */
  protected readonly today = computed(() => {
    const e = this.effectNow;
    if (e.roundsLeft !== undefined && e.durationKind === EffectDurationKind.ROUNDS) {
      return `Hoje resta ${e.roundsLeft}. ${e.endTextPt.replace(/^[^:]*:\s*/, '').replace(/^./, (c) => c.toUpperCase())}`;
    }
    return `Hoje: ${e.endTextPt || e.clockTextPt}`;
  });

  private start(): DurationMode {
    const kind = this.effectNow.durationKind;
    return kind === EffectDurationKind.ROUNDS || kind === EffectDurationKind.CONCENTRATION
      ? 'rounds'
      : modeOfKind(kind);
  }

  protected async save(): Promise<void> {
    if (this.busy() || !this.valid()) {
      return;
    }
    const spec = durationSpec(this.mode(), this.roundsNumber(), this.anchorId(), true);
    this.busy.set(true);
    this.error.set('');
    try {
      const encounter = await this.api.changeDuration(
        this.data.campaignId,
        this.data.encounterId,
        this.effectNow.id,
        spec,
        this.key.keyFor([this.effectNow.id, spec]),
      );
      this.key.renew();
      if (encounter) {
        this.data.state.apply(encounter);
      }
      this.sheet.close(true);
    } catch (err) {
      this.error.set(effectsErrorMessage(err, 'mudar a duração', 'combat'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (!this.busy()) {
      this.sheet.close(false);
    }
  }
}
