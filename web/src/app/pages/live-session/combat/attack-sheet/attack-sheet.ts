import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  untracked,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import { awaitsReaction, steps } from '../../../../core/combat/attack-flow';
import { CombatClient } from '../../../../core/combat/combat-client';
import {
  type AttackDie,
  type DamageDie,
  type PairDie,
} from '../../../../core/combat/combat-client-types';
import { attackDetail, attackName, isCantrip } from '../../../../core/combat/combat-options';
import { combatantInitial, isPlayer } from '../../../../core/combat/combat-view';
import { isCreature } from '../../../../core/combat/creature-names';
import { d20Formula } from '../../../../core/combat/roll-mode';
import { isTheatre } from '../../../../core/combat/theatre';
import { initialPicks } from '../../../../core/combat/damage-parts';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { metersText } from '../../../../core/units';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { injectSheet } from '../../../../shared/sheet/sheet-host';
import { ExtraDice } from '../../effects/extra-dice/extra-dice';
import { DamageParts } from '../damage-parts/damage-parts';
import { InspirationPrompt } from '../inspiration-prompt/inspiration-prompt';
import { LuckyPrompt } from '../lucky-prompt/lucky-prompt';
import { MultiRoll, type RollField } from '../multi-roll/multi-roll';
import { RollModePicker } from '../roll-mode/roll-mode-picker';
import { AttackDamage } from './attack-damage';
import { AttackMode } from './attack-mode';
import { AttackQuestions } from './attack-questions';
import { AttackResult } from './attack-result';
import { AttackRolls } from './attack-rolls';
import type { AttackSheetData } from './attack-sheet-types';
import {
  attackTargetRows,
  heldWait,
  outcomeOf,
  signedBonus,
  spentText,
  withAllies,
} from './attack-sheet-view';
import { AttackState } from './attack-state';
import { AttackSteps } from './attack-steps';
import { ModeRequests } from './mode-requests';

/** An opportunity attack is a melee attack: it reaches 5 ft, whatever range the weapon has when thrown. */
const REACTION_REACH_FT = 5;

export type { AttackSheetData } from './attack-sheet-types';

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
  changeDetection: ChangeDetectionStrategy.OnPush,
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
    RollPicker,
  ],
  templateUrl: './attack-sheet.html',
  styleUrl: './attack-sheet.scss',
})
export class AttackSheet {
  private readonly api = inject(CombatClient);
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<AttackSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly attack = this.data.attack;
  protected readonly s = new AttackState(this.data);
  protected readonly name = attackName(this.attack);

  /** The questions of a held roll (`AttackQuestions`): the Bardic Inspiration die's, and the halfling's Lucky about a 1,
   * asked before it. */
  protected readonly questions = new AttackQuestions(
    {
      resources: inject(ResourceClient),
      campaignId: this.data.campaignId,
      encounterId: this.data.encounterId,
      state: this.data.state,
      animator: this.animator,
      busy: this.s.busy,
      error: this.s.error,
      stage: this.s.stage,
      onHeld: this.data.onHeld,
      settle: (res) => this.rolls.settle(res),
    },
    { offer: this.data.inspiration?.offer ?? null, lucky: this.data.lucky?.offer ?? null },
  );
  /** The mode the player asks the master for (`ModeRequests`): the pick, why, and the request as the combat shows it. */
  protected readonly modes: ModeRequests = new ModeRequests({
    api: this.api,
    campaignId: this.data.campaignId,
    encounterId: this.data.encounterId,
    attackerId: this.data.attackerId,
    attackKey: this.attack.key,
    state: this.data.state,
    targetId: () => this.s.targetId(),
    chosen: () => this.mode.chosen(),
    busy: this.s.busy,
    error: this.s.error,
  });
  protected readonly mode = new AttackMode(this.s, this.modes);
  protected readonly outcome = computed(() => outcomeOf(this.s.roll(), this.s.asking()));
  protected readonly dmg = new AttackDamage(
    this.s,
    this.name,
    this.data.diceMode !== DiceMode.PHYSICAL,
    computed(() => this.outcome()?.crit),
    computed(() => this.targetLabel()),
  );
  private readonly rolls: AttackRolls = new AttackRolls(this.s, this.dmg, this.questions, {
    api: this.api,
    animator: this.animator,
    attack: this.attack,
    name: this.name,
    modeChoice: () => this.mode.choice(),
  });
  protected readonly extraFields = this.rolls.extraFields;
  protected readonly extraFaces = this.rolls.extraFaces;
  private readonly body = viewChild<ElementRef<HTMLElement>>('body');
  private readonly back = viewChild('back', { read: ElementRef<HTMLButtonElement> });

