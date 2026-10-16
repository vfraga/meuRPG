import {
  type Encounter,
  type GetCombatHighlightsResponse,
  type ListCombatLogResponse,
  EncounterMode,
  HitRiderChoice,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  type JoinSpec,
  type MonsterHp,
  type StartExtras,
  type AddMonstersResult,
  type InitiativeRoll,
  type DamageDie,
  type ActionResult,
} from './combat-client-types';
import { toHitPoints, toParticipant, need } from './combat-client-wire';
import { type CombatClientBase } from './combat-client-base';

/** The calls that set a combat up, pass its turns and end it, the action menu, undo, the log and the highlights. */
export class CombatTurnCalls {
  constructor(private readonly base: CombatClientBase) {}

  /** The latest combat of the open session, or `null` while it had none. */
  async get(campaignId: string): Promise<Encounter | null> {
    return (await this.base.client.getEncounter({ campaignId })).encounter ?? null;
  }

  async start(
    campaignId: string,
    name: string,
    participants: readonly JoinSpec[],
    idempotencyKey: string,
    extras: StartExtras = {},
  ): Promise<Encounter> {
    const theatre = extras.mode === EncounterMode.THEATRE;
    const res = await this.base.client.startEncounter({
      campaignId,
      idempotencyKey,
      name,
      participants: participants.map(toParticipant),
      mapPointId: theatre ? '' : (extras.mapPointId ?? ''),
      // Left out, the server reads the table's "combate com mapa" rule (RN-24).
      ...(extras.mode === undefined ? {} : { mode: extras.mode }),
      ...(extras.monsters && extras.monsters.length > 0
        ? {
            monsters: extras.monsters.map((m) => ({
              creatureKey: m.creatureKey,
              count: m.count,
              name: m.name ?? '',
            })),
            monsterHitPoints: toHitPoints(extras.monsterHp),
            monstersHidden: extras.monstersHidden ?? true,
          }
        : {}),
    });
    return need(res.encounter, 'StartEncounter');
  }

  /** "Pôr no combate" (`AddMonsters`): 1 to 10 monsters of one SRD creature. The caller makes the key once per add and sends it
   * again on a retry with the same parameters. */
  async addMonsters(
    campaignId: string,
    encounterId: string,
    add: {
      readonly creatureKey: string;
      readonly count: number;
      readonly name: string;
      readonly hp: MonsterHp;
      readonly hidden: boolean;
    },
    idempotencyKey: string,
  ): Promise<AddMonstersResult> {
    const res = await this.base.client.addMonsters({
      campaignId,
      encounterId,
      idempotencyKey,
      creatureKey: add.creatureKey,
      count: add.count,
      name: add.name,
      hitPoints: toHitPoints(add.hp),
      hidden: add.hidden,
    });
    return { encounter: need(res.encounter, 'AddMonsters'), combatantIds: res.combatantIds };
  }

