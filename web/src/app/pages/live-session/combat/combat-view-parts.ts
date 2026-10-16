import { afterNextRender, computed, inject, Directive } from '@angular/core';
import {
  type InspirationOffer,
  type LuckyOffer,
  EncounterStatus,
  type PendingDamage,
  PendingDamageStatus,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { searchRoute } from '../../../core/traps/trap-search';
import { acts } from '../../../core/combat/joint-turn';
import { featureNote } from '../../../core/combat/combat-derived';
import { CombatSheets } from './combat-sheets';
import { MoveFlow } from './move-flow';
import { ContestHost } from './contest-host';
import { ContestClient } from '../../../core/combat/contest-client';
import { resourceFlow } from '../../../core/resources/resource-actions';
import { openWindows } from '../../../core/combat/reactions';
import { type RevealAnswer } from './hidden-reveal/hidden-reveal-card';
import { CombatViewDerivedDirective } from './combat-view-derived';

/** The questions of a held attack roll the page reopens the attack sheet at: the halfling's Lucky about a 1 (asked
 * first) and the Bardic Inspiration die. */
interface HeldRoll {
  readonly offer?: InspirationOffer;
  readonly lucky?: LuckyOffer;
}
/** The parts the screen is composed of (`CombatSheets`, `MoveFlow`, `ContestHost`) and the calls of the turn, the
 * actions and the attacks. */
@Directive()
export abstract class CombatViewPartsDirective extends CombatViewDerivedDirective {
  /** Every sheet and dialog the screen opens (`CombatSheets`). */
  protected readonly sheets = new CombatSheets({
    dialog: this.dialog,
    bottomSheet: this.bottomSheet,
    api: this.api,
    creatures: this.creaturesApi,
    catalog: this.catalog,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    own: () => this.own(),
    isMaster: () => this.isMaster(),
    state: () => this.state(),
    diceMode: () => this.diceMode(),
    preference: () => this.dicePreference(),
    options: () => this.options(),
    vitals: () => this.vitals(),
    ownVitals: this.ownVitals,
    casterClasses: () => this.casterClasses(),
    armorClass: () => this.armorClass(),
    trapSkills: () => this.trapSkills(),
    mapState: () => this.mapState(),
    image: () => this.image(),
    theatre: () => this.theatre(),
    mapName: () => this.mapName(),
    layers: () => this.layers(),
    fogVision: () => this.fogVision(),
    spendClosed: () => this.moves.moveNote.set(''),
  });
  protected readonly attackSheetOpen = this.sheets.attackOpen;
  protected readonly reactionOpen = this.sheets.reactions.reactionOpen;
  protected readonly reactionHandled = this.sheets.reactions.reactionHandled;
  protected readonly inspirationHandled = this.sheets.inspirationHandled;

  /** Moving: the "Mover" page, the jump, the master's drag and the reach (`MoveFlow`). */
  protected readonly moves = new MoveFlow({
    api: this.api,
    calls: this.calls,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    state: () => this.state(),
    own: () => this.own(),
    isMaster: () => this.isMaster(),
    active: () => this.active(),
    theatre: () => this.theatre(),
    options: () => this.options(),
    creatureOptions: () => this.creatureOptions.data(),
    takeAction: (id, key) => this.takeActionFor(id, key),
    openSpend: (id) => this.sheets.actions.spend(id),
  });
  protected readonly moverId = this.moves.moverId;
  protected readonly dropStart = this.moves.dropStart;
  protected readonly moveError = this.moves.moveError;
  protected readonly moveNote = this.moves.moveNote;
  protected readonly lockedDoor = this.moves.lockedDoor;
  protected readonly forcedMove = this.moves.forcedMove;
  protected readonly forcedNote = this.moves.forcedNote;
  protected readonly moveJump = this.moves.jump;
  protected readonly reachOn = this.moves.reachOn;
  protected readonly moveOptions = this.moves.options;
  protected readonly canDisengage = this.moves.canDisengage;
  protected readonly moverJumps = this.moves.jumps;
  protected readonly masterReach = this.moves.masterReach;
  protected readonly reachSwitch = this.moves.reachSwitch;
  /** The player's side of the contests (W7-X): grapple, shove, escape, Hide, Help, and the questions a contest asks. */
  protected readonly contest = new ContestHost({
    dialog: this.dialog,
    bottomSheet: this.bottomSheet,
    api: inject(ContestClient),
    campaignId: () => this.campaignId(),
    isMaster: () => this.isMaster(),
    encounter: this.encounter,
    own: this.own,
    options: this.options,
    state: () => this.state(),
    diceMode: () => this.diceMode(),
    preference: () => this.dicePreference(),
  });
  // ---- calls ----

  protected begin(): Promise<boolean> {
    return this.calls.run((e) => this.api.turn.begin(this.campaignId(), e.id));
  }

  /** `discard`: the master passes the turn although a damage waits. */
  protected nextTurn(discard = false): Promise<boolean> {
    // A death save still due: the server lets the master pass it, but he is asked first, in place.
    if (this.deathDue().length > 0) {
      this.passAsking.set({ discard });
      return Promise.resolve(false);
    }
    return this.endTurn(discard);
  }

  /** "Passar sem rolar": the turn ends with the death save unrolled. */
  protected passUnrolled(): Promise<boolean> {
    const asked = this.passAsking();
    this.passAsking.set(null);
    return this.endTurn(asked?.discard ?? false);
  }

  protected endTurn(discard: boolean): Promise<boolean> {
    // In a joint turn a player ends their own part; the master ends the part of
    // the first member who still acts (the others have their own button).
    return this.calls.run((e) => {
      const own = this.own();
      const who = !this.isMaster() && own && acts(e, own) ? own.id : e.currentCombatantId;
      return this.api.turn.endTurn(this.campaignId(), e.id, who, discard, e.round);
    });
  }

  /** "Revelar" or "Manter escondidas" on a question about hidden creatures (PM-02c state 9). */
  protected answerReveal(a: RevealAnswer): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.casts.resolveHiddenReveal(this.campaignId(), e.id, a.id, a.reveal),
    );
  }

  /** The last question answered: the focus goes on to the order, the next stop of the column. */
  protected focusOrder(): void {
    afterNextRender(
      () =>
        this.host.nativeElement
          .querySelector<HTMLElement>(
            'app-order-list button:not([disabled]), app-order-list [tabindex="0"]',
          )
          ?.focus(),
      { injector: this.injector },
    );
  }

  /** "Encerrar a parte da Brisa": the master ends one member's part. */
  protected endPart(id: string): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.turn.endTurn(this.campaignId(), e.id, id, false, e.round),
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

  /** A standard action of one of the player's creatures: "Ajudar" and "Esconder" open the same sheets as the character's,
   * for the creature (a familiar's Help is the classic one, SRD 5.1 Find Familiar); the others spend its action. */
  protected creatureAction(id: string, key: string): void {
    const options = this.creatureOptions.data().get(id) ?? null;
    if (key === 'standard:help') {
      this.contest.openHelpFor(id, options);
      return;
    }
    if (key === 'standard:hide') {
      this.contest.openHideFor(id, key, options);
      return;
    }
    void this.takeActionFor(id, key);
  }

  /** A standard action from the list: "Procurar" opens the search for traps (E9-08, it spends the action
   * through `SearchForTraps`); the others spend the action (`takeAction`). */
  protected standardAction(key: string): void {
    // Hide and Help are the contest service's (W7-X): the check, and the master's answer, are not a plain action.
    if (this.contest.hideKeys().includes(key)) {
      this.contest.openHide(key);
      return;
    }
    if (key === 'standard:help') {
      this.contest.openHelp();
      return;
    }
    if (
      key === 'standard:search' &&
      searchRoute(this.mapState().map()?.gridColumns ?? 0, this.own()?.placed ?? false) === 'traps'
    ) {
      this.sheets.actions.searchTraps();
      return;
    }
    void this.takeAction(key);
  }

  /** "Usar" on a feature: Retomar o Fôlego rolls (its own sheet); the others
   * only spend the action and remind the table ("Você tem outra ação"). */
  protected async useFeature(key: string, frenzy = false): Promise<void> {
    const own = this.own();
    const e = this.encounter();
    const option = this.options()?.options?.featureActions.find((a) => a.action?.key === key);
    if (!own || !e || !option?.action) {
      return;
    }
    if (this.contest.hideKeys().includes(key)) {
      // The rogue's Cunning Action: Hide as a bonus action.
      this.contest.openHide(key);
      return;
    }
    const name = option.action.namePt;
    const flow = resourceFlow(key);
    if (flow) {
      this.sheets.actions.resource(flow, key, own.id, e.id);
      return;
    }
    if (key === 'feature:second-wind') {
      this.sheets.actions.secondWind(own, e, name, option.action.economy);
      return;
    }
    const actionKey = key;
    if (
      await this.calls.run(
        async (current) =>
          (
            await this.api.turn.takeAction(
              this.campaignId(),
              current.id,
              own.id,
              actionKey,
              undefined,
              undefined,
              false,
              frenzy,
            )
          ).encounter,
      )
    ) {
      this.actionNote.set(featureNote(name, option.action.resourceKey, frenzy));
    }
  }

  /** "Responder": the question of a held roll (Lucky first, then the die's), at the d20 that was rolled. `false` while
   * the attack is not in the options. */
  protected openHeldRoll(): boolean {
    const held: HeldRoll = {
      offer: this.ownOffer() ?? undefined,
      lucky: this.ownLucky() ?? undefined,
    };
    const key = (held.lucky ?? held.offer)?.attackKey;
    const attack = this.options()?.options?.attacks.find((a) => a.attack?.key === key);
    if (!key || !attack) {
      return false;
    }
    this.sheets.attack({ key, held });
    return true;
  }

  /** "Atacar": the attack sheet (Alvo, Rolar, Dano). `asReaction` is the
   * opportunity attack, off turn. */
  protected openAttack(key: string, asReaction = false, useExtraAction = false): void {
    this.sheets.attack({ key, asReaction, useExtraAction });
  }

  /** A standard action paid with the extra action an effect gives (Velocidade): Disparada, Desengajar, Esconder, Usar um objeto. */
  protected takeExtra(key: string): void {
    const own = this.own();
    if (own) {
      void this.calls.run(
        async (e) =>
          (
            await this.api.turn.takeAction(
              this.campaignId(),
              e.id,
              own.id,
              key,
              undefined,
              undefined,
              true,
            )
          ).encounter,
      );
    }
  }

  /** "Rolar o dano" of a hit whose sheet was closed before the damage. */
  protected rollPendingDamage(): void {
    const p = this.ownPendingRoll();
    if (!p) {
      return;
    }
    const attack = this.options()?.options?.attacks.find(
      (a) => a.attack?.key === p.attackKey,
    )?.attack;
    if (attack && attack.saveDc === 0) {
      this.sheets.attack({ key: p.attackKey, resume: p });
      return;
    }
    // A spell's damage (Mísseis Mágicos, Mãos Flamejantes): the cast sheet, at the damage.
    const own = this.own();
    const owed = (this.options()?.pendingDamages ?? []).filter(
      (x) =>
        x.status === PendingDamageStatus.AWAITING_ROLL &&
        x.attackerId === own?.id &&
        x.attackKey === p.attackKey,
    );
    this.sheets.castSheet(p.attackKey, owed);
  }

  /** "Atacar" on one of the player's creatures (E9-12): the same sheet, with the creature as the attacker; `asReaction` for the familiar of the Pacto da Corrente. */
  protected openCreatureAttack(id: string, key: string, asReaction = false): void {
    const who = this.encounter()?.combatants.find((c) => c.id === id);
    if (who) {
      this.sheets.attack({
        key,
        asReaction,
        who,
        creatureOpts: this.creatureOptions.data().get(id) ?? null,
      });
    }
  }

  /** A hit of one of the player's creatures whose damage waits to be rolled (from the creature's own options). */
  protected creaturePending(id: string): PendingDamage | null {
    return (
      this.creatureOptions
        .data()
        .get(id)
        ?.pendingDamages.find(
          (p) => p.status === PendingDamageStatus.AWAITING_ROLL && p.attackerId === id,
        ) ?? null
    );
  }

  /** "Rolar o dano" of a creature's hit: the attack sheet again, at the damage. */
  protected rollCreatureDamage(id: string): void {
    const p = this.creaturePending(id);
    const who = this.encounter()?.combatants.find((c) => c.id === id);
    if (p && who) {
      this.sheets.attack({
        key: p.attackKey,
        resume: p,
        who,
        creatureOpts: this.creatureOptions.data().get(id) ?? null,
      });
    }
  }
  /** The saving throw of an effect that waits for this player: it has to be answered, so it can be opened again after closing it. */
  protected readonly ownEffectSave = computed(() => {
    const e = this.encounter();
    if (this.isMaster() || !e || e.status !== EncounterStatus.ACTIVE) {
      return null;
    }
    return (
      openWindows(e).find(
        (w) => w.forYou && w.prompt.case === 'effectSave' && !this.reactionOpen(),
      ) ?? null
    );
  });
}
