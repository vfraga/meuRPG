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
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AttackOutcome,
  type AttackRoll,
  AttackRollSchema,
  CombatantSide,
  type InspirationOffer,
  type LuckyOffer,
  type PendingDamage,
  PendingDamageStatus,
  type TargetInReach,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  type RollModeRequest,
  RollMode,
  RollModeRequestStatus,
} from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { EffectRollKind } from '../../../../../gen/meurpg/play/v1/lasting_effects_pb';
import type { Attack, BonusAttackRule } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import {
  type ExtraDieField,
  dieFields,
  genericDieFields,
  missingDice,
  rollDiceOf,
} from '../../../../core/effects/effects';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import {
  type AttackStage,
  awaitsReaction,
  isHit,
  missMark,
  outcomeWord,
  stageAfterRoll,
  stageOfAnswer,
  steps,
  targetAfter,
  targetRows,
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
import { classResourceErrorMessage } from '../../../../core/resources/resources-errors';
import {
  damageFormula,
  diceName,
  extraDiceOf,
  sumRange,
} from '../../../../core/combat/combat-dice';
import {
  brutalLabel,
  brutalTyped,
  brutalTypedHint,
  criticalHint,
  criticalSentence,
  criticalSum,
  criticalTypedHint,
  fixedParts,
  hasExtraDice,
  typedRange,
} from '../../../../core/combat/critical';
import {
  type ExtraPick,
  activeParts,
  initialPicks,
  typedFields,
} from '../../../../core/combat/damage-parts';
import {
  d20Count,
  d20Formula,
  liveRequest,
  modeStatus,
  orNormal,
} from '../../../../core/combat/roll-mode';
import { reactionWait } from '../../../../core/combat/reactions';
import { isTheatre } from '../../../../core/combat/theatre';
import { metersText } from '../../../../core/units';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import {
  attackDetail,
  attackName,
  beamSpentText,
  bonusSpentText,
  isCantrip,
} from '../../../../core/combat/combat-options';
import { article } from '../../../../core/combat/combat-log';
import { joinDots, tight } from '../../../../core/format/text';
import { combatantInitial, isPlayer, stateWord } from '../../../../core/combat/combat-view';
import { isCreature } from '../../../../core/combat/creature-names';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import { AttackResult } from './attack-result';
import { AttackSteps } from './attack-steps';
import { DamageParts } from '../damage-parts/damage-parts';
import { MultiRoll, type RollField } from '../multi-roll/multi-roll';
import { RollModePicker } from '../roll-mode/roll-mode-picker';
import { InspirationPrompt } from '../inspiration-prompt/inspiration-prompt';
import { LuckyPrompt } from '../lucky-prompt/lucky-prompt';
import { LUCKY_TRAIT } from '../../../../core/combat/lucky';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { RollAnimator, showOfDice, showOfDie } from '../../../../shared/roll-overlay/roll-animator';

/** The d4 of Bênção and Perdição. */
const EFFECT_DIE_FACES = 4;

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
  /** The Bardic Inspiration question of a held roll: the d20 is rolled, the result waits for the answer. */
  protected readonly offer = signal<InspirationOffer | null>(this.data.inspiration?.offer ?? null);
  /** The Lucky question of a held roll whose d20 shows a 1: asked before the die's. */
  protected readonly luckyOffer = signal<LuckyOffer | null>(this.data.lucky?.offer ?? null);
  /** The roll waits for an answer: neither a result nor the damage is told. */
  protected readonly asking = computed(
    () => this.stage() === 'lucky' || this.stage() === 'inspire',
  );
  protected readonly pending = signal<PendingDamage | null>(this.data.resume?.pending ?? null);
  /** The damage rolled, once it is. */
  protected readonly damage = signal<PendingDamage | null>(null);
  /** The mode the player picked; null follows the server's suggestion. */
  protected readonly picked = signal<RollMode | null>(null);
  protected readonly reason = signal('');
  /** The request this sheet sent to the master (its state is read from the combat). */
  private readonly asked = signal<RollModeRequest | null>(null);
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
  /** One key per request to the master: the same mode and reason again is a retry. */
  private readonly requestKeys = new ActionKey();
  /** One key per answer about the die (use it with this die, or keep it): the same answer again is a retry. */
  private readonly answerKeys = new ActionKey();
  /** One key per Lucky answer (roll again this way, or keep the 1): the same answer again is a retry. */
  private readonly luckyKeys = new ActionKey();
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
  protected readonly rows = computed(() => this.withAllies(this.baseRows()));
  /** An ally is tagged ("Aliada"), so a table of friends does not misread the list. */
  private withAllies(rows: ReturnType<typeof targetRows>): ReturnType<typeof targetRows> {
    const e = this.data.state.encounter();
    return rows.map((r) => {
      const c = e?.combatants.find((x) => x.id === r.id);
      const ally =
        c && (isPlayer(c) || c.side === CombatantSide.PARTY)
          ? article(c.label) === 'a'
            ? 'Aliada'
            : 'Aliado'
          : '';
      return ally ? { ...r, sub: tight(joinDots([r.sub, ally].filter(Boolean))) } : r;
    });
  }
  private readonly baseRows = computed(() =>
    this.data.opportunity
      ? [
          {
            id: this.data.opportunity.targetId,
            label: this.data.opportunity.targetLabel,
            sub: '',
            blocked: '',
            cover: '',
            coverMark: null,
          },
        ]
      : this.thrown
        ? targetRows(
            this.data.targets.map(
              (t) =>
                ({
                  ...t,
                  tooFar:
                    t.distanceFt === undefined ? t.tooFar : t.distanceFt > this.attack.longRangeFt,
                }) as typeof t,
            ),
            this.attack.longRangeFt,
          )
        : this.data.asReaction
          ? targetRows(
              this.data.targets.map(
                (t) =>
                  ({ ...t, tooFar: t.distanceFt === undefined || t.distanceFt > 5 }) as typeof t,
              ),
              5,
            )
          : targetRows(this.data.targets, this.attack.rangeFt),
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
  protected readonly chosen = computed(() => this.picked() ?? this.suggested());
  /** The request of this attack as the combat shows it now (waiting, answered), or none. */
  protected readonly request = computed(() => {
    const sent = this.asked();
    if (!sent) {
      return null;
    }
    const all = this.data.state.encounter()?.rollModeRequests ?? [];
    const live = liveRequest(all, sent.id);
    return live ?? (all.some((r) => r.id === sent.id) ? null : sent);
  });
  private readonly answered = computed(() => {
    const r = this.request();
    return r?.status === RollModeRequestStatus.ANSWERED ? r : null;
  });
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

  protected readonly outcome = computed(() => {
    const r = this.roll();
    // A held roll has no result yet: neither a word nor a pill (the master has not said, or a reaction holds it).
    return r && !r.heldForReaction && !this.asking()
      ? {
          word: outcomeWord(r.outcome),
          hit: isHit(r.outcome),
          crit: r.outcome === AttackOutcome.CRITICAL_HIT,
          natural: missMark(r),
        }
      : null;
  });
  protected readonly d20Formula = computed(() => {
    const r = this.roll()?.d20;
    return r ? d20Formula(r) : '';
  });
  /** The dice of the damage still to roll: "2d6" (doubled on a critical hit). */
  protected readonly dice = computed(() => {
    const p = this.pending();
    return p
      ? {
          count: p.diceCount,
          sides: p.diceSides,
          bonus: p.bonus,
          name: diceName(p.diceCount, p.diceSides),
        }
      : null;
  });
  /** The damage when a feature adds dice to the critical (Crítico Brutal): the screens say the groups and the feature's name. */
  protected readonly brutal = computed(() => {
    const p = this.pending();
    return p && hasExtraDice(p) ? p : null;
  });
  /** "2d12 + 1d12 + 3" and "2d12 do crítico (dados dobrados) e 1d12 do Crítico Brutal (nível 9), mais 3 de modificador, de cortante.". */
  protected readonly brutalCard = computed(() => {
    const b = this.brutal();
    return b ? { sum: criticalSum(b), sentence: criticalSentence(b, b.damageTypePt) } : null;
  });
  protected readonly range = computed(() => {
    const b = this.brutal();
    if (b) {
      return typedRange(b);
    }
    const d = this.dice();
    return d ? sumRange(d.count, d.sides) : { min: 1, max: 1 };
  });
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
  protected readonly after = computed(() => {
    const d = this.damage();
    if (!d) {
      return '';
    }
    const enc = this.data.state.encounter();
    const target = enc?.combatants.find((c) => c.id === d.targetId);
    return targetAfter(
      this.targetLabel(),
      !!target && isPlayer(target),
      d,
      target ? stateWord(target.state) : '',
    );
  });
  /** The d20 is rolled but a reaction holds the result: "Esperando o mestre. O resultado do seu ataque sai quando ele responder." */
  protected readonly held = computed(() => {
    const r = this.roll();
    const e = this.data.state.encounter();
    return r?.heldForReaction
      ? ((e ? reactionWait(e) : null) ?? {
          title: 'Esperando o mestre',
          detail: 'O resultado do seu ataque sai quando ele responder.',
        })
      : null;
  });
  /** The hit is made, but its damage waits for the target's reaction (Escudo). */
  protected readonly waiting = computed(() => awaitsReaction(this.pending()));
  protected readonly defeated = computed(() => this.damage()?.targetDefeated ?? false);
  /** What the attack spent: the reaction, one of Extra Attack's attacks (the
   * action stays open for the rest) or the action. */
  protected readonly spent = computed(() => {
    if (this.data.opportunity?.byMaster) {
      return 'A reação dele foi usada.';
    }
    if (this.data.asReaction) {
      return 'Sua reação foi usada.';
    }
    const bonus = bonusSpentText(this.data.bonusRule, this.data.bonusAttacksLeft);
    if (bonus) {
      return bonus;
    }
    if (this.cantrip && (this.data.beamsLeft ?? 0) > 1) {
      return beamSpentText(this.name, this.data.beamsLeft ?? 0);
    }
    const left = (this.data.attacksLeft ?? 1) - 1;
    if (!this.cantrip && (this.data.attacksPerAction ?? 1) > 1 && left > 0) {
      return `Você ainda tem ${left} ${left === 1 ? 'ataque' : 'ataques'} desta ação.`;
    }
    return 'Sua ação foi usada.';
  });
  protected readonly title = computed(() =>
    this.typing() ? 'Digite o resultado do dado' : `Atacar com ${this.name}`,
  );
  protected readonly subtitle = computed(() =>
    this.typing()
      ? `${this.name} contra ${this.targetLabel()} · Rodada ${this.data.round}`
      : this.detail,
  );
  protected readonly rollLabel = computed(
    () => `Role 1d20 para ${this.name} (${this.signedBonus()})`,
  );
  protected readonly damageLabel = computed(() => {
    const b = this.brutal();
    if (b) {
      return brutalLabel(b, `${article(this.name)} ${this.name}`);
    }
    const d = this.dice();
    return d && d.count > 1
      ? `Role ${d.name} para o dano: some os dois`
      : `Role ${d?.name ?? ''} para o dano`;
  });
  protected readonly damageHint = computed(() => {
    const b = this.brutal();
    if (b) {
      return brutalTypedHint(b);
    }
    const r = this.range();
    const p = this.pending();
    return criticalTypedHint(
      p?.criticalRule ?? 0,
      p ? diceName(p.diceCount, p.diceSides) : '',
      r.min,
      r.max,
      p?.criticalMax ?? 0,
    );
  });
  /** What the typed sum is added to: the modifier and the critical's fixed maximum (the server's numbers, shown in the total before it is sent). */
  protected readonly damageModifier = computed(
    () => (this.pending()?.bonus ?? 0) + (this.pending()?.criticalMax ?? 0),
  );
  protected readonly fixedText = computed(() => {
    const b = this.brutal();
    if (b) {
      // The maximum stands in the live total's groups; the field's note is the modifier alone.
      return b.bonus === 0 ? '' : `${b.bonus < 0 ? '−' : '+'} ${Math.abs(b.bonus)} de bônus`;
    }
    return fixedParts(this.pending()?.criticalMax ?? 0, this.pending()?.bonus ?? 0);
  });
  /** The critical's line only for whoever rolls physical dice (the app's dice are the server's). */
  protected readonly showCritical = computed(() => !this.canApp || this.typing());
  /** The line above the damage's dice: what a critical hit rolls under the table's rule ("role os dados duas vezes", "o máximo mais uma rolagem"), or "Acertou: role o dano.". */
  protected readonly damageIntro = computed(() => {
    const p = this.pending();
    if (!p) {
      return '';
    }
    const hint = criticalHint(p.criticalRule, p.diceCount, p.diceSides, p.criticalMax);
    if (hint) {
      return hint.line;
    }
    return p.critical || this.outcome()?.crit
      ? `Acerto crítico: os dados do dano dobram (${diceName(p.diceCount, p.diceSides)}).`
      : 'Acertou: role o dano.';
  });
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
        untracked(() => this.askAfterWindow(me?.luckyOffer, me?.inspirationOffer));
      }
    });
  }

  /** The question the combat now carries for the roll a reaction window held: Lucky first, then the die's. */
  private askAfterWindow(lucky: LuckyOffer | undefined, offer: InspirationOffer | undefined): void {
    const held = lucky ?? offer;
    if (!held) {
      return;
    }
    this.luckyOffer.set(lucky ?? null);
    this.offer.set(offer ?? null);
    this.data.onHeld?.(held.holdId);
    this.stage.set(lucky ? 'lucky' : 'inspire');
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

  private signedBonus(): string {
    const b = this.attack.attackBonus;
    return `${b < 0 ? '−' : '+'}${Math.abs(b)}`;
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
    this.dropMode();
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
    this.dropMode();
  }

  /** A new target starts from its own suggestion; a request for the old one is taken back. */
  private dropMode(): void {
    const sent = this.asked();
    this.picked.set(null);
    this.reason.set('');
    this.asked.set(null);
    if (sent && sent.status === RollModeRequestStatus.PENDING) {
      void this.api
        .cancelRollModeRequest(
          this.data.campaignId,
          this.data.encounterId,
          sent.id,
          this.requestKeys.keyFor({ cancel: sent.id }),
        )
        .then((e) => this.data.state.apply(e))
        .catch(() => undefined);
    }
  }

  protected setMode(mode: RollMode): void {
    this.picked.set(mode);
    this.error.set('');
  }

  /** "Pedir ao mestre": the mode asked for and why; the sheet waits for the answer. */
  protected async askMaster(): Promise<void> {
    const id = this.targetId();
    if (id === null || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const mode = this.chosen();
      const reason = this.reason().trim();
      const res = await this.api.requestRollMode(
        this.data.campaignId,
        this.data.encounterId,
        this.data.attackerId,
        this.attack.key,
        id,
        mode,
        reason,
        this.requestKeys.keyFor({ id, mode, reason }),
      );
      this.data.state.apply(res.encounter);
      this.asked.set(res.request);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'pedir ao mestre'));
    } finally {
      this.busy.set(false);
    }
  }

  /** "Cancelar pedido": the player takes it back and picks again. */
  protected async cancelAsk(): Promise<void> {
    const sent = this.asked();
    if (!sent || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const e = await this.api.cancelRollModeRequest(
        this.data.campaignId,
        this.data.encounterId,
        sent.id,
        this.requestKeys.keyFor({ cancel: sent.id }),
      );
      this.data.state.apply(e);
      this.asked.set(null);
      this.picked.set(null);
      this.reason.set('');
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'cancelar o pedido'));
    } finally {
      this.busy.set(false);
    }
  }

  /** What the roll sends about the mode: nothing when the suggestion stands, else the mode, why, and the answered request. */
  private modeChoice(): ModeChoice | undefined {
    if (!this.hasMode()) {
      return undefined;
    }
    const r = this.answered();
    if (r) {
      return {
        mode: r.decidedMode,
        reason: r.decidedMode === r.suggestedMode ? '' : r.reason,
        requestId: r.id,
      };
    }
    return this.chosen() === this.suggested()
      ? undefined
      : { mode: this.chosen(), reason: this.reason().trim() };
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
      this.animate(res);
    } catch (err) {
      this.fail(err);
    } finally {
      this.busy.set(false);
    }
  }

  /**
   * The d20 the app just rolled, tumbling (RN-20): the face, and the total and the word only where this sheet shows them
   * (a roll held for a reaction or for the Inspiração de Bardo question shows the face alone). Never an armor class.
   */
  private animate(res: { readonly roll: AttackRoll; readonly offer?: InspirationOffer }): void {
    const r = res.roll;
    const told = !res.offer && !r.heldForReaction && r.outcome !== AttackOutcome.UNSPECIFIED;
    const hit = isHit(r.outcome);
    const show = showOfDice(`Ataque com ${this.name}`, r.d20, {
      withTotal: !r.heldForReaction,
      outcome: told ? { word: outcomeWord(r.outcome), good: hit } : undefined,
      critical: told && r.outcome === AttackOutcome.CRITICAL_HIT,
      fumble: told && !hit,
    });
    if (show) {
      this.animator.play(show);
    }
  }

  /** The damage dice the app just rolled, as the result step shows them: the faces, the roll's total and the damage type. */
  private animateDamage(p: PendingDamage | undefined): void {
    if (!p || p.status === PendingDamageStatus.AWAITING_REACTION) {
      return;
    }
    const show = showOfDice('Dano', p.roll, { withTotal: true, note: p.damageTypePt || undefined });
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

  /** "Rolar de novo" (rolled in the app, or the typed face) or "Ficar com o 1": the answer resolves the held attack, or
   * moves on to the Bardic Inspiration question on the new roll when the attacker holds a die. */
  protected async answerLucky(reroll: boolean, die: LuckyRoll | null): Promise<void> {
    const offer = this.luckyOffer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.resources.answerLucky(
        this.data.campaignId,
        this.data.encounterId,
        offer.holdId,
        reroll,
        die,
        this.luckyKeys.keyFor({ holdId: offer.holdId, reroll, die }),
      );
      this.luckyKeys.renew();
      this.data.state.apply(res.encounter);
      this.settle(res);
      if (die && 'inApp' in die) {
        this.animateLucky(res.roll);
      }
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder ao Sortudo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The d20 the app rolled again for Lucky, tumbling: the new face alone, "1 → 14" (the sheet tells the result). */
  private animateLucky(roll: AttackRoll): void {
    const again = roll.d20?.rerolled.find((r) => r.sourceKey === LUCKY_TRAIT);
    const show = again
      ? showOfDie('Sortudo', roll.d20?.diceSides ?? 0, again.to, `${again.from} → ${again.to}`)
      : null;
    if (show) {
      this.animator.play(show);
    }
  }

  /** "Somar o d8" (rolled in the app, or the typed face) or "Guardar o dado": the answer finishes the held attack. */
  protected async answerInspiration(use: boolean, die: InspirationRoll | null): Promise<void> {
    const offer = this.offer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.resources.answerBardicInspiration(
        this.data.campaignId,
        this.data.encounterId,
        offer.holdId,
        use,
        die,
        this.answerKeys.keyFor({ holdId: offer.holdId, use, die }),
      );
      this.answerKeys.renew();
      this.data.state.apply(res.encounter);
      this.settle(res);
      // The die the player added after the d20, rolled in the app (the sheet's "Com o d8 da Inspiração de Bardo (+5)" line).
      const bonus =
        use && die && 'inApp' in die ? res.roll.bonusDice.find((b) => b.used) : undefined;
      const show = bonus
        ? showOfDie('Inspiração de Bardo', bonus.sides, bonus.face, `+${bonus.face}`)
        : null;
      if (show) {
        this.animator.play(show);
      }
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder à Inspiração de Bardo'));
    } finally {
      this.busy.set(false);
    }
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
      this.animateDamage(res.pending);
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
    // Physical dice and an effect the app did not list (one the master keeps from the players): the server says how many
    // more d4 the roll takes, and the sheet asks for them.
    const connectErr = ConnectError.from(err, Code.Unavailable);
    const more = connectErr.code === Code.InvalidArgument ? missingDice(connectErr.rawMessage) : 0;
    if (more > 0) {
      this.extraFields.set(genericDieFields(more, EFFECT_DIE_FACES));
      this.error.set(
        more === 1
          ? 'Esta rolagem leva mais um d4: role-o e digite o resultado.'
          : `Esta rolagem leva mais ${more} d4: role-os e digite os resultados.`,
      );
      return;
    }
    this.error.set(combatErrorMessage(err, 'rolar o ataque'));
  }

  /** The faces of the d4 typed for a roll from physical dice; none for the app's dice, `null` while one is missing (the sheet says so). */
  private typedExtra(die: AttackDie | PairDie): readonly number[] | null {
    const fields = this.extraFields();
    if ('inApp' in die || fields.length === 0) {
      return [];
    }
    const faces = this.extraFaces();
    if (faces.length !== fields.length || faces.some((f) => f === null)) {
      this.error.set(
        fields.length === 1
          ? `Digite o resultado do d${fields[0].faces} antes de confirmar.`
          : 'Digite o resultado de cada dado extra antes de confirmar.',
      );
      return null;
    }
    return faces as readonly number[];
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(this.stage() === 'done');
  }
}
