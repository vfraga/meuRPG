import { computed, signal } from '@angular/core';

import { type Combatant, EncounterBlockedReason } from '../../../../gen/meurpg/play/v1/combat_pb';
import type { CombatCalls } from '../../../core/combat/combat-calls';
import { combatErrorMessage, encounterBlocked } from '../../../core/combat/combat-errors';
import type { CombatState } from '../../../core/combat/combat-state';
import { ActionKey } from '../../../core/connect/idempotency';
import type { CreaturesClient } from '../../../core/creatures/creatures-client';
import type { VitalsVm } from '../../../core/play/live-session.types';
import { stayView } from './stay-form';

/** What `CombatForm` reads from the screen it serves. */
export interface CombatFormContext {
  readonly creaturesApi: Pick<CreaturesClient, 'assumeWildShape' | 'leaveWildShape'>;
  readonly calls: Pick<CombatCalls, 'busy' | 'error' | 'refreshAfter'>;
  readonly campaignId: () => string;
  readonly own: () => Combatant | null;
  readonly ownVitals: () => VitalsVm | null | undefined;
  readonly state: () => CombatState;
}

/** The druid's two calls on a form in a combat: "Permanecer na forma" and "Voltar à forma normal", each under its own key. */
export class CombatForm {
  private readonly leaveKey = new ActionKey();
  private readonly stayKey = new ActionKey();
  /** "A forma já acabou" came back for "Permanecer": the row says so until the form changes (nothing was spent). */
  readonly stayEnded = signal(false);
  /** "Permanecer na forma" (the Druida tab, after "Voltar"): what it costs and why it cannot be pressed. `null` outside a form. */
  readonly stay = computed(() => stayView(this.ctx.own(), this.ctx.ownVitals(), this.stayEnded()));

  constructor(private readonly ctx: CombatFormContext) {}

  /** "Permanecer na forma": the same beast, one use, no action; the time starts over. */
  async stayForm(): Promise<void> {
    const own = this.ctx.own();
    if (!own?.wildShapeBeastKey) {
      return;
    }
    const beast = own.wildShapeBeastKey;
    await this.run(
      'permanecer na forma',
      own,
      (key) =>
        this.ctx.creaturesApi.assumeWildShape(
          this.ctx.campaignId(),
          own.characterId,
          beast,
          key,
          true,
        ),
      this.stayKey,
      (err) => {
        if (encounterBlocked(err)?.reason === EncounterBlockedReason.WILD_SHAPE_ENDED) {
          this.stayEnded.set(true);
        }
      },
    );
  }

  /** "Voltar à forma normal": a bonus action. The druid knows it came back: the log's line says "left", and no notice comes of it. */
  async leaveForm(): Promise<void> {
    const own = this.ctx.own();
    if (own) {
      await this.run(
        'voltar à forma normal',
        own,
        (key) => this.ctx.creaturesApi.leaveWildShape(this.ctx.campaignId(), own.characterId, key),
        this.leaveKey,
      );
    }
  }

  /** One call at a time, under its key (kept across a retry), the combat applied or the refusal said and the combat read again. */
  private async run(
    what: string,
    own: Combatant,
    call: (key: string) => Promise<{ encounter?: Parameters<CombatState['apply']>[0] }>,
    key: ActionKey,
    refused?: (err: unknown) => void,
  ): Promise<void> {
    const { busy, error } = this.ctx.calls;
    if (busy()) {
      return;
    }
    busy.set(true);
    error.set('');
    try {
      const res = await call(key.keyFor(own.characterId));
      key.renew();
      if (res.encounter) {
        this.ctx.state().apply(res.encounter);
      }
    } catch (err) {
      refused?.(err);
      error.set(combatErrorMessage(err, what));
      await this.ctx.calls.refreshAfter(err);
    } finally {
      busy.set(false);
    }
  }
}
