import { create } from '@bufbuild/protobuf';
import {
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AttackOutcome,
  type AttackRoll,
  AttackRollSchema,
  type InspirationOffer,
  type LuckyOffer,
  type PendingDamage,
  type TargetInReach,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { EffectRollKind } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import type { Attack, BonusAttackRule } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import {
  type ExtraDieField,
  dieFields,
  genericDieFields,
  rollDiceOf,
} from '../../../../core/effects/effects';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import {
  type AttackStage,
  awaitsReaction,
  stageAfterRoll,
  stageOfAnswer,
  steps,
} from '../../../../core/combat/attack-flow';
import {
  type AttackDie,
  type DamageDie,
  type ModeChoice,
  type PairDie,
  CombatClient,
} from '../../../../core/combat/combat-client';
import { ActionKey } from '../../../../core/connect/idempotency';
import {
  type InspirationRoll,
  type LuckyRoll,
  ResourceClient,
} from '../../../../core/resources/resources-client';
import { damageFormula, extraDiceOf } from '../../../../core/combat/combat-dice';
import { brutalTyped, criticalSentence, criticalSum } from '../../../../core/combat/critical';
import {
  type ExtraPick,
  activeParts,
  initialPicks,
  typedFields,
} from '../../../../core/combat/damage-parts';
import { d20Count, d20Formula, modeStatus, orNormal } from '../../../../core/combat/roll-mode';
import { isTheatre } from '../../../../core/combat/theatre';
import { metersText } from '../../../../core/units';
import type { CombatState } from '../../../../core/combat/combat-state';
import { attackDetail, attackName, isCantrip } from '../../../../core/combat/combat-options';
import { combatantInitial, isPlayer } from '../../../../core/combat/combat-view';
import { isCreature } from '../../../../core/combat/creature-names';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import { AttackResult } from './attack-result';
import { AttackSteps } from './attack-steps';
import { DamageParts } from '../damage-parts/damage-parts';
import { MultiRoll, type RollField } from '../multi-roll/multi-roll';
import { RollModePicker } from '../roll-mode/roll-mode-picker';
import { InspirationPrompt } from '../inspiration-prompt/inspiration-prompt';
import { LuckyPrompt } from '../lucky-prompt/lucky-prompt';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { RollAnimator, type RollShow } from '../../../../shared/roll-overlay/roll-animator';
import { AttackQuestions } from './attack-questions';
import {
  EFFECT_DIE_FACES,
  afterDamageText,
  attackShow,
  attackTargetRows,
  brutalOf,
  damageDiceOf,
  damageHintOf,
  damageIntroOf,
  damageLabelOf,
  damageRangeOf,
  damageShow,
  extraDiceProblem,
  fixedTextOf,
  heldWait,
  modeChoiceOf,
  outcomeOf,
  rollRefusal,
  signedBonus,
  spentText,
  withAllies,
} from './attack-sheet-view';
import { ModeRequests } from './mode-requests';

/** What the page hands the attack sheet. */
export interface AttackSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly attackerId: string;
  readonly round: number;
  readonly attack: Attack;
  /** The attack's targets from `GetTurnOptions`, in turn order. */
  readonly targets: readonly TargetInReach[];
  /** The weapon attack of the extra action an effect gives (Velocidade): it spends that action, not the Attack action. */
  readonly useExtraAction?: boolean;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  /** Where each answer's combat goes (the page's copy of the combat). */
  readonly state: CombatState;
  /** An opportunity attack: a melee attack off turn that spends the reaction
   * instead of the action (`RollAttack.as_reaction`). */
  readonly asReaction?: boolean;
  /** The master rolls it (for an NPC): any roll mode, with no need to ask. */
  readonly master?: boolean;
  /** Extra Attack: how many attacks of the Attack action remain before this
   * one, and how many it makes. */
  readonly attacksLeft?: number;
  readonly attacksPerAction?: number;
  /** A bonus action attack (off hand, Artes Marciais, Rajada de Golpes): it
   * spends the bonus action, not the action; a Rajada de Golpes strike counts
   * down the `bonusAttacksLeft` the options had. */
  readonly bonusRule?: BonusAttackRule;
  readonly bonusAttacksLeft?: number;
  /** A cantrip with several beams (Eldritch Blast): how many were still to fire before this one. */
  readonly beamsLeft?: number;
  /** The answer to an opportunity offer (E9-13): the target is the mover (so the
   * sheet starts at "Rolar"), the reach is not checked (the attack comes right
   * before it leaves), and the roll names the offer. `byMaster` when the master
   * rolls it for an NPC. */
  readonly opportunity?: {
    readonly offerId: string;
    readonly targetId: string;
    readonly targetLabel: string;
    readonly byMaster: boolean;
  };
  /** A hit whose damage was never rolled (the sheet was closed): the sheet
   * opens at "Dano" with it. */
  readonly resume?: { readonly pending: PendingDamage; readonly targetLabel: string };
  /** A roll held for the Bardic Inspiration question (the player left the sheet, or read the screen again): the sheet
   * opens at the question, with the d20 that was rolled. */
  readonly inspiration?: { readonly offer: InspirationOffer; readonly targetLabel: string };
  /** The same for a roll held for the halfling's Lucky question ("Tirou 1: rolar de novo?"), which comes first. */
  readonly lucky?: { readonly offer: LuckyOffer; readonly targetLabel: string };
  /** A roll of this sheet was held for a question (Lucky or Bardic Inspiration): the page learns the hold, so it does not
   * open the question a second time. */
  readonly onHeld?: (holdId: string) => void;
  /** The monk's throw back after Defletir Projéteis: the reaction window that caught the missile
   * (`RollAttack.catch_window_id`). It is part of the same reaction, so it asks nothing more. */
  readonly catchWindowId?: string;
}

