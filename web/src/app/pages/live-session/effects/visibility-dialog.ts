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
  EffectAudience,
  type LastingEffect,
} from '../../../../gen/meurpg/play/v1/lasting_effects_pb';
import type { CombatState } from '../../../core/combat/combat-state';
import { ActionKey } from '../../../core/connect/idempotency';
import { EffectsClient } from '../../../core/effects/effects-client';
import { effectsErrorMessage } from '../../../core/effects/effects-errors';
import { MAX_LABEL } from '../../../core/effects/effects-text';
import { HiddenSwitch } from '../../../shared/hidden-switch/hidden-switch';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../shared/sheet/sheet-host';
import { VisibilityFields } from './visibility-fields';

/** What the page hands "O que os jogadores veem". */
export interface VisibilityData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly state: CombatState;
  readonly effect: LastingEffect;
}

/**
 * "O que os jogadores veem de Imobilizar Pessoa em Goblin 2" (W7-E board 4c): the switch "Os jogadores veem este
 * efeito" (off: nothing reveals it, not the wait, not the source of an advantage, not the log), which players
 * ("Todos os jogadores" or "Só o dono do alvo") and the free label of up to 30 characters.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-visibility-dialog',
  imports: [MatButtonModule, MatIconModule, SheetFrame, HiddenSwitch, VisibilityFields],
  templateUrl: './visibility-dialog.html',
  styleUrls: ['./effects-sheet.scss'],
})
export class VisibilityDialog {
  private readonly api = inject(EffectsClient);
  private readonly sheet = injectSheet<VisibilityData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  private readonly effectNow = this.data.effect;

  protected readonly title = `O que os jogadores veem de ${this.effectNow.sourceNamePt} em ${this.effectNow.targetLabels.join(', ')}`;
  protected readonly visible = signal(this.effectNow.playerVisible);
  protected readonly audience = signal(
    this.effectNow.audience === EffectAudience.OWNER ? EffectAudience.OWNER : EffectAudience.ALL,
  );
  protected readonly label = signal(this.effectNow.playerLabel);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  private readonly key = new ActionKey();

  protected readonly changed = computed(
    () =>
      this.visible() !== this.effectNow.playerVisible ||
      (this.visible() &&
        (this.audience() !== this.effectNow.audience ||
          this.label() !== this.effectNow.playerLabel)),
  );

  protected async save(): Promise<void> {
    if (this.busy() || !this.changed()) {
      return;
    }
    const visibility = {
      playerVisible: this.visible(),
      audience: this.audience(),
      playerLabel: this.label().trim().slice(0, MAX_LABEL),
    };
    this.busy.set(true);
    this.error.set('');
    try {
      const encounter = await this.api.setVisibility(
        this.data.campaignId,
        this.data.encounterId,
        this.effectNow.id,
        visibility,
        this.key.keyFor([this.effectNow.id, visibility]),
      );
      this.key.renew();
      if (encounter) {
        this.data.state.apply(encounter);
      }
      this.sheet.close(true);
    } catch (err) {
      this.error.set(effectsErrorMessage(err, 'mudar o que os jogadores veem', 'combat'));
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
