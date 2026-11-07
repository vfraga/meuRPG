import { opensSheet } from '../../../core/combat/action-spend';
import { afterNextRender, computed, inject, Directive } from '@angular/core';
import {
  EncounterStatus,
  type PendingDamage,
  PendingDamageStatus,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { searchRoute } from '../../../core/traps/trap-search';
import { acts } from '../../../core/combat/joint-turn';
import { featureNote } from '../../../core/combat/combat-derived';
import { CombatSheets } from './combat-sheets';
import { pressedButton, revealMap } from './target-mode/reveal-map';
import { TargetFlow } from './target-mode/target-flow';
import { TargetMode } from './target-mode/target-mode';
import { CombatOffers } from './combat-offers';
import { MoveFlow } from './move-flow';
import { LegendaryFlow } from './legendary-card/legendary-flow';
import { ContestHost } from './contest-host';
import { ZoneFlow } from './zone-flow';
import { ContestClient } from '../../../core/combat/contest-client';
import { resourceFlow } from '../../../core/resources/resource-actions';
import { openWindows } from '../../../core/combat/reactions';
import { type RevealAnswer } from './hidden-reveal/hidden-reveal-card';
import { CombatViewActingDirective } from './combat-view-acting';
import { DiceMode } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { HeldQuestions } from '../../../core/held-rolls/held-questions';
import { HeldVanishWatch } from '../../../core/held-rolls/held-vanish';
import { pendingCountText } from '../../../core/held-rolls/held-text';
import { GearClient } from '../../../core/characters/gear';
import { GearFlow } from '../../../core/combat/gear-flow';
import { ResourceClient } from '../../../core/resources/resources-client';

/** The parts the screen is composed of (`CombatSheets`, `MoveFlow`, `ContestHost`) and the calls of the turn, the
 * actions and the attacks. */
@Directive()
export abstract class CombatViewPartsDirective extends CombatViewActingDirective {
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
    rollDice: () => this.rollDice(),
    armorClass: () => this.armorClass(),
    trapSkills: () => this.trapSkills(),
    mapState: () => this.mapState(),
    image: () => this.image(),
    theatre: () => this.theatre(),
    mapName: () => this.mapName(),
    layers: () => this.layers(),
    fogVision: () => this.fogVision(),
    spendClosed: () => this.moves.moveNote.set(''),
    target: () => this.targeting,
  });
  /** The player's choice of a target on the map for an attack, a spell or a grapple (C7-04, C7-05); the sheets open with it named. */
  protected readonly targeting = new TargetFlow(
    new TargetMode((run) => afterNextRender(run, { injector: this.injector })),
    {
      onMap: () =>
        (!this.isMaster() || !!this.acting()) &&
        this.myTurn() &&
        !this.ownDown() &&
        !this.creatureTab() &&
        !this.theatre() &&
        !!this.image(),
      placed: (id) => !!this.encounter()?.combatants.find((c) => c.id === id)?.placed,
      reveal: () => revealMap(this.host.nativeElement, this.injector, this.laptop()),
      origin: () => pressedButton(this.host.nativeElement),
      by: () => this.acting()?.label ?? '',
    },
  );
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
  /** The legendary cards' side of the page: the options' names for the log, the legendary attack and the armed move (`LegendaryFlow`). */
  protected readonly legendary = new LegendaryFlow({
    api: this.api,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    attack: (req) => this.sheets.attack(req),
    error: this.error,
    armed: this.moves.legendaryMove,
  });
  /** The spell zones: the list the map and the panel read, the master's end and the zone he asked to move (`ZoneFlow`). */
  protected readonly zoneFlow = new ZoneFlow({
    api: this.api,
    calls: this.calls,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    state: () => this.state(),
    isMaster: () => this.isMaster(),
    openMove: (id, onClosed) => this.sheets.moveZone(id, onClosed),
  });
  /** The master's side of the opportunity attacks: the offer form, his answers and the reactors' attacks (`CombatOffers`). */
  protected readonly offers = new CombatOffers({
    api: this.api,
    calls: this.calls,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    theatre: () => this.theatre(),
    options: () => this.options(),
    toAnswer: () => this.toAnswer(),
    info: () => this.info(),
    attack: (offer, key, options) =>
      this.sheets.reactions.opportunityAttack(offer, key, options, true),
  });
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
    target: () => this.targeting,
  });
  /** The player's gear in the combat: "Sacar ou guardar" (`GearFlow`). */
  protected readonly gearFlow = new GearFlow({
    client: inject(GearClient),
    campaignId: () => this.campaignId(),
    characterId: () => (this.isMaster() ? '' : (this.own()?.characterId ?? '')),
    signature: () => {
      const o = this.own();
      return o
        ? `${o.objectInteractionUsed}|${o.actionUsed}|${o.bonusActionUsed}|${o.armorClassBonus}`
        : '';
    },
  });

  /** The master's questions about the rolls that wait for their roller (kept in step with the combat's, in its effects). */
  protected readonly held = new HeldQuestions({
    send: (holdId, answer, key) =>
      this.injector.get(ResourceClient).answerHeldRoll(this.campaignId(), holdId, answer, key),
  });
  protected readonly heldCount = computed(() => pendingCountText(this.held.count()));
  protected readonly heldCanType = computed(() => this.diceMode() !== DiceMode.APP);
  /** The master may roll in the app for a player (the campaign did not force physical dice). */
  protected readonly heldCanRoll = computed(() => this.diceMode() !== DiceMode.PHYSICAL);
  /** The roller's notice when the master answered the question they were shown. */
  protected readonly vanish = new HeldVanishWatch();

  // ---- calls ----

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

  protected endTurn(discard: boolean): Promise<boolean> {
    // In a joint turn a player ends their own part; the master ends the part of
    // the first member who still acts (the others have their own button).
    return this.calls.run((e) => {
      // The player ends their own part; so does the master for the character he acts for (`own` is that one).
      const own = this.own();
      const who = own && acts(e, own) ? own.id : e.currentCombatantId;
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
    if (opensSheet(option)) {
      // A table feature that asks how many, for whom or the dice of its heal: its own sheet.
      this.sheets.actions.tableAction(own, e, option);
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
              0,
              '',
              option.action?.stateActive === true,
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
    const held = this.ownHeld();
    const key = held?.attackKey;
    const attack = this.options()?.options?.attacks.find((a) => a.attack?.key === key);
    if (!held || !key || !attack) {
      return false;
    }
    this.sheets.attack({ key, held });
    return true;
  }

  /** The master's "Aparar: perguntar a cada golpe" of an NPC with a Parry: each hit asks him (a window), or the server decides. */
  protected setParryAsk(change: { id: string; ask: boolean }): Promise<boolean> {
    return this.calls.run((e) =>
      this.api.combatants.setMonsterReactionMode(this.campaignId(), e.id, change.id, change.ask),
    );
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