/**
 * "Atacar com Raio de Fogo" (MR-012, MR-014, E6-07, E6-08): the player's
 * attack in three steps, Alvo, Rolar and Dano, in a bottom sheet on a phone
 * and a dialog from a tablet up. It calls `RollAttack` and `RollDamage`
 * itself and hands each answer's combat to the page's `CombatState`. A
 * player never sees an armor class: the result says only "Acertou",
 * "Crítico" or "Errou". The two keys (one for the d20, one for the damage)
 * are made once, so a tap repeated after a lost answer never rolls twice.
 * The damage of a hit that closed before its roll comes back as `resume`.
 * Focus: the title opens first; after the result it goes to "Voltar à sua vez".
 */
@Component({
  selector: 'app-attack-sheet',
  imports: [
    AttackResult,
    AttackSteps,
    CombatantToken,
    DamageParts,
    ExtraDice,
    MatButtonModule,
    MatIconModule,
    MultiRoll,
    RollModePicker,
    InspirationPrompt,
    LuckyPrompt,
    MatButtonModule,
    MatIconModule,
    RollPicker,
  ],
  templateUrl: './attack-sheet.html',
  styleUrl: './attack-sheet.scss',
})
export class AttackSheet {
  private readonly api = inject(CombatClient);
  private readonly resources = inject(ResourceClient);
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<AttackSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly attack = this.data.attack;
  /** The held roll the sheet opens at (the screen was read again): its question, Lucky first. */
  private readonly reopened = this.data.lucky ?? this.data.inspiration;

