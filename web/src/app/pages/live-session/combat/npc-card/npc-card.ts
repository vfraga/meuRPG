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
import { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { type Attack } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatUndone } from '../../../../core/combat/combat-undone';
import { CombatClient } from '../../../../core/combat/combat-client';
import { newKey } from '../../../../core/connect/idempotency';
import { d20Count, orNormal } from '../../../../core/combat/roll-mode';
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
import { isCreature } from '../../../../core/combat/creature-names';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import { Portrait } from '../../../../shared/portrait/portrait';
import { NextTurn } from '../combat-bar/next-turn';
import { MultiRoll } from '../multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { MasterSpend } from '../theatre/master-spend';
import { TheatrePill } from '../theatre/theatre-pill';
import { AttackChoice } from './attack-choice';
import {
  D20_FIELDS,
  type LastRoll,
  areaSpellsOf,
  attackShow,
  coverNoteOf,
  distanceText,
  modeLinesOf,
  pairHintOf,
  resultOf,
  rollLabelOf,
  rollWhyOf,
  rollableAttacks,
  statsOf,
} from './npc-card-view';
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
    MasterSpend,
    MatButtonModule,
    TheatrePill,
    MatFormFieldModule,
    MatIconModule,
    MatSelectModule,
    NextTurn,
    PendingDamages,
    Portrait,
    RageStatus,
    MultiRoll,
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

  readonly campaignId = input.required<string>();
  readonly encounter = input.required<Encounter>();
  /** The one on turn (a player's character too: only its pending damage). */
  readonly subject = input.required<Combatant>();
  readonly options = input<GetTurnOptionsResponse | null>(null);
  readonly state = input.required<CombatState>();
  /** On a phone or a tablet the card is the whole turn (E6-12): its title, the
   * warning that damage is owed and "Próximo turno" at its end. */
  readonly narrow = input(false);
  /** A joint turn is running: the turn passes when the last part ends, so no "Próximo turno". */
  readonly joint = input(false);
  /** What the turn still owes ("Falta aplicar 5 de dano"), or `null`. */
  readonly pendingNote = input<string | null>(null);
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
  /** A question about hidden creatures holds the turn: why "Próximo turno" waits ("Responda ao pedido abaixo para seguir."), or `''`. */
  readonly waitWhy = input('');
  /** "Oferecer ataque de oportunidade": the page opens the form. */
  readonly offer = output<void>();
  /** "Conjurar" on an NPC's area spell placed on the map: the page opens the master's picker (PM-02d state 11). */
  readonly castArea = output<string>();
  /** "Próximo turno"; `true` when the master passes it with a damage waiting. */
  readonly next = output<boolean>();
  /** A damage was applied or discarded (`PendingDamages`): the page keeps the card for its note. */
  readonly settledNote = output<void>();
  /** "Encerrar fúria": the id of the raging combatant. */
  readonly endRage = output<string>();

  protected readonly turnKey = computed(
    () => `${this.encounter().currentCombatantId}:${this.encounter().round}`,
  );
  protected readonly attackKey = signal('');
  protected readonly targetId = signal('');
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** The master is typing the d20: the roll takes the whole row. */
  protected readonly typing = signal(false);
  private readonly picker = viewChild(RollPicker);
  private readonly multi = viewChild(MultiRoll);
  /** The d20 just rolled: shown with the armor class, until the turn changes. */
  protected readonly last = signal<LastRoll | null>(null);
  private key = newKey();
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
  protected readonly isNpc = computed(
    () => !isPlayer(this.subject()) && !isCreature(this.subject()),
  );
  /** The one on turn is at 0 hit points: nobody spends movement for them. */
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
    () => !!this.attack() && !!this.targetId() && !this.busy() && !this.rollWhy(),
  );
  /** The damage of the attack just rolled shows inside its result box. */
  protected readonly inBox = computed(() => {
    const id = this.last()?.pending?.id;
    return this.pendings().filter((p) => p.id === id);
  });
  protected readonly others = computed(() => {
    const id = this.last()?.pending?.id;
    return this.pendings().filter((p) => p.id !== id);
  });
  protected readonly stats = computed(() => statsOf(this.subject(), this.theatre()));
  protected readonly result = computed(() =>
    resultOf(this.last(), this.encounter(), this.reactionStopped(), this.shieldName()),
  );
  /** The mode the server works out for this attack on this target (advantage, disadvantage, normal): the d20 the master
   * types follow it, one face or two, as the player's own sheet does (SRD 5.1, "Advantage and Disadvantage"). */
  protected readonly rollMode = computed(() =>
    orNormal(
      this.targets().find((t) => t.combatantId === this.targetId())?.rollMode ?? RollMode.NORMAL,
    ),
  );
  /** Why the roll has advantage or disadvantage ("Vantagem: atacante não visto"), so the master knows which die counts and why. */
  protected readonly modeLines = computed(() =>
    modeLinesOf(
      this.rollMode(),
      this.targets().find((t) => t.combatantId === this.targetId())?.sources ?? [],
    ),
  );
  protected readonly faceCount = computed(() => d20Count(this.rollMode()));
  protected readonly d20Fields = D20_FIELDS;
  protected readonly combine = computed(() =>
    this.rollMode() === RollMode.DISADVANTAGE ? 'lower' : 'higher',
  );
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
        untracked(() => this.last.set(null));
      }
    });
    effect(() => {
      const keys = this.attacks().map((a) => a.key);
      if (!keys.includes(this.attackKey())) {
        this.attackKey.set(keys[0] ?? '');
        this.key = newKey();
      }
    });
    effect(() => {
      const ids = this.targets().map((t) => t.combatantId);
      if (!ids.includes(this.targetId())) {
        this.targetId.set(ids[0] ?? '');
        this.key = newKey();
      }
    });
  }

  protected pickAttack(key: string): void {
    this.attackKey.set(key);
    this.key = newKey();
  }

  protected pickTarget(id: string): void {
    this.targetId.set(id);
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
      );
      this.key = newKey();
      this.state().apply(res.encounter);
      this.last.set({ roll: res.roll, pending: res.pending ?? null, subject: this.subject().id });
      const show = attackShow(a, res.roll);
      if (show) {
        this.animator.play(show);
      }
      this.picker()?.reset();
      this.multi()?.reset();
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'rolar o ataque'));
    } finally {
      this.busy.set(false);
    }
  }
}
