import { Component, computed, inject, input, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { Code } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { DICE_PREFERENCE_OPTIONS, effectivePreference } from '../../core/campaigns/dice-labels';
import { describeConnectError } from '../../core/connect/connect-errors';
import { DiceChoice } from '../dice-choice/dice-choice';

/**
 * "Como você rola os dados" (E6-18, RN-18): the player's choice, "No app" or
 * "Meus próprios dados", for one campaign. It is only a choice while the
 * campaign's mode is "Cada jogador escolhe"; when the master decided for
 * everyone, the same cards come locked, with the master's decision as a
 * notice and the unavailable option marked.
 *
 * It sits on the campaign page for now. The design puts it in the live
 * session's menu, next to "Registro do combate": that menu will render this
 * same component (it only needs the campaign's id, name, mode and the
 * viewer's saved preference).
 */
@Component({
  selector: 'app-dice-preference-panel',
  imports: [DiceChoice, MatButtonModule, MatIconModule],
  templateUrl: './dice-preference-panel.html',
  styleUrl: './dice-preference-panel.scss',
})
export class DicePreferencePanel {
  private readonly campaigns = inject(CampaignsService);

  readonly campaignId = input.required<string>();
  readonly campaignName = input.required<string>();
  readonly mode = input.required<DiceMode>();
  /** The preference the server has now (Campaign.my_dice_preference). */
  readonly savedPreference = input.required<DicePreference>();

  protected readonly options = DICE_PREFERENCE_OPTIONS;
  protected readonly DiceMode = DiceMode;

  private readonly picked = signal<DicePreference | null>(null);
  private readonly confirmed = signal<DicePreference | null>(null);
  protected readonly saving = signal(false);
  protected readonly error = signal('');
  protected readonly saved = signal(false);

  protected readonly canChoose = computed(() => this.mode() === DiceMode.PLAYERS_CHOOSE);
  private readonly baseline = computed(() =>
    effectivePreference(DiceMode.PLAYERS_CHOOSE, this.confirmed() ?? this.savedPreference()),
  );
  /** When the master decided, the card in force; otherwise the player's pick. */
  protected readonly selected = computed(() =>
    this.canChoose()
      ? (this.picked() ?? this.baseline())
      : effectivePreference(this.mode(), this.baseline()),
  );
  protected readonly dirty = computed(
    () => this.canChoose() && this.selected() !== this.baseline(),
  );
  protected readonly decision = computed(() =>
    this.mode() === DiceMode.PHYSICAL ? 'todos rolam os próprios dados' : 'todos rolam no app',
  );
  protected readonly notes = computed<Partial<Record<number, string>>>(() => {
    if (this.canChoose()) {
      return {};
    }
    const inForce = this.selected();
    return {
      [DicePreference.APP]:
        inForce === DicePreference.APP
          ? 'Definido pelo mestre para todos.'
          : 'Indisponível nesta campanha.',
      [DicePreference.PHYSICAL]:
        inForce === DicePreference.PHYSICAL
          ? 'Definido pelo mestre para todos.'
          : 'Indisponível nesta campanha.',
    };
  });

  protected pick(preference: DicePreference): void {
    this.picked.set(preference);
    this.saved.set(false);
  }

  protected async save(): Promise<void> {
    this.saving.set(true);
    this.error.set('');
    try {
      const res = await this.campaigns.setDicePreference(this.campaignId(), this.selected());
      this.confirmed.set(res.preference);
      this.picked.set(null);
      this.saved.set(true);
    } catch (err) {
      this.error.set(
        describeConnectError(err, {
          [Code.NotFound]: 'Você não está mais nesta campanha.',
          [Code.Unavailable]: 'Não foi possível salvar agora. Tente de novo em instantes.',
        }),
      );
    } finally {
      this.saving.set(false);
    }
  }
}
