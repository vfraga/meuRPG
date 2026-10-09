import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatSelectModule } from '@angular/material/select';

import { type Encounter, CombatantState } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type {
  LegendaryOffer,
  LegendaryResistancePrompt,
} from '../../../../../gen/meurpg/play/v1/creature_types_pb';
import { Ability } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { CreatureClient, newKey } from '../../../../core/combat/creature-client';
import { abilityPt, doesNotFit, legendaryButton } from '../../../../core/combat/creature-turn';

/**
 * What a legendary creature offers the master (W7-M, SRD 5.1, "Legendary Creatures"): at the end of
 * another creature's turn, a legendary action, one for each offer ("Uma por oferta"), with the
 * ones that do not fit in what is left grey and saying why; and, when the creature fails a saving
 * throw, the choice to use Legendary Resistance, which lives as long as the damage of the failure
 * is not rolled ("Deixar falhar" is the default). Both only the master gets, in the combat it
 * receives (`legendaryOffers`, `legendaryResistancePrompts`).
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-legendary-offers',
  imports: [MatButtonModule, MatFormFieldModule, MatIconModule, MatSelectModule],
  templateUrl: './legendary-offers.html',
  styleUrl: './legendary-offers.scss',
})
export class LegendaryOffers {
  private readonly api = inject(CreatureClient);

  readonly campaignId = input.required<string>();
  readonly encounter = input.required<Encounter>();
  readonly state = input.required<CombatState>();

  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly targetIds = signal<readonly string[]>([]);
  private key = newKey();

  protected readonly offers = computed(() =>
    this.encounter().legendaryOffers.map((o) => ({
      offer: o,
      who: this.label(o.combatantId),
      after: this.label(o.afterCombatantId),
      options: o.options.map((opt) => ({
        key: opt.key,
        name: opt.namePt || opt.name,
        text: opt.text,
        cost: opt.cost,
        available: opt.available,
        button: legendaryButton(opt.cost),
        why: opt.available ? '' : doesNotFit(o.left),
      })),
    })),
  );
  protected readonly prompts = computed(() =>
    this.encounter().legendaryResistancePrompts.map((p) => ({
      prompt: p,
      who: this.label(p.combatantId),
      line: this.promptLine(p),
    })),
  );
  protected readonly targets = computed(() =>
    this.encounter().combatants.filter((c) => c.state !== CombatantState.DEFEATED),
  );

  private label(id: string): string {
    return this.encounter().combatants.find((c) => c.id === id)?.label ?? '';
  }

  private promptLine(p: LegendaryResistancePrompt): string {
    const spell = p.spellNamePt || p.spellKey;
    const bonus = `${p.bonus < 0 ? '−' : '+'} ${Math.abs(p.bonus)}`;
    return `falhou no teste de resistência de ${abilityPt(p.ability as Ability)} contra ${spell} (CD ${p.dc}): 1d20 (${p.d20}) ${bonus} = ${p.total}.`;
  }

  protected pickTargets(ids: string[]): void {
    this.targetIds.set(ids);
    this.key = newKey();
  }

  protected async use(o: LegendaryOffer, optionKey: string): Promise<void> {
    await this.run(async () => {
      const res = await this.api.useLegendary(
        this.campaignId(),
        this.encounter().id,
        o.combatantId,
        optionKey,
        this.targetIds(),
        this.key,
      );
      this.state().apply(res.encounter);
    });
  }

  protected async pass(o: LegendaryOffer): Promise<void> {
    await this.run(async () => {
      this.state().apply(
        await this.api.declineLegendary(
          this.campaignId(),
          this.encounter().id,
          o.combatantId,
          this.key,
        ),
      );
    });
  }

  protected async answer(p: LegendaryResistancePrompt, use: boolean): Promise<void> {
    await this.run(async () => {
      this.state().apply(
        await this.api.answerResistance(
          this.campaignId(),
          this.encounter().id,
          p.combatantId,
          p.castId,
          use,
          this.key,
        ),
      );
    });
  }

  private async run(call: () => Promise<void>): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      await call();
      this.key = newKey();
      this.targetIds.set([]);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'usar a ação lendária'));
    } finally {
      this.busy.set(false);
    }
  }
}