  protected readonly stage = signal<AttackStage>(
    this.data.lucky
      ? 'lucky'
      : this.data.inspiration
        ? 'inspire'
        : this.data.resume
          ? 'damage'
          : this.data.opportunity
            ? 'roll'
            : 'target',
  );
  protected readonly targetId = signal<string | null>(
    this.data.opportunity?.targetId ?? this.reopened?.offer.targetId ?? null,
  );
  protected readonly typing = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly roll = signal<AttackRoll | null>(
    this.reopened
      ? create(AttackRollSchema, {
          attackerId: this.data.attackerId,
          targetId: this.reopened.offer.targetId,
          attackKey: this.reopened.offer.attackKey,
          d20: this.reopened.offer.d20,
        })
      : null,
  );
  /** The questions of a held roll (`AttackQuestions`): the Bardic Inspiration die's, and the halfling's Lucky about a 1,
   * asked before it. */
  private readonly questions = new AttackQuestions(
    {
      resources: this.resources,
      campaignId: this.data.campaignId,
      encounterId: this.data.encounterId,
      state: this.data.state,
      animator: this.animator,
      busy: this.busy,
      error: this.error,
      stage: this.stage,
      onHeld: this.data.onHeld,
      settle: (res) => this.settle(res),
    },
    { offer: this.data.inspiration?.offer ?? null, lucky: this.data.lucky?.offer ?? null },
  );
  protected readonly offer = this.questions.offer;
  protected readonly luckyOffer = this.questions.luckyOffer;
  /** The roll waits for an answer: neither a result nor the damage is told. */
  protected readonly asking = computed(
    () => this.stage() === 'lucky' || this.stage() === 'inspire',
  );
  protected readonly pending = signal<PendingDamage | null>(this.data.resume?.pending ?? null);
  /** The damage rolled, once it is. */
  protected readonly damage = signal<PendingDamage | null>(null);
  /** The mode the player asks the master for (`ModeRequests`): the pick, why, and the request as the combat shows it. */
  private readonly modes: ModeRequests = new ModeRequests({
    api: this.api,
    campaignId: this.data.campaignId,
    encounterId: this.data.encounterId,
    attackerId: this.data.attackerId,
    attackKey: this.attack.key,
    state: this.data.state,
    targetId: () => this.targetId(),
    chosen: (): RollMode => this.chosen(),
    busy: this.busy,
    error: this.error,
  });
  /** The mode the player picked; null follows the server's suggestion. */
  protected readonly picked = this.modes.picked;
  protected readonly reason = this.modes.reason;
  /** The extras marked in the damage step. */
  protected readonly picks = signal<readonly ExtraPick[]>([]);

  /** The d4 the effects on the attacker add to the roll (Bênção, Perdição), typed when the roll is from physical dice. */
  protected readonly extraFields = signal<readonly ExtraDieField[]>(
    dieFields(
      rollDiceOf(
        this.data.state.encounter()?.combatants.find((c) => c.id === this.data.attackerId)?.effects,
        EffectRollKind.ATTACK,
      ),
    ),
  );
  protected readonly extraFaces = signal<readonly (number | null)[]>([]);

  /** One key per attack roll (target and die): the same values again are a retry, others a new attack. */
  private readonly attackKeys = new ActionKey();
  /** One key per damage roll: the same die again is a retry, another die (typed after an app roll was lost) is a new request. */
  private readonly damageKeys = new ActionKey();
  private readonly body = viewChild<ElementRef<HTMLElement>>('body');
  private readonly back = viewChild('back', { read: ElementRef<HTMLButtonElement> });

