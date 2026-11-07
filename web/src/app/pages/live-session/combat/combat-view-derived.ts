import { noticeOpenedDoors } from './door-notice';
import { TemplateRef, computed, effect, input, output, signal, Directive } from '@angular/core';
import {
  CombatLogKind,
  CombatantSide,
  EncounterStatus,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { ActionKey } from '../../../core/connect/idempotency';
import { ResourceClient } from '../../../core/resources/resources-client';
import { HeldRollKind } from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  asksLucky,
  asksReroll,
  asksStroke,
  dieOf,
  heldOfferOf,
  rerollFace,
  weaponHold,
} from '../../../core/held-rolls/held-roll-offer';
import { combatErrorMessage } from '../../../core/combat/combat-errors';
import { wildActionLine } from '../../../core/combat/combat-notices';
import { coverTargets, everyoneStanding } from '../../../core/combat/theatre';
import { offersHolding, waitingText } from '../../../core/combat/opportunity';
import { masterQueue } from '../../../core/combat/reaction-queue';
import type { OfferMark } from '../../../shared/combat-map/combat-map';
import { openDamages, pendingNote } from '../../../core/combat/attack-flow';
import { isDead, isDown, isPlayer } from '../../../core/combat/combat-view';
import type { TabItem } from '../../../shared/tab-list/tab-list';
import { masterWaitNote } from '../../../core/combat/auto-pass';
import {
  coverAgainst,
  creatureTrapNotes,
  deathDueOf,
  dyingOf,
  offerMarks,
  opportunityAttacks,
  pendingRollOf,
  stoppedByReaction,
  trapNote,
  unactedPlayerOnTurn,
} from '../../../core/combat/combat-derived';
import { reactionWait } from '../../../core/combat/reactions';
import { legendaryLeftLine } from '../../../core/combat/legendary-left';
import type { CombatantInfo } from './combat-info';
import { heldWait, revealWhy } from '../../../core/combat/hidden-reveal';
import { CombatDeaths } from './combat-deaths';
import { CombatInitiative } from './combat-initiative';
import { CombatNotices } from './combat-notices';
import { CombatRage } from './combat-rage';
import { CombatViewStateDirective } from './combat-view-state';

/** What the combat bar says while a downed character still owes its death save. */
const DEATH_SAVE_MISSING = 'Falta o teste contra a morte';

/** The most the badge writes before "99+". */
const MAX_BADGE = 99;

/** What the screen derives from the combat for the template: notes, marks, cards and who waits for what. */
@Directive()
export abstract class CombatViewDerivedDirective extends CombatViewStateDirective {
  protected readonly Status = EncounterStatus;
  /** What the last feature said ("Surto de Ação: você tem outra ação"), and the
   * death save just rolled this turn; both are cleared when the turn changes. */
  protected readonly actionNote = signal('');
  /** The turn whose damage was just settled: the master's card stays for its note. */
  protected readonly settledTurn = signal('');
  /** The death saves, the rage question and the initiative, each a plain class over the shared calls. */
  protected readonly deaths = new CombatDeaths({
    api: this.api,
    animator: this.animator,
    calls: this.calls,
    answerHeld: (holdId, answer, key) =>
      this.injector.get(ResourceClient).answerHeldRoll(this.campaignId(), holdId, answer, key),
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    ownId: () => this.own()?.id,
    ownLabel: () => this.own()?.label,
    note: (text) => this.actionNote.set(text),
  });
  /** The question of the player's own death save that the combat says waits (after a reload, or asked on another screen). */
  protected readonly ownDeathQuestion = computed(() => {
    const offer = this.own()?.heldRollOffer;
    return offer?.kind === HeldRollKind.DEATH_SAVE ? offer : undefined;
  });
  protected readonly resumeDeathQuestion = effect(() =>
    this.deaths.resume(this.ownDeathQuestion()),
  );
  protected readonly rage = new CombatRage({
    api: this.api,
    calls: this.calls,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    isMaster: () => this.isMaster(),
    state: () => this.state(),
  });
  protected readonly initiative = new CombatInitiative({
    api: this.api,
    animator: this.animator,
    answerHeld: (holdId, answer, key) =>
      this.injector.get(ResourceClient).answerHeldRoll(this.campaignId(), holdId, answer, key),
    calls: this.calls,
    campaignId: () => this.campaignId(),
    encounter: () => this.encounter(),
    ownId: () => this.own()?.id,
  });
  /** The question of the player's own initiative that the combat says waits (after a reload, or asked on another screen). */
  protected readonly ownInitiativeQuestion = computed(() => {
    const offer = this.own()?.heldRollOffer;
    return offer?.kind === HeldRollKind.INITIATIVE ? offer : undefined;
  });
  protected readonly resumeInitiativeQuestion = effect(() =>
    this.initiative.resume(this.ownInitiativeQuestion()),
  );
  protected readonly leaveFormKey = new ActionKey();
  /** "Combate atualizado agora." (PM-02c 9c): said once when the page comes up with the turn on hold, that is, after a reload;
   * it goes when the wait is over. Nothing here is kept in the browser: the combat says what waits. */
  protected readonly refreshed = signal('');
  /** The cover editor asked for from the order's menu (the fallback for a joint or master turn): the panel opens on that combatant. */
  protected readonly coverRequest = signal<{ id: string; n: number } | null>(null);
  protected readonly everyone = computed(() => {
    const e = this.encounter();
    return e ? everyoneStanding(e) : [];
  });
  /** The log says the table hides the death saves, to the other players and only when someone is down. */
  protected readonly deathNote = computed(() => {
    const e = this.encounter();
    return (
      this.deathsHidden() &&
      !this.isMaster() &&
      !!e &&
      e.combatants.some((c) => isDown(c) && !c.mine)
    );
  });
  /** Whose cover the master marks (the other side of whoever is on turn). */
  protected readonly coverTargets = computed(() => {
    const e = this.encounter();
    return e ? coverTargets(e) : [];
  });
  protected readonly info = computed<ReadonlyMap<string, CombatantInfo>>(() => {
    const merged = new Map(this.rosterInfo());
    // A player has no roster: their own combatant says what the table needs.
    for (const [id, p] of this.partyInfo()) {
      if (!merged.has(id)) {
        merged.set(id, {
          classSummary: p.classSummary,
          playerName: p.playerName,
          kindLabel: '',
          raceName: '',
        });
      }
    }
    return merged;
  });
  /** The characters whose "Dano/Cura" has a vitals row to adjust. */
  protected readonly adjustable = computed(() => new Set(this.vitals().map((v) => v.characterId)));
  // ---- Forma Selvagem (MR-037, E9-11) ----

