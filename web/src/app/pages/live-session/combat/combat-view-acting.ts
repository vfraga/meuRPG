import { computed, Directive } from '@angular/core';

import { isDead, isDown, isPlayer, soloTitle } from '../../../core/combat/combat-view';
import { vitalsPool } from '../../../core/combat/form-pool';
import { type MineTab, membersToEnd } from '../../../core/combat/mine';
import { moveLine } from '../../../core/combat/move-line';
import { turnEnds } from '../../../core/combat/pair-turn';
import { reactionPending } from '../../../core/combat/combat-errors';
import { economyShapes } from '../../../shared/economy-shapes/economy-shapes';
import { CombatViewDerivedDirective } from './combat-view-derived';
import { type RevealAnswer } from './hidden-reveal/hidden-reveal-card';

/**
 * What the master's "Turno" reads to act for a player character (C8-01, C8-02): the card with "Agir por …" while he does not, the
 * character's shapes and hit points while he does, and whether the NPC's card is the one to draw. The character the master acts
 * for is `acting`; `own` is that one (see `CombatViewStateDirective`). The calls that start and end a turn or a part, and a
 * standard action, are here too: they need only the state.
 */
@Directive()
export abstract class CombatViewActingDirective extends CombatViewDerivedDirective {
  /** The player character on turn, on its feet, that the master has not taken the turn of: its card has "Agir por". */
  protected readonly actCard = computed(() => {
    const c = this.masterCard();
    return c && isPlayer(c) && !this.acting() && !isDown(c) && !isDead(c) && !this.cardVisible()
      ? c
      : null;
  });
  /** "Vez do Pensantus": the same title the bar and the player's own screens give a turn. */
  protected readonly actCardTitle = computed(() => {
    const e = this.encounter();
    const c = this.actCard();
    return e && c ? soloTitle(e, c) : '';
  });
  /** The NPC card: for an NPC on turn, and for a player's turn that left damage to apply; never over the player's own card. */
  protected readonly npcCardOn = computed(() => {
    const c = this.masterCard();
    if (!c || this.acting() || this.actCard()) {
      return null;
    }
    const damage = this.cardVisible();
    if (isPlayer(c)) {
      return damage ? c : null;
    }
    return !this.laptop() || damage || this.theatre() ? c : null;
  });
  /** The hit points of the character on the card (from its vitals: the master reads the numbers). */
  protected readonly actCardHitPoints = computed(() => {
    const c = this.actCard();
    const v = c ? this.vitals().find((x) => x.characterId === c.characterId) : undefined;
    return v ? vitalsPool(v) : null;
  });
  /** The shapes of the character the master acts for, with the walker while there is a move to make. */
  protected readonly actingShapes = computed(() => {
    const c = this.acting();
    if (!c) {
      return [];
    }
    const line = moveLine(c, this.contestFacts()?.grappled === true);
    return economyShapes(c, {
      attacksLeft: this.attacksLeft(),
      attacksPerAction: this.attacksPerAction(),
      moveText: line.canMove ? line.left : null,
    });
  });

  // ---- the calls of the turn ----

  protected begin(): Promise<boolean> {
    return this.calls.run((e) => this.api.turn.begin(this.campaignId(), e.id));
  }

  /** `discard`: the master passes the turn although a damage waits. A downed character's turn does not pass before its death
   * save (SRD 5.1): the button is off and the server refuses it too. */
  protected nextTurn(discard = false): Promise<boolean> {
    if (this.deathDue().length > 0) {
      return Promise.resolve(false);
    }
    return this.endTurn(discard);
  }

  /**
   * One call for each part the button ends (`turnEnds`): a rider's own part, then the mount's; a refusal stops there. The end of
   * the rider's part may open a save of its own (an effect that asks at the end of the turn), which holds the mount's call: that
   * refusal is expected and says nothing, as the save's sheet is what the player reads; the button stays for the mount's part.
   */
  protected async endTurn(discard: boolean): Promise<boolean> {
    const e = this.encounter();
    let ended = 0;
    for (const id of e ? turnEnds(e, this.own(), this.riding()) : []) {
      if (!(await this.endPart(id, discard, ended > 0 ? reactionPending : undefined))) {
        return false;
      }
      ended++;
    }
    return !!e;
  }

  /** "Revelar" or "Manter escondidas" on a question about hidden creatures (PM-02c state 9). */
  protected answerReveal(a: RevealAnswer): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.casts.resolveHiddenReveal(this.campaignId(), e.id, a.id, a.reveal),
    );
  }

  /** "Encerrar a parte da Brisa": the master ends one member's part. */
  protected endPart(
    id: string,
    discard = false,
    expected?: (err: unknown) => boolean,
  ): Promise<boolean> {
    return this.calls.run(
      (e) => this.api.turn.endTurn(this.campaignId(), e.id, id, discard, e.round),
      expected,
    );
  }

  /** A standard action ("Disparada"): it spends the action; Dash doubles the
   * movement. */
  protected takeAction(key: string): Promise<boolean> {
    const own = this.own();
    return own ? this.takeActionFor(own.id, key) : Promise.resolve(false);
  }

  /** The same for a combatant of the player's: their character, or one of their creatures. */
  protected takeActionFor(id: string, key: string): Promise<boolean> {
    return this.calls.run(
      async (e) => (await this.api.turn.takeAction(this.campaignId(), e.id, id, key)).encounter,
    );
  }

  /** "Encerrar a parte dos Lobos" / "Encerrar a vez do Nanquim": the part of every member of the tab that still acts, one call each (a joint turn ends part by part). */
  protected async endTab(tab: MineTab): Promise<void> {
    for (const member of tab.members) {
      const e = this.encounter();
      // Read again before each call: the answer to the one before may have moved the turn on.
      if (!e || !membersToEnd(e, { ...tab, members: [member] }).length) {
        continue;
      }
      if (!(await this.endPart(member.id))) {
        return;
      }
    }
  }
}