  async submitInitiative(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    roll: InitiativeRoll,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['submitInitiative', campaignId, encounterId, combatantId, roll],
      (sent) =>
        this.base.client.submitInitiative({
          campaignId,
          encounterId,
          combatantId,
          idempotencyKey: sent,
          roll:
            'inApp' in roll
              ? { case: 'rollInApp', value: true }
              : { case: 'd20Face', value: roll.face },
        }),
    );
    return need(res.encounter, 'SubmitInitiative');
  }

  async setOrder(
    campaignId: string,
    encounterId: string,
    combatantIds: readonly string[],
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['setInitiativeOrder', campaignId, encounterId, combatantIds],
      (sent) =>
        this.base.client.setInitiativeOrder({
          campaignId,
          encounterId,
          idempotencyKey: sent,
          combatantIds: [...combatantIds],
        }),
    );
    return need(res.encounter, 'SetInitiativeOrder');
  }

  async begin(campaignId: string, encounterId: string): Promise<Encounter> {
    const res = await this.base.keyed(['beginCombat', campaignId, encounterId], (sent) =>
      this.base.client.beginCombat({
        campaignId,
        encounterId,
        idempotencyKey: sent,
      }),
    );
    return need(res.encounter, 'BeginCombat');
  }

  /** `expectedCombatantId` is whose turn the screen thinks it is: a tap from a screen that has not caught up with the
   * turn is refused (`aborted`), never skipping another one; the same tap sent again keeps its key, so the server
   * answers with the first. Empty only when nobody is on turn. */
  async endTurn(
    campaignId: string,
    encounterId: string,
    expectedCombatantId: string,
    discardPendingDamage = false,
    expectedRound = 0,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      [
        'endTurn',
        campaignId,
        encounterId,
        expectedCombatantId,
        discardPendingDamage,
        expectedRound,
      ],
      (sent) =>
        this.base.client.endTurn({
          campaignId,
          encounterId,
          idempotencyKey: sent,
          expectedCombatantId,
          discardPendingDamage,
          expectedRound,
        }),
    );
    return need(res.encounter, 'EndTurn');
  }

  /** "A sua fúria vai acabar?": `endRage` true lets the rage end and passes the turn, false goes back to the turn. */
  async answerRageEnd(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    endRage: boolean,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['answerRageEnd', campaignId, encounterId, combatantId, endRage],
      (sent) =>
        this.base.client.answerRageEnd({
          campaignId,
          encounterId,
          combatantId,
          endRage,
          idempotencyKey: sent,
        }),
    );
    return need(res.encounter, 'AnswerRageEnd');
  }

  /** `UseHitRider`: the monk's answer to an offer after a hit (Open Hand technique, Golpe Atordoante). */
  async useHitRider(
    campaignId: string,
    encounterId: string,
    riderId: string,
    choice: HitRiderChoice,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['useHitRider', campaignId, encounterId, riderId, choice],
      (sent) =>
        this.base.client.useHitRider({
          campaignId,
          encounterId,
          riderId,
          choice,
          idempotencyKey: sent,
        }),
    );
    return need(res.encounter, 'UseHitRider');
  }

  /** "Encerrar fúria": the bonus action that ends a rage. */
  async endRage(campaignId: string, encounterId: string, combatantId: string): Promise<Encounter> {
    const res = await this.base.keyed(['endRage', campaignId, encounterId, combatantId], (sent) =>
      this.base.client.endRage({ campaignId, encounterId, combatantId, idempotencyKey: sent }),
    );
    return need(res.encounter, 'EndRage');
  }

  /** A standard action ("standard:dash") or a feature's ("feature:second-wind").
   * `die` is for the one that rolls (Retomar o Fôlego: the d10); `key` is made
   * once per action, so a retry after a lost answer never spends it twice. */
  async takeAction(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    actionKey: string,
    die?: DamageDie,
    key?: string,
    useExtraAction = false,
    frenzy = false,
  ): Promise<ActionResult> {
    const res = await this.base.keyed(
      ['takeAction', campaignId, encounterId, combatantId, actionKey, die, useExtraAction, frenzy],
      (sent) =>
        this.base.client.takeAction({
          campaignId,
          encounterId,
          combatantId,
          actionKey,
          idempotencyKey: sent,
          // The extra action an effect gives (Velocidade) pays for it instead of the action (RN-22).
          useExtraAction,
          // The Berserker's Fúria in a frenzy (SRD 5.1, Frenzy); ignored by every other action.
          frenzy,
          roll: !die
            ? { case: undefined }
            : 'inApp' in die
              ? { case: 'rollInApp', value: true }
              : { case: 'typedSum', value: die.sum },
        }),
      key,
    );
    return { encounter: need(res.encounter, 'TakeAction'), roll: res.roll, healed: res.healed };
  }

  /** "Desfazer última ação": `expectedEventId` is the log's undoable event. */
  async undo(campaignId: string, encounterId: string, expectedEventId: string): Promise<Encounter> {
    const res = await this.base.keyed(
      ['undoLastAction', campaignId, encounterId, expectedEventId],
      (sent) =>
        this.base.client.undoLastAction({
          campaignId,
          encounterId,
          expectedEventId,
          idempotencyKey: sent,
        }),
    );
    return need(res.encounter, 'UndoLastAction');
  }

  /** The combat log, latest first, as the caller may see it. */
  log(campaignId: string, encounterId: string): Promise<ListCombatLogResponse> {
    return this.base.client.listCombatLog({ campaignId, encounterId });
  }

  /** "Destaques do combate" (MR-032): who did the most in a combat that ended.
   * The master's answer also has the table of every player's numbers. */
  highlights(campaignId: string, encounterId: string): Promise<GetCombatHighlightsResponse> {
    return this.base.client.getCombatHighlights({ campaignId, encounterId });
  }

  async end(campaignId: string, encounterId: string): Promise<Encounter> {
    const res = await this.base.keyed(['endEncounter', campaignId, encounterId], (sent) =>
      this.base.client.endEncounter({
        campaignId,
        encounterId,
        idempotencyKey: sent,
      }),
    );
    return need(res.encounter, 'EndEncounter');
  }
}