  protected readonly name = attackName(this.attack);
  // An opportunity attack is a melee attack: a thrown dagger reads "corpo a
  // corpo" here, not its thrown range.
  // The Deflect Missiles throw back is a ranged attack of its own (20 ft, long range 60 ft).
  private readonly thrown = !!this.data.catchWindowId;
  protected readonly detail = this.thrown
    ? `Reação · ${attackDetail(this.attack)}`
    : this.data.asReaction
      ? `Reação · ${attackDetail({ ...this.attack, rangeFt: 5, longRangeFt: 0 })}`
      : `${this.data.useExtraAction ? 'Ação extra' : 'Ação'} · ${attackDetail(this.attack)}`;
  protected readonly cantrip = isCantrip(this.attack);
  protected readonly rangeText = metersText(
    this.thrown ? this.attack.longRangeFt : this.data.asReaction ? 5 : this.attack.rangeFt,
  );
  /** An opportunity attack reaches 5 ft, whatever range the weapon has when thrown. */
  protected readonly rows = computed(() =>
    withAllies(this.baseRows(), this.data.state.encounter()?.combatants ?? []),
  );
  private readonly baseRows = computed(() =>
    attackTargetRows({
      opportunity: this.data.opportunity,
      thrown: this.thrown,
      asReaction: this.data.asReaction ?? false,
      targets: this.data.targets,
      attack: this.attack,
    }),
  );
  /** The target's circumstances from the options: the suggested mode and why. An opportunity attack has none. */
  private readonly reach = computed(() => {
    const id = this.targetId();
    return this.data.targets.find((t) => t.combatantId === id) ?? null;
  });
  protected readonly hasMode = computed(() => !this.data.opportunity && this.reach() !== null);
  protected readonly suggested = computed(() =>
    orNormal(this.reach()?.rollMode ?? RollMode.NORMAL),
  );
  protected readonly sources = computed(() => this.reach()?.sources ?? []);
  protected readonly criticalOnHit = computed(() => this.reach()?.criticalOnHit ?? false);
  protected readonly approval = this.data.master ? 'free' : 'request';
  protected readonly chosen = computed<RollMode>(() => this.picked() ?? this.suggested());
  /** The request of this attack as the combat shows it now (waiting, answered), or none. */
  protected readonly request = this.modes.request;
  private readonly answered = this.modes.answered;
  /** The mode the d20 rolls with: the master's decision on a request, else the player's choice. */
  protected readonly rollMode = computed(() => this.answered()?.decidedMode ?? this.chosen());
  protected readonly modeState = computed(() =>
    modeStatus(this.approval, this.suggested(), this.chosen(), this.reason(), this.request()),
  );
  /** The dice may be rolled: the mode is settled (or the server picks it, for an opportunity attack). */
  protected readonly modeReady = computed(() => !this.hasMode() || this.modeState() === 'ready');
  protected readonly faceCount = computed(() => (this.hasMode() ? d20Count(this.rollMode()) : 1));
  protected readonly d20Fields = computed<readonly RollField[]>(() => [
    { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
  ]);
  protected readonly combine = computed(() =>
    this.rollMode() === RollMode.DISADVANTAGE ? 'lower' : 'higher',
  );
  protected readonly stepList = computed(() => steps(this.stage()));
  protected readonly target = computed(() => {
    const id = this.targetId();
    const row = this.rows().find((r) => r.id === id);
    return row ?? null;
  });
  protected readonly targetLabel = computed(
    () => this.target()?.label ?? this.data.resume?.targetLabel ?? this.reopened?.targetLabel ?? '',
  );
  protected readonly doneLabel = this.data.opportunity ? 'Fechar' : 'Voltar à sua vez';
  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;

  protected readonly outcome = computed(() => outcomeOf(this.roll(), this.asking()));
  protected readonly d20Formula = computed(() => {
    const r = this.roll()?.d20;
    return r ? d20Formula(r) : '';
  });
  /** The dice of the damage still to roll: "2d6" (doubled on a critical hit). */
  protected readonly dice = computed(() => damageDiceOf(this.pending()));
  /** The damage when a feature adds dice to the critical (Crítico Brutal): the screens say the groups and the feature's name. */
  protected readonly brutal = computed(() => brutalOf(this.pending()));
  /** "2d12 + 1d12 + 3" and "2d12 do crítico (dados dobrados) e 1d12 do Crítico Brutal (nível 9), mais 3 de modificador, de cortante.". */
  protected readonly brutalCard = computed(() => {
    const b = this.brutal();
    return b ? { sum: criticalSum(b), sentence: criticalSentence(b, b.damageTypePt) } : null;
  });
  protected readonly range = computed(() => damageRangeOf(this.brutal(), this.dice()));
  protected readonly parts = computed(() => this.pending()?.parts ?? []);
  /** One field for each group of dice of the damage that is marked, for physical dice. */
  protected readonly damageFields = computed<readonly RollField[]>(() =>
    typedFields(this.parts(), this.picks()).map((f) => ({
      key: f.key,
      label: f.label,
      min: f.min,
      max: f.max,
    })),
  );
  protected readonly damageFlat = computed(() =>
    activeParts(this.parts(), this.picks()).reduce((sum, p) => sum + p.flat, 0),
  );
  protected readonly damageLine = computed(() => {
    const d = this.damage();
    return d?.roll ? damageFormula(d.roll, d.damageTypePt, extraDiceOf(d), d.criticalMax) : '';
  });
  /** The typed sum's live total with the groups, only for the damage with extra dice. */
  protected readonly typedFormula = computed(() => {
    const b = this.brutal();
    return b
      ? (sum: number) => ({ text: brutalTyped(b, sum), total: sum + b.criticalMax + b.bonus })
      : null;
  });
  protected readonly totalNote = computed(() => {
    const b = this.brutal();
    return b && b.damageTypePt ? `de ${b.damageTypePt}` : 'Dano total';
  });
  protected readonly damageAppLabel = computed(() => {
    const d = this.dice();
    return this.brutal() ? 'Rolar dano no app' : `Rolar ${d?.name ?? ''} no app`;
  });
  protected readonly after = computed(() =>
    afterDamageText(this.damage(), this.data.state.encounter(), this.targetLabel()),
  );
  /** The d20 is rolled but a reaction holds the result: "Esperando o mestre. O resultado do seu ataque sai quando ele responder." */
  protected readonly held = computed(() => heldWait(this.roll(), this.data.state.encounter()));
  /** The hit is made, but its damage waits for the target's reaction (Escudo). */
  protected readonly waiting = computed(() => awaitsReaction(this.pending()));
  protected readonly defeated = computed(() => this.damage()?.targetDefeated ?? false);
  /** What the attack spent: the reaction, one of Extra Attack's attacks (the
   * action stays open for the rest) or the action. */
  protected readonly spent = computed(() =>
    spentText({
      byMaster: !!this.data.opportunity?.byMaster,
      asReaction: !!this.data.asReaction,
      bonusRule: this.data.bonusRule,
      bonusAttacksLeft: this.data.bonusAttacksLeft,
      cantrip: this.cantrip,
      name: this.name,
      beamsLeft: this.data.beamsLeft,
      attacksLeft: this.data.attacksLeft,
      attacksPerAction: this.data.attacksPerAction,
    }),
  );
  protected readonly title = computed(() =>
    this.typing() ? 'Digite o resultado do dado' : `Atacar com ${this.name}`,
  );
  protected readonly subtitle = computed(() =>
    this.typing()
      ? `${this.name} contra ${this.targetLabel()} · Rodada ${this.data.round}`
      : this.detail,
  );
  protected readonly rollLabel = computed(
    () => `Role 1d20 para ${this.name} (${signedBonus(this.attack.attackBonus)})`,
  );
  protected readonly damageLabel = computed(() =>
    damageLabelOf(this.brutal(), this.dice(), this.name),
  );
  protected readonly damageHint = computed(() =>
    damageHintOf(this.brutal(), this.pending(), this.range()),
  );
  /** What the typed sum is added to: the modifier and the critical's fixed maximum (the server's numbers, shown in the total before it is sent). */
  protected readonly damageModifier = computed(
    () => (this.pending()?.bonus ?? 0) + (this.pending()?.criticalMax ?? 0),
  );
  protected readonly fixedText = computed(() => fixedTextOf(this.brutal(), this.pending()));
  /** The critical's line only for whoever rolls physical dice (the app's dice are the server's). */
  protected readonly showCritical = computed(() => !this.canApp || this.typing());
  /** The line above the damage's dice: what a critical hit rolls under the table's rule ("role os dados duas vezes", "o máximo mais uma rolagem"), or "Acertou: role o dano.". */
  protected readonly damageIntro = computed(() =>
    damageIntroOf(this.pending(), this.outcome()?.crit),
  );
  /** A combat without a map: the master judges the reach, and the list says so instead of "Longe demais". */
  protected readonly theatre = computed(() => isTheatre(this.data.state.encounter()));

  constructor() {
    // After a result the focus goes to the one next action, as soon as it is drawn.
    effect(() => this.back()?.nativeElement.focus());
    // The extras that come marked are the starting choice of each damage.
    effect(() => {
      const parts = this.pending()?.parts ?? [];
      untracked(() => this.picks.set(initialPicks(parts)));
    });
    // The answer of a request in the air has to be shown: the sheet can't be dismissed meanwhile.
    effect(() => this.sheet.lock(this.busy()));
    // An error opens at the top of the scrolling body, where it is seen.
    effect(() => {
      if (this.error()) {
        this.body()?.nativeElement.scrollTo({ top: 0 });
      }
    });
    // A hit whose damage waits for the target's reaction (Escudo, answered by the master): the sheet follows the server
    // when the combat changes, so the hit turns into the miss, or into the damage to roll, instead of waiting for good.
    effect(() => {
      this.data.state.encounter();
      if (this.waiting()) {
        untracked(() => void this.followReaction());
      }
    });
    // A roll a reaction window held (Palavras de Interrupção) and a question for the attacker (a d20 of 1 of a halfling,
    // a Bardic Inspiration die it holds): once the window is answered the d20 is kept for the question, and the sheet that
    // waited for the window shows it.
    effect(() => {
      const waiting = this.roll()?.heldForReaction === true;
      const me = this.data.state.encounter()?.combatants.find((c) => c.id === this.data.attackerId);
      if (waiting && this.stage() === 'done') {
        untracked(() => this.questions.askAfterWindow(me?.luckyOffer, me?.inspirationOffer));
      }
    });
  }

  /** Reads the attacker's pending damages again: the hit that waited for a reaction is now a damage to roll, or gone
   * (the reaction stopped it: a miss). The player learns only that: never the armor class or the NPC's numbers (RN-10). */
  private async followReaction(): Promise<void> {
    const waiting = this.pending();
    if (!waiting) {
      return;
    }
    try {
      const res = await this.api.turnOptions(
        this.data.campaignId,
        this.data.encounterId,
        this.data.attackerId,
      );
      const now = res.pendingDamages.find((p) => p.id === waiting.id);
      if (this.pending()?.id !== waiting.id || awaitsReaction(now)) {
        return;
      }
      if (now) {
        this.pending.set(now);
        this.stage.set(stageAfterRoll(this.roll()?.outcome ?? AttackOutcome.HIT, now));
        return;
      }
      const r = this.roll();
      if (r) {
        // A protobuf-es message is a plain object: the same roll, now a miss.
        this.roll.set({ ...r, outcome: AttackOutcome.MISS });
      }
      this.pending.set(null);
      this.stage.set('done');
    } catch {
      // The sheet keeps what it shows; the next change of the combat asks again.
    }
  }

  protected initial(label: string): string {
    return combatantInitial(label);
  }

  protected npcTarget(id: string): boolean {
    const c = this.data.state.encounter()?.combatants.find((x) => x.id === id);
    return !!c && !isPlayer(c) && !isCreature(c);
  }

  protected creatureTarget(id: string): boolean {
    const c = this.data.state.encounter()?.combatants.find((x) => x.id === id);
    return !!c && isCreature(c);
  }

  protected pick(id: string): void {
    this.targetId.set(id);
    this.error.set('');
    this.modes.drop();
  }

  /** The target is chosen: on to the roll (a click, or Enter). */
  protected advance(): void {
    if (this.targetId() !== null) {
      this.stage.set('roll');
    }
  }

  protected change(): void {
    this.typing.set(false);
    this.stage.set('target');
    this.modes.drop();
  }

  protected setMode(mode: RollMode): void {
    this.picked.set(mode);
    this.error.set('');
  }

  /** "Pedir ao mestre": the mode asked for and why; the sheet waits for the answer. */
  protected askMaster(): Promise<void> {
    return this.modes.ask();
  }

  /** "Cancelar pedido": the player takes it back and picks again. */
  protected cancelAsk(): Promise<void> {
    return this.modes.cancel();
  }

  /** What the roll sends about the mode: nothing when the suggestion stands, else the mode, why, and the answered request. */
  private modeChoice(): ModeChoice | undefined {
    return modeChoiceOf({
      hasMode: this.hasMode(),
      answered: this.answered(),
      chosen: this.chosen(),
      suggested: this.suggested(),
      reason: this.reason(),
    });
  }

  protected async rollAttack(die: AttackDie | PairDie): Promise<void> {
    const id = this.targetId();
    if (id === null || this.busy()) {
      return;
    }
    const extra = this.typedExtra(die);
    if (extra === null) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const mode = this.modeChoice();
      const res = await this.api.rollAttack(
        this.data.campaignId,
        this.data.encounterId,
        this.data.attackerId,
        this.attack.key,
        id,
        die,
        this.attackKeys.keyFor({ id, die, mode, extra, spend: this.data.useExtraAction }),
        this.data.asReaction ?? false,
        this.data.opportunity?.offerId ?? '',
        mode,
        this.data.catchWindowId ?? '',
        { extraDieFaces: extra, useExtraAction: this.data.useExtraAction ?? false },
      );
      this.data.state.apply(res.encounter);
      this.settle(res);
      this.play(attackShow(this.name, res));
    } catch (err) {
      this.fail(err);
    } finally {
      this.busy.set(false);
    }
  }

