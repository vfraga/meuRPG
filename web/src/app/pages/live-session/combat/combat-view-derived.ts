import { computed, signal, Directive } from '@angular/core';
import {
  CombatLogKind,
  CombatantSide,
  EncounterStatus,
  type OpportunityOffer,
  type GetTurnOptionsResponse,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { ActionKey } from '../../../core/connect/idempotency';
import { combatErrorMessage } from '../../../core/combat/combat-errors';
import {
  type FormEnded,
  ConcentrationWatch,
  FormNotices,
  type LostConcentration,
  formNoticeText,
  lostNoticeText,
  wildActionLine,
} from '../../../core/combat/combat-notices';
import { coverTargets, everyoneStanding, reactorRows } from '../../../core/combat/theatre';
import {
  type ReactorAttack,
  offersHolding,
  reactorAttacks,
  waitingText,
} from '../../../core/combat/opportunity';
import type { OfferMark } from '../../../shared/combat-map/combat-map';
import { openDamages, pendingNote } from '../../../core/combat/attack-flow';
import { isDown, isPlayer } from '../../../core/combat/combat-view';
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
} from '../../../core/combat/combat-derived';
import { reactionWait } from '../../../core/combat/reactions';
import type { CombatantInfo } from './combat-info';
import { heldWait, revealWhy } from '../../../core/combat/hidden-reveal';
import { CombatViewStateDirective } from './combat-view-state';

/** What the screen derives from the combat for the template: notes, marks, cards and who waits for what. */
@Directive()
export abstract class CombatViewDerivedDirective extends CombatViewStateDirective {
  protected readonly Status = EncounterStatus;
  /** What the last feature said ("Surto de Ação: você tem outra ação"), and the
   * death save just rolled this turn; both are cleared when the turn changes. */
  protected readonly actionNote = signal('');
  protected readonly deathResult = signal('');
  /** The turn whose damage was just settled: the master's card stays for its note. */
  protected readonly settledTurn = signal('');
  /** The characters at three failures the master put away with "Ainda não". */
  protected readonly deathLater = signal<ReadonlySet<string>>(new Set());
  protected readonly deathKey = new ActionKey();
  /** The master tapped "Próximo turno" while a death save was due: the question waits for "Rolar agora" or "Passar sem rolar". */
  protected readonly passAsking = signal<{ readonly discard: boolean } | null>(null);
  /** The sentence of the death save the master rolled for an absent player, kept after the save is no longer due. */
  protected readonly masterDeathResult = signal('');
  protected readonly confirmDeathKey = new ActionKey();
  protected readonly leaveFormKey = new ActionKey();
  /** "Combate atualizado agora." (PM-02c 9c): said once when the page comes up with the turn on hold, that is, after a reload;
   * it goes when the wait is over. Nothing here is kept in the browser: the combat says what waits. */
  protected readonly refreshed = signal('');
  /** The master's "Oferecer ataque de oportunidade" form is open. */
  protected readonly offering = signal(false);
  protected readonly offerError = signal('');
  /** Who the mover can have left the reach of (the other side, standing): the form's rows. */
  protected readonly offerRows = computed(() => {
    const e = this.encounter();
    return e
      ? reactorRows(
          e,
          (c) =>
            this.info().get(c.characterId)?.classSummary ??
            this.info().get(c.characterId)?.kindLabel ??
            '',
        )
      : [];
  });
  /** Why the offer cannot be made now (nobody on the other side can react), or `''`. */
  protected readonly offerWhy = computed(() => {
    const rows = this.offerRows();
    return rows.every((r) => r.spent || r.offered) ? 'Ninguém pode reagir agora.' : '';
  });
  /** One idempotency key per form and reactor: a repeated tap never offers twice, a new form is a new offer. */
  protected offerKeys = new Map<string, string>();
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
  /** What the form's end said, from the combat log's line (the server's number and reason): until it is touched. */
  protected readonly formEnded = signal<FormEnded | null>(null);
  protected readonly formNotice = computed(() => {
    const ended = this.formEnded();
    return ended ? formNoticeText(ended) : '';
  });
  protected readonly formNotices = new FormNotices();
  // ---- losing the concentration that holds the creatures (MR-037, E9-12 state 5) ----