  // An opportunity attack is a melee attack: a thrown dagger reads "corpo a corpo"
  // here, not its thrown range.
  // The Deflect Missiles throw back is a ranged attack of its own (20 ft, long range 60 ft).
  private readonly thrown = !!this.data.catchWindowId;
  protected readonly detail = this.thrown
    ? `Reação · ${attackDetail(this.attack)}`
    : this.data.asReaction
      ? `Reação · ${attackDetail({ ...this.attack, rangeFt: REACTION_REACH_FT, longRangeFt: 0 })}`
      : `${this.data.useExtraAction ? 'Ação extra' : 'Ação'} · ${attackDetail(this.attack)}`;
  protected readonly cantrip = isCantrip(this.attack);
  protected readonly rangeText = metersText(
    this.thrown
      ? this.attack.longRangeFt
      : this.data.asReaction
        ? REACTION_REACH_FT
        : this.attack.rangeFt,
  );
  /** An opportunity attack reaches 5 ft, whatever range the weapon has when thrown. */
  protected readonly rows = computed(() =>
    withAllies(
      attackTargetRows({
        opportunity: this.data.opportunity,
        thrown: this.thrown,
        asReaction: this.data.asReaction ?? false,
        targets: this.data.targets,
        attack: this.attack,
      }),
      this.data.state.encounter()?.combatants ?? [],
    ),
  );
  protected readonly d20Fields: readonly RollField[] = [
    { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
  ];
  protected readonly stepList = computed(() => steps(this.s.stage()));
  protected readonly target = computed(() => {
    const id = this.s.targetId();
    return this.rows().find((r) => r.id === id) ?? null;
  });
  protected readonly targetLabel = computed(
    () =>
      this.target()?.label ?? this.data.resume?.targetLabel ?? this.s.reopened?.targetLabel ?? '',
  );
  protected readonly doneLabel = this.data.opportunity ? 'Fechar' : 'Voltar à sua vez';
  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;
  protected readonly d20Formula = computed(() => {
    const r = this.s.roll()?.d20;
    return r ? d20Formula(r) : '';
  });
  /** The d20 is rolled but a reaction holds the result: "Esperando o mestre. O resultado do seu ataque sai quando ele responder." */
  protected readonly held = computed(() => heldWait(this.s.roll(), this.data.state.encounter()));
  /** The hit is made, but its damage waits for the target's reaction (Escudo). */
  protected readonly waiting = computed(() => awaitsReaction(this.s.pending()));
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
    this.s.typing() ? 'Digite o resultado do dado' : `Atacar com ${this.name}`,
  );
  protected readonly subtitle = computed(() =>
    this.s.typing()
      ? `${this.name} contra ${this.targetLabel()} · Rodada ${this.data.round}`
      : this.detail,
  );
  protected readonly rollLabel = computed(
    () => `Role 1d20 para ${this.name} (${signedBonus(this.attack.attackBonus)})`,
  );
  /** A combat without a map: the master judges the reach, and the list says so instead of "Longe demais". */
  protected readonly theatre = computed(() => isTheatre(this.data.state.encounter()));

  constructor() {
    const { s } = this;
    // After a result the focus goes to the one next action, as soon as it is drawn.
    effect(() => this.back()?.nativeElement.focus());
    // The extras that come marked are the starting choice of each damage.
    effect(() => {
      const parts = s.pending()?.parts ?? [];
      untracked(() => s.picks.set(initialPicks(parts)));
    });
    // The answer of a request in the air has to be shown: the sheet can't be dismissed meanwhile.
    effect(() => this.sheet.lock(s.busy()));
    // An error opens at the top of the scrolling body, where it is seen.
    effect(() => {
      if (s.error()) {
        this.body()?.nativeElement.scrollTo({ top: 0 });
      }
    });
    // A hit whose damage waits for the target's reaction (Escudo, answered by the master): the sheet follows the server
    // when the combat changes, so the hit turns into the miss, or into the damage to roll, instead of waiting for good.
    effect(() => {
      this.data.state.encounter();
      if (this.waiting()) {
        untracked(() => void this.rolls.followReaction());
      }
    });
    // A roll a reaction window held (Palavras de Interrupção) and a question for the attacker (a d20 of 1 of a halfling,
    // a Bardic Inspiration die it holds): once the window is answered the d20 is kept for the question, and the sheet that
    // waited for the window shows it.
    effect(() => {
      const waiting = s.roll()?.heldForReaction === true;
      const me = this.data.state.encounter()?.combatants.find((c) => c.id === this.data.attackerId);
      if (waiting && s.stage() === 'done') {
        untracked(() => this.questions.askAfterWindow(me?.luckyOffer, me?.inspirationOffer));
      }
    });
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
    this.s.targetId.set(id);
    this.s.error.set('');
    this.modes.drop();
  }

  /** The target is chosen: on to the roll (a click, or Enter). */
  protected advance(): void {
    if (this.s.targetId() !== null) {
      this.s.stage.set('roll');
    }
  }

  protected change(): void {
    this.s.typing.set(false);
    this.s.stage.set('target');
    this.modes.drop();
  }

  protected busy(): boolean {
    return this.s.busy();
  }

  protected rollAttack(die: AttackDie | PairDie): Promise<void> {
    return this.rolls.rollAttack(die);
  }

  protected rollAttackTyped(face: number): Promise<void> {
    return this.rolls.rollAttack({ face });
  }

  /** Two physical d20, in the order they were rolled. */
  protected rollAttackPair(faces: number[]): Promise<void> {
    return this.rolls.rollAttack({ faces });
  }

  protected rollDamage(
    die: DamageDie | { readonly parts: true },
    typedParts?: { partKey: string; sum: number }[],
  ): Promise<void> {
    return this.rolls.rollDamage(die, typedParts);
  }

  protected rollDamageTyped(sum: number): Promise<void> {
    return this.rolls.rollDamage({ sum });
  }

  protected rollDamageParts(sums: number[]): Promise<void> {
    return this.rolls.rollDamageParts(sums);
  }

  protected close(): void {
    if (this.s.busy()) {
      return;
    }
    this.sheet.close(this.s.stage() === 'done');
  }
}