  private play(show: RollShow | null): void {
    if (show) {
      this.animator.play(show);
    }
  }

  /** What a roll answered: the result and, for a hit, the damage; or, for a held roll, the d20 and its question (Lucky
   * about a 1 first, then the Bardic Inspiration die): no result is told before the answer. */
  private settle(res: {
    readonly roll: AttackRoll;
    readonly pending: PendingDamage | undefined;
    readonly offer?: InspirationOffer;
    readonly lucky?: LuckyOffer;
  }): void {
    this.roll.set(res.roll);
    this.pending.set(res.pending ?? null);
    this.offer.set(res.offer ?? null);
    this.luckyOffer.set(res.lucky ?? null);
    const held = res.lucky ?? res.offer;
    if (held) {
      this.data.onHeld?.(held.holdId);
    }
    this.typing.set(false);
    this.stage.set(stageOfAnswer(res));
  }

  /** "Rolar de novo" or "Ficar com o 1" (`AttackQuestions`). */
  protected answerLucky(reroll: boolean, die: LuckyRoll | null): Promise<void> {
    return this.questions.answerLucky(reroll, die);
  }

  /** "Somar o d8" or "Guardar o dado" (`AttackQuestions`). */
  protected answerInspiration(use: boolean, die: InspirationRoll | null): Promise<void> {
    return this.questions.answerInspiration(use, die);
  }