  /** What the player's own concentration ending took with it: the spell and the creatures that left, until it is touched. */
  protected readonly lostConcentration = signal<LostConcentration | null>(null);
  protected readonly lostNotice = computed(() => {
    const lost = this.lostConcentration();
    return lost ? lostNoticeText(lost) : '';
  });
  protected readonly concentration = new ConcentrationWatch();
  /** On a phone, on the player's own turn, the order strip goes under the actions instead of between the title and them. */
  protected readonly orderBelow = computed(
    () => !this.laptop() && this.myTurn() && !this.ownDown() && !this.creatureTab(),
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
  /** Why "Próximo turno" waits while questions about hidden creatures are open ("Responda ao pedido abaixo para seguir."). */
  protected readonly waitWhy = computed(() => {
    const e = this.encounter();
    return e && this.isMaster() ? revealWhy(e) : '';
  });
  /** The NPC reactors' attacks with their numbers, by offer (read from the reactor's own options). */
  protected readonly reactorOptions = signal<ReadonlyMap<string, GetTurnOptionsResponse>>(
    new Map(),
  );
  protected readonly masterAttacks = computed<ReadonlyMap<string, readonly ReactorAttack[]>>(
    () =>
      new Map(
        this.toAnswer().map((o) => [
          o.id,
          reactorAttacks(
            o,
            (this.reactorOptions().get(o.id)?.options?.attacks ?? []).flatMap((a) =>
              a.attack ? [a.attack] : [],
            ),
          ),
        ]),
      ),
  );
  protected readonly offersHandled = new Set<string>();
  protected readonly reactorReading = new Set<string>();
  /** The offers whose reactor's attacks could not be read: the master's card offers "Tentar de novo". */
  protected readonly reactorFailed = signal<ReadonlySet<string>>(new Set());
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
    return e && this.isMaster()
      ? masterWaitNote(e, this.autoLeft(), (id) => this.info().get(id)?.playerName ?? '')
      : '';
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
  protected readonly dying = computed(() => (this.isMaster() ? dyingOf(this.encounter()) : []));
  /** The downed player characters whose death save is due this turn: the master rolls it when the player is away (RN-03). */
  protected readonly deathDue = computed(() =>
    this.isMaster() ? deathDueOf(this.encounter()) : [],
  );
  /** The player's hit whose damage is still to roll (the sheet was closed). */
  protected readonly ownPendingRoll = computed(() => pendingRollOf(this.options(), this.own()?.id));
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
  /** Reads an NPC reactor's own options (its attacks with their numbers) for the master's prompt. */
  protected async loadReactor(offer: OpportunityOffer): Promise<void> {
    const e = this.encounter();
    if (!e) {
      return;
    }
    this.reactorReading.add(offer.id);
    this.reactorFailed.update((s) => {
      const next = new Set(s);
      next.delete(offer.id);
      return next;
    });
    try {
      const res = await this.api.moves.turnOptions(this.campaignId(), e.id, offer.reactorId);
      this.reactorOptions.update((m) => new Map(m).set(offer.id, res));
    } catch {
      this.reactorFailed.update((s) => new Set(s).add(offer.id));
    } finally {
      this.reactorReading.delete(offer.id);
    }
  }
  /** The Bardic Inspiration die the player's character holds (the server sends it to the holder and the master only). */
  protected readonly ownDie = computed(() => this.own()?.inspirationDie ?? null);
  /** The attack roll of the player's character that waits for the answer about the die. */
  protected readonly ownOffer = computed(() => this.own()?.inspirationOffer ?? null);
  /** The attack roll of the player's halfling that waits for the Lucky answer ("Tirou 1: rolar de novo?"), asked first. */
  protected readonly ownLucky = computed(() => this.own()?.luckyOffer ?? null);
  /** The raging barbarian the turn waits for, when the question is mine to answer: the owner's, or the master's. */
  protected readonly rageAsked = computed(() => {
    const e = this.encounter();
    const id = e?.ragePendingCombatantId;
    const who = id ? e.combatants.find((c) => c.id === id) : undefined;
    return who && (this.isMaster() || who.mine) ? who : null;
  });
}
