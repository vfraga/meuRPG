import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { RouterLink } from '@angular/router';
import { creatureSlug } from '../../../../core/creatures/bestiary-format';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatSelectModule } from '@angular/material/select';
import { MatIconModule } from '@angular/material/icon';

import {
  type Combatant,
  type Encounter,
  type GetTurnOptionsResponse,
  type TargetInReach,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { type Attack, type Creature } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CreaturesClient } from '../../../../core/creatures/creatures-client';
import { CombatUndone } from '../../../../core/combat/combat-undone';
import { CombatClient } from '../../../../core/combat/combat-client';
import { newKey } from '../../../../core/connect/idempotency';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import { ofThe } from '../../../../core/combat/move-plan';
import { standUpRow } from '../../../../core/combat/stand-up';
import { cannotAct, conditionTags } from '../../../../core/combat/conditions';
import { CombatantTags } from '../combatant-tags/combatant-tags';
import { RageStatus } from '../rage-end/rage-status';
import type { CombatState } from '../../../../core/combat/combat-state';
import { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import {
  combatantInitial,
  isDown,
  isPlayer,
  roundLabel,
} from '../../../../core/combat/combat-view';
import { joinDots } from '../../../../core/format/text';
import { defaultTarget, isHostile } from '../../../../core/combat/attack-flow';
import { isCreature } from '../../../../core/combat/creature-names';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import { Portrait } from '../../../../shared/portrait/portrait';
import { MultiRoll } from '../../../../shared/multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { MasterSpend } from '../theatre/master-spend';
import { RollModePicker } from '../roll-mode/roll-mode-picker';
import { AttackChoice } from './attack-choice';
import { CreatureActions, type CreatureAreaRequest } from './creature-actions';
import { NpcRollMode } from './npc-mode';
import {
  D20_FIELDS,
  type LastRoll,
  areaSpellsOf,
  attackShow,
  coverNoteOf,
  distanceText,
  pairHintOf,
  resultOf,
  rollLabelOf,
  rollWhyOf,
  rollableAttacks,
  statsOf,
} from './npc-card-view';
import { attacksMade, routinesOf } from './multiattack';
import { PendingDamages } from './pending-damages';

/**
 * The master's card of the one on turn (E6-11, E6-12): "Ações do Capitão
 * Goblin" with its PV, CA and speed, the economy it spent, the attack (radio
 * rows), the target with the distance and "Rolar ataque" (the app rolls it,
 * or the master types a value). The result shows the armor class the roll
 * was compared with ("Acertou contra CA 18 do Toren"), then the damage to
 * roll and apply (`PendingDamages`). The one filled button of the screen
 * stays "Próximo turno". When a player is on turn the card has no attack
 * form, only the damage their hits left for the master to apply. An NPC's
 * portrait (MR-031) stands at the left of the title, or its initials when it
 * has none; the order list and the map keep their tokens.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-npc-card',
  imports: [
    AttackChoice,
    CombatantTags,
    CombatantToken,
    CreatureActions,
    MasterSpend,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatSelectModule,
    PendingDamages,
    Portrait,
    RageStatus,
    MultiRoll,
    RollModePicker,
    RollPicker,
    RouterLink,
  ],
  templateUrl: './npc-card.html',
  styleUrl: './npc-card.scss',
})
export class NpcCard {
  private readonly api = inject(CombatClient);
  private readonly animator = inject(RollAnimator);
  private readonly undone = inject(CombatUndone);
  private readonly creatures = inject(CreaturesClient);

  readonly campaignId = input.required<string>();
  readonly encounter = input.required<Encounter>();
  /** The one on turn (a player's character too: only its pending damage). */
  readonly subject = input.required<Combatant>();
  readonly options = input<GetTurnOptionsResponse | null>(null);
  readonly state = input.required<CombatState>();
  /** The card has its own accent frame (the theatre's, in the middle of the page); inside the rail it is bare. */
  readonly framed = input(true);
  /** The newest attack of this one was stopped by the target's Escudo (from the log). */
  readonly reactionStopped = input(false);
  /** The page's own call (the turn passing) is in flight. */
  readonly turnBusy = input(false);
  /** The combat is played without a map (RN-25): movement is a number and the opportunity attack is the master's offer. */
  readonly theatre = input(false);
  /** The offer's form is open: "Oferecer ataque de oportunidade" says so. */
  readonly offering = input(false);
  /** Why "Oferecer ataque de oportunidade" cannot be used now ("Ninguém pode reagir agora."), or `''`. */
  readonly offerWhy = input('');
  /** "Oferecer ataque de oportunidade": the page opens the form. */
  readonly offer = output<void>();
  /** "Conjurar" on an NPC's area spell placed on the map: the page opens the master's picker (PM-02d state 11). */
  readonly castArea = output<string>();
  /** "Usar" on a creature's action with a shape, on a map: the page opens the picker of the area and answers with what the action did. */
  readonly creatureArea = output<CreatureAreaRequest>();
  /** A damage was applied or discarded (`PendingDamages`): the page keeps the card for its note. */
  readonly settledNote = output<void>();
  /** "Encerrar fúria": the id of the raging combatant. */
  readonly endRage = output<string>();

  protected readonly attackKey = signal('');
  protected readonly targetId = signal('');
  /** The master chose the target himself: a move of the creature does not take it away. */
  private pickedByHand = false;
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** The master is typing the d20: the roll takes the whole row. */
  protected readonly typing = signal(false);
  private readonly picker = viewChild(RollPicker);
  private readonly multi = viewChild(MultiRoll);
  /** The d20 just rolled: shown with the armor class, until the turn changes. */
  protected readonly last = signal<LastRoll | null>(null);
  private key = newKey();
  /** The creature's stat block, read once per kind (the client keeps it): its Multiattack routines. `null` until read, and when it cannot be. */
  protected readonly block = signal<Creature | null>(null);
  protected readonly creatureDamageIds = signal<readonly string[]>([]);
  /** The routine the master picked, when the creature has a choice between several. */
  protected readonly routineIdx = signal(0);
  /** "Encerrar os ataques": the master ended the routine before its last attack; the action counts as used. */
  protected readonly ended = signal(false);
  /** The reaction spell's name, remembered from the prompt that waited (it is gone once answered). */
  protected readonly shieldName = signal('Escudo Arcano');

  protected readonly conditions = computed(() => conditionTags(this.subject()));
  protected readonly round = computed(() => roundLabel(this.encounter().round));
  protected readonly initial = computed(() => combatantInitial(this.subject().label));
  protected readonly isCreatureSubject = computed(() => isCreature(this.subject()));
  /** The creature's route segment when the subject is a monster of the bestiary ("bandit"); the key only reaches the master (RN-29). */
  protected readonly creatureSlug = computed(() =>
    creatureSlug(this.subject().bestiaryCreatureKey ?? ''),
  );
  /** "Goblin · ND 1/4": what it is, under the title; empty for a character or a creature of a player. */
  protected readonly identity = computed(() => {
    const c = this.subject();
    const nd = c.challengeRating ? `ND\u00a0${c.challengeRating}` : '';
    return joinDots([c.monsterNamePt, nd].filter(Boolean));
  });
  protected readonly isNpc = computed(
    () => !isPlayer(this.subject()) && !isCreature(this.subject()),
  );
  /** "do Goblin", "da Brisa". */
  protected readonly ofLabel = computed(() => ofThe([this.subject().label]));
  protected readonly down = computed(() => isDown(this.subject()));
  /** The card with the stats, the economy and the movement: an NPC's, and in a combat without a map also a player's (the master spends their movement when they are away). */
  protected readonly full = computed(
    () => this.isNpc() || (this.theatre() && !this.isCreatureSubject()),
  );
  protected readonly attacks = computed<Attack[]>(() => rollableAttacks(this.options()));
  protected readonly attack = computed(
    () => this.attacks().find((a) => a.key === this.attackKey()) ?? null,
  );
  protected readonly targets = computed(
    () =>
      this.options()?.attackTargets.find((t) => t.attackKey === this.attackKey())?.targets ?? [],
  );
  /** The creatures hostile to the one on turn: the party against the enemies, by the sides of the combat. */
  private readonly foeIds = computed(() => {
    const side = this.subject().side;
    return new Set(
      this.encounter()
        .combatants.filter((c) => isHostile(side, c.side))
        .map((c) => c.id),
    );
  });
  protected readonly targetLabel = computed(
    () => this.targets().find((t) => t.combatantId === this.targetId())?.label ?? '',
  );
  protected readonly pendings = computed(() => this.options()?.pendingDamages ?? []);
  /** The NPC's spells whose area is placed on the map (none without a map): each one a "Conjurar" for the master's picker. */
  protected readonly areaSpells = computed(() => areaSpellsOf(this.options(), this.theatre()));
  /** A hit whose damage is still to roll or to apply holds the attacker's next attack: the server refuses it too. */
  protected readonly rollWhy = computed(() => rollWhyOf(this.options(), this.pendings().length));
  /**
   * Every condition `rollAttack` checks, in one place: the picker's buttons are off exactly when a roll would be refused,
   * so a "Confirmar" that looks enabled is never dropped (the attack, the target and the reason can change under the card
   * when the turn's options are read again, e.g. right after the turn passes).
   */
  protected readonly canRoll = computed(
    () =>
      !!this.attack() && !!this.targetId() && !this.busy() && !this.rollWhy() && this.mode.ready(),
  );
  /** The damage of the attack just rolled shows inside its result box. */
  protected readonly inBox = computed(() => {
    const id = this.last()?.pending?.id;
    return this.pendings().filter((p) => p.id === id);
  });
  protected readonly others = computed(() => {
    const id = this.last()?.pending?.id;
    return this.pendings().filter((p) => p.id !== id && !this.creatureDamageIds().includes(p.id));
  });
  /** The routines of the creature's Multiattack that this NPC can make; none for a creature without one (the card is then as it was). */
  protected readonly routines = computed(() => routinesOf(this.block(), this.attacks()));
  protected readonly routine = computed(
    () => this.routines()[Math.min(this.routineIdx(), this.routines().length - 1)] ?? null,
  );
  /** The attacks of the Attack action made this turn, from the count the server keeps. */
  protected readonly made = computed(() => {
    const eco = this.options()?.options?.economy;
    return attacksMade(eco?.attacksPerAction, eco?.attacksLeft);
  });
  /** The routine is done: its last attack was made, or the master ended it. */
  protected readonly routineDone = computed(() => {
    const r = this.routine();
    return !!r && (this.ended() || this.made() >= r.total);
  });
  /** Some attacks of the routine were made and more may be: the action is under way, not used. */
  protected readonly underWay = computed(
    () => !!this.routine() && this.made() > 0 && !this.routineDone(),
  );
  /** The "Ação" chip: its word and whether it reads as spent. */
  protected readonly actionChip = computed(() => {
    const r = this.routine();
    if (r && this.underWay()) {
      return { used: false, word: `${this.made()} de ${r.total}` };
    }
    const used = r && this.routineDone() ? true : this.subject().actionUsed;
    return { used, word: used ? 'Usada' : 'Disponível' };
  });
  protected readonly stats = computed(() => statsOf(this.subject(), this.theatre()));
  protected readonly result = computed(() =>
    resultOf(
      this.last(),
      this.encounter(),
      this.reactionStopped(),
      this.shieldName(),
      this.attacks(),
    ),
  );
  /** How the d20 rolls: the server's suggestion for the target and the master's choice (the player's dialog has the same). */
  protected readonly mode = new NpcRollMode(
    () => this.targets().find((t) => t.combatantId === this.targetId()) ?? null,
  );
  protected readonly faceCount = this.mode.faceCount;
  protected readonly d20Fields = D20_FIELDS;
  protected readonly combine = this.mode.combine;
  protected readonly pairHint = computed(() => pairHintOf(this.combine()));
  protected readonly rollLabel = computed(() => rollLabelOf(this.attack()));

  constructor() {
    // Each new turn starts clean, with the first attack and the first target.
    let turnOf = '';
    effect(() => {
      const id = this.subject().id;
      if (id !== turnOf) {
        turnOf = id;
        untracked(() => {
          this.pickedByHand = false;
          this.last.set(null);
          this.error.set('');
        });
      }
    });
    // "Desfazer última ação" took back an action: the result card of the last roll may be that action, so it goes.
    let undoneSeen = this.undone.count();
    effect(() => {
      const n = this.undone.count();
      if (n !== undoneSeen) {
        undoneSeen = n;
        untracked(() => {
          this.last.set(null);
          this.ended.set(false);
        });
      }
    });
    // The creature's Multiattack: read its stat block once per kind.
    effect(() => {
      const key = this.subject().bestiaryCreatureKey ?? '';
      const campaignId = this.campaignId();
      untracked(() => {
        this.block.set(null);
        this.routineIdx.set(0);
        if (!key) {
          return;
        }
        this.creatures.statBlock(campaignId, key).then(
          (block) => {
            if (untracked(() => this.subject().bestiaryCreatureKey) === key) {
              this.block.set(block);
            }
          },
          () => undefined, // no routine to show: the card is as it was
        );
      });
    });
    // A new turn (no attack counted) starts the routine again.
    effect(() => {
      if (this.made() === 0) {
        untracked(() => this.ended.set(false));
      }
    });
    effect(() => {
      const keys = this.attacks().map((a) => a.key);
      if (!keys.includes(this.attackKey())) {
        this.attackKey.set(keys[0] ?? '');
        this.key = newKey();
      }
    });
    // The target the attack starts on: the nearest hostile creature in reach, then the nearest out of reach, never an ally while
    // another creature exists. It follows the creature when it moves (the distances change), until the master picks one.
    effect(() => {
      const targets = this.targets();
      const foes = this.foeIds();
      untracked(() => {
        const current = this.targetId();
        if (this.pickedByHand && targets.some((t) => t.combatantId === current)) {
          return;
        }
        this.pickedByHand = false;
        const next = defaultTarget(targets, (id) => foes.has(id));
        if (next !== current) {
          this.targetId.set(next);
          this.mode.reset();
          this.key = newKey();
        }
      });
    });
  }

  protected pickAttack(key: string): void {
    this.attackKey.set(key);
    this.mode.reset();
    this.key = newKey();
  }

  protected pickRoutine(index: number): void {
    this.routineIdx.set(index);
  }

  /** "Encerrar os ataques": the rest of the routine is not made; the action counts as used. */
  protected endRoutine(): void {
    this.ended.set(true);
  }

  protected pickTarget(id: string): void {
    this.pickedByHand = true;
    this.targetId.set(id);
    this.mode.reset();
    this.key = newKey();
  }

  protected coverNote(t: TargetInReach): string {
    return coverNoteOf(t);
  }

  protected distance(ft: number | undefined, tooFar: boolean): string {
    return distanceText(ft, tooFar);
  }

  protected rollApp(): Promise<void> {
    return this.rollAttack({ inApp: true });
  }

  protected rollTyped(face: number): Promise<void> {
    return this.rollAttack({ face });
  }

  /** The two d20 of an advantage or a disadvantage, in the order they were rolled. */
  protected rollTypedPair(faces: number[]): Promise<void> {
    return this.rollAttack({ faces });
  }

  /** "Levantar-se" of a prone card (half the speed; the master may for anyone on turn). An incapacitated one is not offered it: Riso Histérico keeps it prone. */
  protected readonly standRow = computed(() =>
    cannotAct(this.subject()) ? null : standUpRow(this.subject()),
  );

  protected async standUp(): Promise<void> {
    if (this.busy() || this.turnBusy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      this.state().apply(
        await this.api.moves.standUp(this.campaignId(), this.encounter().id, this.subject().id),
      );
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'levantar'));
    } finally {
      this.busy.set(false);
    }
  }

  private async rollAttack(
    die: { inApp: true } | { face: number } | { faces: number[] },
  ): Promise<void> {
    const a = this.attack();
    const target = this.targetId();
    if (this.busy()) {
      return; // the roll already on its way answers
    }
    if (this.rollWhy()) {
      return; // its reason is already written under the roll (#roll-why)
    }
    if (!a || !target) {
      // Never silent: the options changed between the tap and now (the button state follows them a frame later).
      this.error.set(
        'O ataque ou o alvo mudou enquanto você confirmava. Confira e confirme de novo.',
      );
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.attacks.rollAttack(
        this.campaignId(),
        this.encounter().id,
        this.subject().id,
        a.key,
        target,
        die,
        this.key,
        false,
        '',
        this.mode.choice(),
      );
      this.key = newKey();
      this.state().apply(res.encounter);
      this.last.set({ roll: res.roll, pending: res.pending ?? null, subject: this.subject().id });
      const show = attackShow(a, res.roll);
      if (show) {
        this.animator.play(show);
      }
      this.mode.reset();
      this.picker()?.reset();
      this.multi()?.reset();
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'rolar o ataque'));
    } finally {
      this.busy.set(false);
    }
  }
}