  protected rollAttackTyped(face: number): Promise<void> {
    return this.rollAttack({ face });
  }

  /** Two physical d20, in the order they were rolled. */
  protected rollAttackPair(faces: number[]): Promise<void> {
    return this.rollAttack({ faces });
  }

  protected async rollDamage(
    die: DamageDie | { readonly parts: true },
    typedParts?: { partKey: string; sum: number }[],
  ): Promise<void> {
    const p = this.pending();
    if (!p || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      // With parts the answer is the whole choice, the extras marked (even none) and the sums typed.
      const choice =
        this.parts().length > 0
          ? { extras: this.picks(), ...(typedParts ? { typedParts } : {}) }
          : undefined;
      const res = await this.api.rollDamage(
        this.data.campaignId,
        this.data.encounterId,
        p.id,
        die,
        this.damageKeys.keyFor({ die, choice }),
        choice,
      );
      this.data.state.apply(res.encounter);
      this.damage.set(res.pending);
      this.play(damageShow(res.pending));
      this.typing.set(false);
      this.stage.set('done');
    } catch (err) {
      this.fail(err);
    } finally {
      this.busy.set(false);
    }
  }

  protected rollDamageTyped(sum: number): Promise<void> {
    return this.rollDamage({ sum });
  }

  /** The sums typed for each group of dice, in the order of the fields. */
  protected rollDamageParts(sums: number[]): Promise<void> {
    const fields = this.damageFields();
    return this.rollDamage(
      { parts: true },
      fields.map((f, i) => ({ partKey: f.key, sum: sums[i] })),
    );
  }

  private fail(err: unknown): void {
    const refusal = rollRefusal(err);
    if (refusal.more > 0) {
      this.extraFields.set(genericDieFields(refusal.more, EFFECT_DIE_FACES));
    }
    this.error.set(refusal.message);
  }

  /** The faces of the d4 typed for a roll from physical dice; none for the app's dice, `null` while one is missing (the sheet says so). */
  private typedExtra(die: AttackDie | PairDie): readonly number[] | null {
    const fields = this.extraFields();
    if ('inApp' in die || fields.length === 0) {
      return [];
    }
    const problem = extraDiceProblem(fields, this.extraFaces());
    if (problem) {
      this.error.set(problem);
      return null;
    }
    return this.extraFaces() as readonly number[];
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(this.stage() === 'done');
  }
}
