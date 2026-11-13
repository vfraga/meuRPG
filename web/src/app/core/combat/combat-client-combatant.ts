import {
  type CombatEffect,
  type Encounter,
  SunlightScope,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { type CombatClientBase } from './combat-client-base';
import {
  type AttackDie,
  type DeathSaveResult,
  type ConditionChange,
  type HpAdjust,
} from './combat-client-types';
import { need } from './combat-client-wire';

/** The calls that change one combatant: death saves, conditions, effects and hit points. */
export class CombatCombatantCalls {
  constructor(private readonly base: CombatClientBase) {}

  async rollDeathSave(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    die: AttackDie,
    key: string,
  ): Promise<DeathSaveResult> {
    const res = await this.base.client.rollDeathSave({
      campaignId,
      encounterId,
      combatantId,
      idempotencyKey: key,
      roll:
        'inApp' in die ? { case: 'rollInApp', value: true } : { case: 'd20Face', value: die.face },
    });
    return {
      encounter: need(res.encounter, 'RollDeathSave'),
      save: res.deathSave,
      held: res.heldRollOffer,
    };
  }

  /** The master spends the legendary actions of an option of kind "text" (`SpendLegendaryAction`). */
  async spendLegendaryAction(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    optionKey: string,
    key?: string,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['spendLegendaryAction', campaignId, encounterId, combatantId, optionKey],
      (sent) =>
        this.base.client.spendLegendaryAction({
          campaignId,
          encounterId,
          combatantId,
          optionKey,
          idempotencyKey: sent,
        }),
      key,
    );
    return need(res.encounter, 'SpendLegendaryAction');
  }

  /**
   * The master's "Aparar: perguntar a cada golpe" (`SetMonsterReactionMode`): `ask` true makes each melee hit on an NPC
   * with a Parry a window the master answers; false lets the server decide it. `key` is the caller's, kept across the
   * retries of one tap (a lost answer must not flip the switch twice).
   */
  async setMonsterReactionMode(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    ask: boolean,
    key?: string,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['setMonsterReactionMode', campaignId, encounterId, combatantId, ask],
      (sent) =>
        this.base.client.setMonsterReactionMode({
          campaignId,
          encounterId,
          combatantId,
          ask,
          idempotencyKey: sent,
        }),
      key,
    );
    return need(res.encounter, 'SetMonsterReactionMode');
  }

  /**
   * The master's sunlight switch (`SetSunlight`): `on` puts the combat in the sun or takes it out. `scope` unspecified keeps the
   * current one; `addSquares` and `removeSquares` change the sunlit squares. `key` is the caller's, kept across the retries of one
   * tap (a lost answer must not flip the switch twice).
   */
  async setSunlight(
    campaignId: string,
    encounterId: string,
    change: {
      on: boolean;
      scope?: SunlightScope;
      addSquares?: readonly { col: number; row: number }[];
      removeSquares?: readonly { col: number; row: number }[];
    },
    key?: string,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['setSunlight', campaignId, encounterId, change],
      (sent) =>
        this.base.client.setSunlight({
          campaignId,
          encounterId,
          on: change.on,
          scope: change.scope ?? SunlightScope.UNSPECIFIED,
          addSquares: [...(change.addSquares ?? [])],
          removeSquares: [...(change.removeSquares ?? [])],
          idempotencyKey: sent,
        }),
      key,
    );
    return need(res.encounter, 'SetSunlight');
  }

  /** The master confirms a death (three failed death saves). It cannot be undone. */
  async confirmDeath(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    key: string,
  ): Promise<Encounter> {
    const res = await this.base.client.confirmDeath({
      campaignId,
      encounterId,
      combatantId,
      idempotencyKey: key,
    });
    return need(res.encounter, 'ConfirmDeath');
  }

  /** "Condições…": the labels (the master) and ending the concentration (the
   * master, or a player for their own character). */
  async setConditions(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    change: ConditionChange,
    key?: string,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['setCombatantConditions', campaignId, encounterId, combatantId, change],
      (sent) =>
        this.base.client.setCombatantConditions({
          campaignId,
          encounterId,
          combatantId,
          idempotencyKey: sent,
          conditions: change.keys ? { keys: [...change.keys] } : undefined,
          endConcentration: change.endConcentration ?? false,
        }),
      key,
    );
    return need(res.encounter, 'SetCombatantConditions');
  }

  /** The master's "Encerrar Ajuda": ends a lasting effect on a combatant (only Ajuda has an action; the Escudo Arcano ends by itself). */
  async endCombatEffect(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    effect: CombatEffect,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['endCombatEffect', campaignId, encounterId, combatantId, effect],
      (sent) =>
        this.base.client.endCombatEffect({
          campaignId,
          encounterId,
          combatantId,
          effect,
          idempotencyKey: sent,
        }),
    );
    return need(res.encounter, 'EndCombatEffect');
  }

  /** The master's "Dano/Cura" on an NPC. */
  async adjustHitPoints(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    adjust: HpAdjust,
    key: string,
  ): Promise<Encounter> {
    const c = adjust.change;
    const res = await this.base.client.adjustCombatantHitPoints({
      campaignId,
      encounterId,
      combatantId,
      idempotencyKey: key,
      change: !c
        ? { case: undefined }
        : c.kind === 'damage'
          ? { case: 'damage', value: c.value }
          : c.kind === 'heal'
            ? { case: 'heal', value: c.value }
            : { case: 'hitPoints', value: c.value },
      hitPointsTemporary: adjust.temporary,
    });
    return need(res.encounter, 'AdjustCombatantHitPoints');
  }
}