  /** The druid's Wild Shape as a line of the Ação: its uses ("restam 2 de 2 usos") and why it cannot be used now. `null` without it, and while in a form. */
  protected readonly wild = computed(() => {
    const own = this.own();
    const feature = this.options()?.options?.featureActions.find((a) =>
      a.action?.key.startsWith('feature:wild-shape'),
    );
    const resource = feature?.action
      ? this.ownVitals()?.resources?.find((r) => r.key === feature.action?.resourceKey)
      : undefined;
    return own ? wildActionLine(feature, !!own.wildShapeBeastKey, resource) : null;
  });
  /** The notices of a form or a concentration that ended on its own (`CombatNotices`). */
  protected readonly notices = new CombatNotices({
    api: this.api,
    calls: this.calls,
    campaignId: () => this.campaignId(),
    ownId: () => this.own()?.id,
  });
  /** On a phone, on the player's own turn, the order strip goes under the actions instead of between the title and them. */
  protected readonly orderBelow = computed(
    () => !this.laptop() && this.myTurn() && !this.ownDown() && !this.creatureTab(),
  );
  /** The conversation the page draws in the rail ("Turno · Conversa" for the master, "Ações · Conversa" for the player), or `null` where the page keeps it elsewhere. */
  readonly chat = input<TemplateRef<unknown> | null>(null);
  /** The conversation's lines that came while it was not shown: the count of its tab. */
  readonly chatUnread = input(0);
  /** The conversation is what the rail shows now (its tab is open): the page counts nothing as unread meanwhile. */
  readonly chatShown = output<boolean>();
  /** The player's own move opened a door: the page says so in a toast. */
  readonly doorOpened = output<void>();
  protected readonly doors = noticeOpenedDoors(this.log, this.own, () => this.doorOpened.emit());
  protected readonly railTab = signal('actions');
  /** "Ações", and "Conversa" with its unread count (grená, and the number in words in the tab's name). */
  protected readonly railTabItems = computed<readonly TabItem[]>(() => {
    const n = this.chatUnread();
    return [
      { id: 'actions', label: 'Ações', icon: 'swords' },
      ...(this.chat() !== null
        ? [
            {
              id: 'chat',
              label: 'Conversa',
              icon: 'chat_bubble_outline',
              ...(n > 0
                ? {
                    badge: n > MAX_BADGE ? `${MAX_BADGE}+` : String(n),
                    tone: 'news' as const,
                    words: `${n} ${n === 1 ? 'mensagem nova' : 'mensagens novas'}`,
                  }
                : {}),
            },
          ]
        : []),
    ];
  });
  /** The player's own character on its turn, with the movement row under the map (a map, no creature's page, not down). */
  protected readonly moveRowOn = computed(() => {
    const own = this.own();
    return own && this.myTurn() && !this.ownDown() && !this.creatureTab() && !this.theatre()
      ? own
      : null;
  });
  /** From 1024px, off turn, the rail still has the character's reaction to say (C6-07). */
  protected readonly offRail = computed(() => {
    const own = this.own();
    return own && !this.myTurn() && !this.creatureTab() && !this.ownDown() && !isDead(own)
      ? own
      : null;
  });
  /** The actions column is a rail: on the player's turn with its options, a creature's page, or off turn with a reaction to say, and with the conversation in it. */
  protected readonly railOn = computed(
    () =>
      (this.myTurn() && !!this.options()) ||
      !!this.creatureTab() ||
      !!this.offRail() ||
      this.chat() !== null,
  );
  /** The turn bar sticks under the bars of the page from 1024px, on the player's own turn. */
  protected readonly stickyTurn = computed(
    () => this.laptop() && this.myTurn() && !this.ownDown() && !this.creatureTab(),
  );
  /** What it waits for ("Esperando a reação do mestre") on the page of a creature's turn. */
  protected readonly creatureLocked = computed(() => this.waiting()?.title ?? '');

