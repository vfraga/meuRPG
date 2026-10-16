import { Directive } from '@angular/core';
import {
  CombatEffect,
  DeathSaveOutcome,
  CombatantSide,
  CoverDegree,
  type Encounter,
  type OpportunityOffer,
  type ReactionWindow,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { type AttackDie } from '../../../core/combat/combat-client-types';
import { newKey } from '../../../core/connect/idempotency';
import { type MineTab, membersToEnd } from '../../../core/combat/mine';
import type { Square } from '../../../core/combat/combat-grid';
import { outcomeText, saveAnnouncement } from '../../../core/combat/death-saves';
import { showOfDice, showOfDie } from '../../../shared/roll-overlay/roll-animator';
import { currentCombatant } from '../../../core/combat/combat-view';
import type { TokenDrop } from '../../../shared/combat-map/combat-map';
import { initiativeLine } from '../../../core/combat/combat-derived';
import { type DeathSaveRoll } from './death-save-due/death-save-due';
import { type RiderPick } from './hit-rider/hit-rider-card';
import { type JumpRequest } from './move-page/move-page';
import { type MasterAnswer } from './opportunity/opportunity-card';
import type { DoorSquare } from '../../../core/maps/layers';
import {
  type StartCombatData,
  StartCombatDialog,
} from '../../../shared/start-combat/start-combat-dialog';
import { CombatViewPartsDirective } from './combat-view-parts';

/** The d20 the initiative is rolled with. */
const INITIATIVE_DIE = 20;
/** The rest of the screen's calls: the death saves, the rage, the form, the initiative, the opportunity attacks and
 * the move. */
@Directive()
export abstract class CombatViewFlowsDirective extends CombatViewPartsDirective {
  /** The death save of the player's own turn (RN-03, RN-18 per roll). */
  protected async rollDeathSave(die: AttackDie): Promise<void> {
    const own = this.own();
    if (!own) {
      return;
    }
    const text = await this.rollDeathSaveOf(own.id, own.label, die);
    if (text) {
      this.deathResult.set(text);
    }
  }

  /** The master rolls the death save of a player who is not there (the server accepts the master, RN-03). */
  protected async rollMasterDeathSave(roll: DeathSaveRoll): Promise<void> {
    const who = this.encounter()?.combatants.find((c) => c.id === roll.id);
    if (!who) {
      return;
    }
    this.passAsking.set(null);
    const text = await this.rollDeathSaveOf(who.id, who.label, roll.die);
    if (text) {
      this.masterDeathResult.set(text);
    }
  }

  /** One death save call, with the dice animation; the sentence it announced, or '' when the call failed. */
  protected async rollDeathSaveOf(id: string, label: string, die: AttackDie): Promise<string> {
    let text = '';
    await this.calls.run(async (e) => {
      const key = this.deathKey.keyFor([e.id, id, die]);
      const res = await this.api.combatants.rollDeathSave(this.campaignId(), e.id, id, die, key);
      this.deathKey.renew();
      text = saveAnnouncement(res.save, label);
      // The death save, as the line under the sheet tells it (hidden saves arrive without a roll: nothing plays).
      const show = showOfDice('Teste contra a morte', res.save.roll, {
        withTotal: true,
        outcome: {
          word: outcomeText(res.save.outcome),
          good:
            res.save.outcome === DeathSaveOutcome.SUCCESS ||
            res.save.outcome === DeathSaveOutcome.REVIVED,
        },
        critical: res.save.outcome === DeathSaveOutcome.REVIVED,
        fumble: res.save.outcome === DeathSaveOutcome.CRITICAL_FAILURE,
      });
      if (show) {
        this.animator.play(show);
      }
      if (res.save.outcome === DeathSaveOutcome.REVIVED && id === this.own()?.id) {
        this.actionNote.set(text);
      }
      return res.encounter;
    });
    return text;
  }

  /** The master's "Confirmar a morte" (ConfirmDeath): the character is dead for good. */
  protected async confirmDeath(id: string): Promise<void> {
    await this.calls.run(async (e) => {
      const res = await this.api.combatants.confirmDeath(
        this.campaignId(),
        e.id,
        id,
        this.confirmDeathKey.keyFor([e.id, id]),
      );
      this.confirmDeathKey.renew();
      return res;
    });
  }

  /** "Voltar e atacar" (`endRage` false) or "Deixar a fúria acabar": the answer to the rage question. */
  protected async answerRage(id: string, endRage: boolean): Promise<void> {
    // "Deixar a fúria acabar" ends the rage and then passes the turn, in one tap, as the
    // question says: the server only ends the rage, so the turn goes in the same call.
    await this.calls.run(async (e) => {
      const answered = await this.api.turn.answerRageEnd(this.campaignId(), e.id, id, endRage);
      if (!endRage) {
        return answered;
      }
      this.state().apply(answered);
      return this.api.turn.endTurn(this.campaignId(), e.id, id, false, answered.round);
    });
  }

  /** "Técnica da Mão Aberta" and "Golpe Atordoante": the monk's answer to an offer after a hit. */
  protected answerRider(pick: RiderPick): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.turn.useHitRider(this.campaignId(), e.id, pick.id, pick.choice),
    );
  }

  /** "Encerrar fúria": the bonus action. */
  protected endRageOf(id: string): Promise<boolean> {
    return this.calls.run((e) => this.api.turn.endRage(this.campaignId(), e.id, id));
  }

  protected deathLaterFor(id: string): void {
    this.deathLater.update((s) => new Set(s).add(id));
  }

  protected deathAskAgain(id: string): void {
    this.deathLater.update((s) => {
      const next = new Set(s);
      next.delete(id);
      return next;
    });
  }

  /** The player ends their own concentration (the conditions are the master's). */
  protected endOwnConcentration(): Promise<boolean> {
    const own = this.own();
    this.concentration.endedByMe = true;
    return own
      ? this.calls
          .run((e) =>
            this.api.combatants.setConditions(this.campaignId(), e.id, own.id, {
              endConcentration: true,
            }),
          )
          .then((ok) => {
            if (!ok) {
              this.concentration.endedByMe = false;
            }
            return ok;
          })
      : Promise.resolve(false);
  }

  /** "Encerrar a parte dos Lobos" / "Encerrar a vez do Nanquim": the part of every member of the tab that still acts, one call each (a joint turn ends part by part). */
  protected async endTab(tab: MineTab): Promise<void> {
    for (const member of tab.members) {
      const e = this.encounter();
      // Read again before each call: the answer to the one before may have moved the turn on.
      if (!e || !membersToEnd(e, { ...tab, members: [member] }).length) {
        continue;
      }
      if (
        !(await this.calls.run((current) =>
          this.api.turn.endTurn(this.campaignId(), current.id, member.id, false, current.round),
        ))
      ) {
        return;
      }
    }
  }
  /** A damage was settled: the master's card stays for its note. */
  protected settled(): void {
    this.settledTurn.set(this.subject()?.id ?? '');
  }

  protected endCombat(): Promise<boolean> {
    return this.calls.run((e) => this.api.turn.end(this.campaignId(), e.id));
  }

  protected submitFace(combatantId: string, face: number): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.turn.submitInitiative(this.campaignId(), e.id, combatantId, { face }),
    );
  }

  protected async rollInApp(combatantId: string): Promise<boolean> {
    const done = await this.calls.run((e) =>
      this.api.turn.submitInitiative(this.campaignId(), e.id, combatantId, { inApp: true }),
    );
    if (done) {
      this.animateInitiative(combatantId);
    }
    return done;
  }

  /** The initiative d20 the app just rolled for the player's own combatant, as the initiative card tells it (d20 + bonus = total). */
  protected animateInitiative(combatantId: string): void {
    const shown = initiativeLine(this.encounter()?.combatants.find((x) => x.id === combatantId));
    const show = shown && showOfDie('Iniciativa', INITIATIVE_DIE, shown.face, shown.line);
    if (show) {
      this.animator.play(show);
    }
  }

  protected setOrder(ids: string[]): Promise<boolean> {
    return this.calls.run((e) => this.api.turn.setOrder(this.campaignId(), e.id, ids));
  }

  protected reveal(change: { id: string; hidden: boolean }): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.moves.setHidden(this.campaignId(), e.id, change.id, change.hidden),
    );
  }

  /** The master: "Perdeu a concentração" (the app does not roll the save; the damage card reminds the DC). */
  protected loseConcentration(id: string): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.combatants.setConditions(this.campaignId(), e.id, id, { endConcentration: true }),
    );
  }

  /** The master: "Encerrar Ajuda": the bonus goes, and the hit points lose only what passes the new maximum. */
  protected endAid(id: string): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.combatants.endCombatEffect(this.campaignId(), e.id, id, CombatEffect.AID),
    );
  }

  protected remove(id: string): Promise<boolean> {
    return this.calls.run((e) => this.api.moves.remove(this.campaignId(), e.id, id));
  }

  protected adjustFor(characterId: string): void {
    const v = this.vitals().find((x) => x.characterId === characterId);
    if (v) {
      this.adjust.emit(v);
    }
  }

  /** "Adicionar combatente": the start dialog, with the NPCs only. */
  protected addCombatants(): void {
    const e = this.encounter();
    if (!e) {
      return;
    }
    const phone = this.phone();
    this.dialog
      .open<StartCombatDialog, StartCombatData, Encounter>(StartCombatDialog, {
        data: {
          campaignId: this.campaignId(),
          mode: 'add',
          map: null,
          encounterId: e.id,
          existing: e.combatants.length,
        },
        width: phone ? '100vw' : '760px',
        maxWidth: phone ? '100vw' : 'calc(100vw - 32px)',
        height: phone ? '100dvh' : undefined,
        maxHeight: phone ? '100dvh' : '92dvh',
        ariaLabelledBy: 'start-title',
        autoFocus: 'first-tabbable',
      })
      .afterClosed()
      .subscribe((added) => added && this.state().apply(added));
  }
  // ---- opportunity attacks (E9-13) ----

  /** The master answers for an NPC: "Não atacar", or "Atacar com <arma>". */
  protected answerOpportunity(a: MasterAnswer): Promise<boolean> | void {
    if (!a.attack) {
      return this.calls.run((e) =>
        this.api.moves.declineOpportunity(this.campaignId(), e.id, a.offer.id),
      );
    }
    this.sheets.reactions.opportunityAttack(
      a.offer,
      a.attack.key,
      this.reactorOptions().get(a.offer.id),
      true,
    );
  }

  /** "Desfazer o movimento": the master takes back his last action when it is the move that made the offers. */
  protected undoMove(): Promise<boolean> {
    const entry = this.log.undoable();
    if (!entry) {
      return Promise.resolve(false);
    }
    return this.calls.run(async (e) => {
      const undone = await this.api.turn.undo(this.campaignId(), e.id, entry.id);
      this.undone.mark();
      return undone;
    });
  }

  /** "Seguir sem esperar": the master passes over an offer the player does not answer. */
  protected skipOpportunity(offer: OpportunityOffer): Promise<boolean> {
    return this.calls.run((e) => this.api.moves.skipOpportunity(this.campaignId(), e.id, offer.id));
  }

  /** "Oferecer a Toren": the master says the one on turn left that combatant's reach (a combat without a map). */
  protected async makeOffer(reactorId: string): Promise<void> {
    const e = this.encounter();
    const mover = e ? currentCombatant(e) : null;
    if (!e || !mover) {
      return;
    }
    this.offerError.set('');
    let key = this.offerKeys.get(reactorId);
    if (!key) {
      key = newKey();
      this.offerKeys.set(reactorId, key);
    }
    const ok = await this.calls.run((current) =>
      this.api.moves.offerOpportunity(this.campaignId(), current.id, mover.id, reactorId, key),
    );
    if (ok) {
      this.offering.set(false);
    } else if (this.error()) {
      // The refusal stays in the form, where the master is looking (the page's notice has it too).
      this.offerError.set(this.error());
      this.error.set('');
      // A refused offer is not a retry: the next try is a new one.
      this.offerKeys.delete(reactorId);
    }
  }

  /** "Oferecer ataque de oportunidade": opens the form under its button (a new form, new keys), or closes it. */
  protected toggleOffer(): void {
    if (!this.offering()) {
      this.offerKeys = new Map();
      this.offerError.set('');
    }
    this.offering.set(!this.offering());
  }

  /** "Retirar a oferta": the master takes back an offer nobody answered; the reactor keeps its reaction. */
  protected withdrawOpportunity(offer: OpportunityOffer): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.moves.withdrawOpportunity(this.campaignId(), e.id, offer.id),
    );
  }

  /** The master's "Marcar como aliado" / "Voltar a ser inimigo". */
  protected setSide(change: { id: string; side: CombatantSide }): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.moves.setSide(this.campaignId(), e.id, change.id, change.side),
    );
  }

  /** The master's cover mark, applied at once. */
  protected setCover(change: { id: string; cover: CoverDegree }): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.moves.setCover(this.campaignId(), e.id, change.id, change.cover),
    );
  }

  // ---- moving ----

  /** "Levantar-se": the combatant (the player's own, a creature of theirs, or the one on the master's card) stands up from Prone. */
  protected standUp(id?: string): Promise<boolean> {
    const who = id ?? this.own()?.id;
    return who
      ? this.calls.run((e) => this.api.moves.standUp(this.campaignId(), e.id, who))
      : Promise.resolve(false);
  }

  /** "Colocar no mapa": the server picks the square, by the rule that places the
   * NPCs when the combat begins (free floor, away from the players). */
  protected async place(id: string): Promise<void> {
    await this.calls.run((current) => this.api.moves.place(this.campaignId(), current.id, id));
  }

  // ---- sheets and moving: the template and the screens below call these ----

  protected askFamiliarEyes(): void {
    const name = this.familiar();
    if (name) {
      this.sheets.actions.familiarEyes(name);
    }
  }
  protected openTransform(): void {
    this.sheets.actions.wildShape();
  }
  protected openDoor(door: DoorSquare): void {
    this.sheets.actions.door(door);
  }
  protected describeSpell(key: string, name: string): void {
    this.sheets.actions.describeSpell(key, name);
  }
  protected openCast(key: string): Promise<void> {
    return this.sheets.cast(key);
  }
  protected openMasterArea(key: string): void {
    this.sheets.masterArea(key);
  }
  protected openConditions(combatantId: string): void {
    this.sheets.actions.conditions(combatantId);
  }
  protected openEffectSave(window: ReactionWindow): void {
    this.sheets.reactions.effectSave(window);
  }
  protected adjustNpc(combatantId: string): void {
    this.sheets.actions.adjustNpc(combatantId);
  }
  protected openSpend(id?: string): void {
    this.sheets.actions.spend(id);
  }
  protected drop(drop: TokenDrop): Promise<void> {
    return this.moves.drop(drop);
  }
  protected confirmMove(to: Square): Promise<void> {
    return this.moves.confirmMove(to);
  }
  protected confirmJump(req: JumpRequest): Promise<void> {
    return this.moves.confirmJump(req);
  }
  protected disengage(): Promise<boolean> {
    return this.moves.disengage();
  }
  protected closeFullPage(): void {
    this.moves.closeFullPage();
  }
  protected openMove(): void {
    this.moves.open();
  }
  protected openCreatureMove(id: string): void {
    this.moves.openCreature(id);
  }

  protected leave(): void {
    this.state().dismissEnded();
  }
}
