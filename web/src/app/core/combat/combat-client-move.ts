import {
  type CoverDegree,
  type CombatantSide,
  type Encounter,
  type GetMoveOptionsResponse,
  type GetTurnOptionsResponse,
  JumpKind,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { type JoinSpec, type MoveResult, type SpendResult } from './combat-client-types';
import { toParticipant, need } from './combat-client-wire';
import { type CombatClientBase } from './combat-client-base';

/** The calls of movement, sides, cover, opportunity offers and the roster. */
export class CombatMoveCalls {
  constructor(private readonly base: CombatClientBase) {}

  /** `MoveCombatant`: a walk to a square, or a jump (`jump`: a long one to the
   * square, or a high one by `jumpHeightDft` tenths of a foot). `stoppedEarly`
   * is "something you did not see stopped you"; `provoked` that an opportunity
   * offer now waits. The master's drags are plain moves: only `forced` (the
   * "Movimento forçado" box: a teleport, a push or a pull) skips the offers.
   * `key` is the caller's, kept across the retries of one jump (a lost answer
   * must not charge it twice). */
  async move(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    col: number,
    row: number,
    jump?: { readonly kind: 'long' } | { readonly kind: 'high'; readonly heightDft: number },
    key?: string,
    forced = false,
  ): Promise<MoveResult> {
    const res = await this.base.keyed(
      ['moveCombatant', campaignId, encounterId, combatantId, col, row, jump, forced],
      (sent) =>
        this.base.client.moveCombatant({
          campaignId,
          encounterId,
          combatantId,
          idempotencyKey: sent,
          col,
          row,
          jump: jump
            ? jump.kind === 'long'
              ? JumpKind.LONG
              : JumpKind.HIGH
            : JumpKind.UNSPECIFIED,
          jumpHeightDft: jump?.kind === 'high' ? jump.heightDft : 0,
          forced,
        }),
      key,
    );
    return {
      encounter: need(res.encounter, 'MoveCombatant'),
      stoppedEarly: res.stoppedEarly,
      lockedDoor: res.lockedDoor,
      provoked: res.provoked,
    };
  }

  /** "Colocar no mapa": the server picks the square of a combatant that has none
   * (free floor, away from the players), so the screen never guesses one. */
  async place(campaignId: string, encounterId: string, combatantId: string): Promise<Encounter> {
    const res = await this.base.keyed(
      ['placeCombatant', campaignId, encounterId, combatantId],
      (sent) =>
        this.base.client.moveCombatant({
          campaignId,
          encounterId,
          combatantId,
          idempotencyKey: sent,
          place: true,
        }),
    );
    return need(res.encounter, 'MoveCombatant');
  }

  /** `GetMoveOptions`: where the combatant can go in one straight move, the
   * cost of each square and why the others inside the circle are refused. */
  moveOptions(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    jump?: { readonly runningStart: boolean },
  ): Promise<GetMoveOptionsResponse> {
    // With a jump the squares are the long jump's, and `provokes_reactor_ids` warns about its opportunity attacks as a walk's does.
    return this.base.client.getMoveOptions({
      campaignId,
      encounterId,
      combatantId,
      jump: jump ? JumpKind.LONG : JumpKind.UNSPECIFIED,
      jumpRunningStart: jump?.runningStart ?? false,
    });
  }

  /** The master's "Aliado" (PARTY) or back to enemy (ENEMY). */
  async setSide(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    side: CombatantSide,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['setCombatantSide', campaignId, encounterId, combatantId, side],
      (sent) =>
        this.base.client.setCombatantSide({
          campaignId,
          encounterId,
          combatantId,
          idempotencyKey: sent,
          side,
        }),
    );
    return need(res.encounter, 'SetCombatantSide');
  }

  /** The master's "Marcar cobertura": the cover the map does not show. */
  async setCover(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    cover: CoverDegree,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['setCombatantCover', campaignId, encounterId, combatantId, cover],
      (sent) =>
        this.base.client.setCombatantCover({
          campaignId,
          encounterId,
          combatantId,
          idempotencyKey: sent,
          cover,
        }),
    );
    return need(res.encounter, 'SetCombatantCover');
  }

  /** "Não atacar": turns an opportunity offer down. */
  async declineOpportunity(
    campaignId: string,
    encounterId: string,
    offerId: string,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['declineOpportunity', campaignId, encounterId, offerId],
      (sent) =>
        this.base.client.declineOpportunity({
          campaignId,
          encounterId,
          opportunityOfferId: offerId,
          idempotencyKey: sent,
        }),
    );
    return need(res.encounter, 'DeclineOpportunity');
  }

  /** The master's "Seguir sem esperar": passes over an offer nobody answers. */
  async skipOpportunity(
    campaignId: string,
    encounterId: string,
    offerId: string,
    key?: string,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['skipOpportunity', campaignId, encounterId, offerId],
      (sent) =>
        this.base.client.skipOpportunity({
          campaignId,
          encounterId,
          opportunityOfferId: offerId,
          idempotencyKey: sent,
        }),
      key,
    );
    return need(res.encounter, 'SkipOpportunity');
  }

  /** "Gastar movimento" (a combat without a map, RN-25): whole feet, never more than what is left. The server says what is left. */
  async spendMovement(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    distanceFt: number,
    idempotencyKey: string,
  ): Promise<SpendResult> {
    const res = await this.base.client.spendMovement({
      campaignId,
      encounterId,
      combatantId,
      distanceFt,
      idempotencyKey,
    });
    return {
      encounter: need(res.encounter, 'SpendMovement'),
      movementLeftDft: res.movementLeftDft,
    };
  }

  /** "Levantar-se": the combatant stands up from Prone for half its speed (SRD 5.1, "Being Prone"), on a map or without one. */
  async standUp(campaignId: string, encounterId: string, combatantId: string): Promise<Encounter> {
    const res = await this.base.keyed(['standUp', campaignId, encounterId, combatantId], (sent) =>
      this.base.client.standUp({ campaignId, encounterId, combatantId, idempotencyKey: sent }),
    );
    return need(res.encounter, 'StandUp');
  }

  /** The master's "Oferecer ataque de oportunidade" (a combat without a map): who left whose reach. */
  async offerOpportunity(
    campaignId: string,
    encounterId: string,
    moverId: string,
    reactorId: string,
    idempotencyKey: string,
  ): Promise<Encounter> {
    const res = await this.base.client.offerOpportunity({
      campaignId,
      encounterId,
      moverId,
      reactorId,
      idempotencyKey,
    });
    return need(res.encounter, 'OfferOpportunity');
  }

  /** "Retirar a oferta": an offer nobody answered is taken back; the reactor keeps its reaction. */
  async withdrawOpportunity(
    campaignId: string,
    encounterId: string,
    offerId: string,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['withdrawOpportunity', campaignId, encounterId, offerId],
      (sent) =>
        this.base.client.withdrawOpportunity({
          campaignId,
          encounterId,
          opportunityOfferId: offerId,
          idempotencyKey: sent,
        }),
    );
    return need(res.encounter, 'WithdrawOpportunity');
  }

  async setHidden(
    campaignId: string,
    encounterId: string,
    combatantId: string,
    hidden: boolean,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['setCombatantHidden', campaignId, encounterId, combatantId, hidden],
      (sent) =>
        this.base.client.setCombatantHidden({
          campaignId,
          encounterId,
          combatantId,
          idempotencyKey: sent,
          hidden,
        }),
    );
    return need(res.encounter, 'SetCombatantHidden');
  }

  async add(
    campaignId: string,
    encounterId: string,
    participants: readonly JoinSpec[],
    key?: string,
  ): Promise<Encounter> {
    const res = await this.base.keyed(
      ['addCombatants', campaignId, encounterId, participants],
      (sent) =>
        this.base.client.addCombatants({
          campaignId,
          encounterId,
          idempotencyKey: sent,
          participants: participants.map(toParticipant),
        }),
      key,
    );
    return need(res.encounter, 'AddCombatants');
  }

  async remove(campaignId: string, encounterId: string, combatantId: string): Promise<Encounter> {
    const res = await this.base.keyed(
      ['removeCombatant', campaignId, encounterId, combatantId],
      (sent) =>
        this.base.client.removeCombatant({
          campaignId,
          encounterId,
          combatantId,
          idempotencyKey: sent,
        }),
    );
    return need(res.encounter, 'RemoveCombatant');
  }

  /** What a combatant can do now ("Sua vez"), with the targets of each
   * attack and the damage still to roll or apply. */
  turnOptions(
    campaignId: string,
    encounterId: string,
    combatantId: string,
  ): Promise<GetTurnOptionsResponse> {
    return this.base.client.getTurnOptions({ campaignId, encounterId, combatantId });
  }
}