  /** What the waiting mover reads: their move landed, and each offer waits for its answer. Any other wait for the master
   * (`turn_held`: a question about hidden creatures) is only "Esperando o mestre", on the player's turn and on another's,
   * never why (RN-10). */
  protected readonly waiting = computed(() => {
    const e = this.encounter();
    const own = this.own();
    if (!e || !own || this.isMaster()) {
      return null;
    }
    // What the server says the combat waits for (a reaction, a concentration save), written for this player.
    return (
      reactionWait(e) ?? waitingText(e, offersHolding(e, own.id)) ?? heldWait(e, this.myTurn())
    );
  });
  /** "Restam 2 ações lendárias: ...": under "Próximo turno" while a legendary creature has points to spend now. */
  protected readonly legendaryLeft = computed(() => {
    const e = this.encounter();
    return e && this.isMaster() ? legendaryLeftLine(e) : '';
  });
  /** Why "Próximo turno" waits while questions about hidden creatures are open ("Responda ao pedido abaixo para seguir."). */
  protected readonly waitWhy = computed(() => {
    const e = this.encounter();
    return e && this.isMaster() ? revealWhy(e) : '';
  });
  /** The master's last action is a move: the log's undoable entry says so ("Desfazer o movimento"). */
  protected readonly canUndoMove = computed(
    () => this.log.undoable()?.kind === CombatLogKind.MOVED,
  );
  protected readonly turnSide = computed(() => this.subject()?.side ?? CombatantSide.UNSPECIFIED);
  protected readonly turnLabel = computed(() =>
    this.isMaster() ? (this.subject()?.label ?? '') : '',
  );
  /** "Voltar à forma normal": a bonus action. The druid knows it came back: the log's line says "left", and no notice comes of it. */
  protected async leaveForm(): Promise<void> {
    const own = this.own();
    if (!own || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.creaturesApi.leaveWildShape(
        this.campaignId(),
        own.characterId,
        this.leaveFormKey.keyFor(own.characterId),
      );
      this.leaveFormKey.renew();
      if (res.encounter) {
        this.state().apply(res.encounter);
      }
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'voltar à forma normal'));
      await this.calls.refreshAfter(err);
    } finally {
      this.busy.set(false);
    }
  }
  /** The contest facts of the player's turn (hidden, surprised, grappled) and the special attacks, from `GetTurnOptions`. */
  protected readonly contestFacts = computed(() => this.options()?.contestState);
  protected readonly contestAttacks = computed(() => this.options()?.contestAttackOptions ?? []);
  protected readonly economy = computed(() => this.options()?.options?.economy);
  /** Extra Attack: the attacks of this Attack action that remain, and how many it makes. */
  protected readonly attacksLeft = computed(() => this.economy()?.attacksLeft ?? 0);
  protected readonly attacksPerAction = computed(() => this.economy()?.attacksPerAction ?? 1);
  /** The spell the player concentrates on, written out. */
  protected readonly ownConcentration = computed(() => {
    return this.own()?.concentrationSpellNamePt ?? '';
  });
  /** What the master's turn still owes a hit: "Falta aplicar 5 de dano". */
  protected readonly owed = computed(() =>
    this.isMaster() ? pendingNote(this.options()?.pendingDamages ?? []) : null,
  );
  /** The master's card shows for an NPC on turn, and for any turn that left
   * damage to apply. */
  protected readonly masterCard = computed(() => {
    const c = this.subject();
    return this.isMaster() && c ? c : null;
  });
  protected readonly cardVisible = computed(() => {
    const c = this.masterCard();
    return (
      !!c &&
      (!isPlayer(c) ||
        openDamages(this.options()?.pendingDamages ?? []).length > 0 ||
        this.settledTurn() === c.id)
    );
  });
  protected readonly unplaced = computed(() =>
    this.theatre() ? [] : (this.encounter()?.combatants ?? []).filter((c) => !c.placed),
  );
  /** The master's bar: "Esperando a sua resposta: escondidas atingidas", or "Esperando a sua reação: Goblin 2". */
  protected readonly waitNote = computed(() => {
    const e = this.encounter();
    if (!e || !this.isMaster()) {
      return '';
    }
    // A downed character's turn starts with its death save (SRD 5.1): the turn does not pass without it (G-02).
    if (this.deathDue().length > 0) {
      return DEATH_SAVE_MISSING;
    }
    return masterWaitNote(e, this.autoLeft(), (id) => this.info().get(id)?.playerName ?? '');
  });
  /** The reactors drawn on the map for whoever answers, with the square the mover left. */
  protected readonly offerMarks = computed<readonly OfferMark[]>(() => {
    const e = this.encounter();
    return e ? offerMarks(e) : [];
  });
  /** Who has the cover each combatant has against whoever has the turn (the master's order list). */
  protected readonly coverAgainst = computed(() => coverAgainst(this.options()));
  /** The melee attacks an opportunity attack can use: off turn, with the reaction free, someone in reach and the character
   * on its feet (E6-28). */
  protected readonly opportunities = computed(() =>
    opportunityAttacks(
      this.own(),
      this.options(),
      !this.isMaster() && !this.myTurn() && this.active(),
    ),
  );
  /** The characters at three failures: the master is asked to confirm each death (E6-30). */
  /** The player on turn who has not acted: "Próximo turno" asks before passing. */
  protected readonly idleOnTurn = computed(() => unactedPlayerOnTurn(this.encounter()));
  protected readonly dying = computed(() => (this.isMaster() ? dyingOf(this.encounter()) : []));
  /** The downed player characters whose death save is due this turn: the master rolls it when the player is away (RN-03). */
  protected readonly deathDue = computed(() =>
    this.isMaster() ? deathDueOf(this.encounter()) : [],
  );
  /** The player's hit whose damage is still to roll (the sheet was closed). */
  protected readonly ownPendingRoll = computed(() => pendingRollOf(this.options(), this.own()?.id));
  /** A window waits for the master: the queue stands above the turn card, so its answer is on the first screen. */
  protected readonly queueFirst = computed(() => {
    const e = this.encounter();
    return this.isMaster() && !!e && masterQueue(e).count > 0;
  });
  /** The newest attack of the one on turn was stopped by the target's Escudo (from the log). */
  protected readonly reactionStopped = computed(() =>
    stoppedByReaction(this.log.rounds(), this.subject()?.id),
  );
  /** What a trap did to this player's character this round (E9-08 E), from the combat log. */
  protected readonly trapNote = computed(() =>
    trapNote(this.encounter(), this.own(), this.log.rounds()),
  );
  /** The same note for each of the player's creatures that a trap caught (a wolf or Nanquim that walked into one on its own part). */
  protected readonly creatureTrapNotes = computed(() =>
    creatureTrapNotes(this.encounter(), this.isMaster(), this.log.rounds()),
  );
  /** The Bardic Inspiration die the player's character holds (the server sends it to the holder and the master only). */
  protected readonly ownDie = computed(() => this.own()?.inspirationDie ?? null);
  /** The weapon attack of the player's character that waits for an answer: the halfling's Lucky ("Tirou 1: rolar de
   * novo?"), asked first, or the Bardic Inspiration die. The attack sheet reopens at it. */
  protected readonly ownHeld = computed(() => {
    const own = this.own();
    return own ? weaponHold(heldOfferOf(own)) : null;
  });
  protected readonly ownLucky = computed(() => asksLucky(this.ownHeld()));
  /** The weapon attack waits for a table feature's reroll ("Tirou 2: rolar de novo?"). */
  protected readonly ownReroll = computed(() => asksReroll(this.ownHeld()));
  protected readonly ownRerollFace = computed(() => rerollFace(this.ownHeld()));
  protected readonly ownStroke = computed(() => asksStroke(this.ownHeld()));
  /** The Bardic Inspiration die's question, and only it. */
  protected readonly ownDieAsked = computed(() => dieOf(this.ownHeld()) !== undefined);
}
