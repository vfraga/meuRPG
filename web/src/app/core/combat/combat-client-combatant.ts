import { type CombatEffect, type Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
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
      save: need(res.deathSave, 'RollDeathSave'),
    };
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
