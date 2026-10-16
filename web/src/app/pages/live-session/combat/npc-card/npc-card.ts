import {
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
  AreaPlacement,
  AttackOutcome,
  type AttackRoll,
  type Combatant,
  type Encounter,
  type GetTurnOptionsResponse,
  type PendingDamage,
  type TargetInReach,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { DisabledReasonCode, type Attack } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { isHit, outcomeWord } from '../../../../core/combat/attack-flow';
import { CombatUndone } from '../../../../core/combat/combat-undone';
import { CombatClient, newKey } from '../../../../core/combat/combat-client';
import { coverText } from '../../../../core/combat/cover';
import { d20Count, modeWord, orNormal, sourceLine } from '../../../../core/combat/roll-mode';
import { rollFormula } from '../../../../core/combat/combat-dice';
import { article } from '../../../../core/combat/combat-log';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import { metersFixed, reachSquares, squaresText } from '../../../../core/units';
import { restamText } from '../../../../core/combat/theatre';
import { ofThe } from '../../../../core/combat/move-plan';
import { joinDots } from '../../../../core/format/text';
import { standUpRow } from '../../../../core/combat/stand-up';
import { cannotAct, conditionTags } from '../../../../core/combat/conditions';
import { CombatantTags } from '../combatant-tags/combatant-tags';
import { RageStatus } from '../rage-end/rage-status';
import type { CombatState } from '../../../../core/combat/combat-state';
import { attackName, reasonText } from '../../../../core/combat/combat-options';
import { RollAnimator, showOfDice } from '../../../../shared/roll-overlay/roll-animator';
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
import { MultiRoll, type RollField } from '../multi-roll/multi-roll';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { MasterSpend } from '../theatre/master-spend';
import { TheatrePill } from '../theatre/theatre-pill';
import { AttackChoice } from './attack-choice';
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
  protected readonly last = signal<{
    roll: AttackRoll;
    pending: PendingDamage | null;
    subject: string;
  } | null>(null);
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
  protected readonly attacks = computed<Attack[]>(() =>
    (this.options()?.options?.attacks ?? []).flatMap((a) =>
      a.attack && a.attack.saveDc === 0 ? [a.attack] : [],
    ),
  );
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
  protected readonly areaSpells = computed(() => {
    const opts = this.options();
    if (this.theatre() || !opts?.options) {
      return [];
    }
    return (opts.options.spells ?? []).flatMap((s) => {
      const t = (opts.spellTargets ?? []).find((x) => x.spellKey === s.spell?.key);
      return s.spell && s.enabled && t && t.placement !== AreaPlacement.UNSPECIFIED
        ? [{ key: s.spell.key, name: s.spell.namePt || s.spell.name }]
        : [];
    });
  });
  /** A hit whose damage is still to roll or to apply holds the attacker's next attack: the server refuses it too. */
  protected readonly rollWhy = computed(() => {
    if (this.pendings().length > 0) {
      return 'Role ou aplique o dano do ataque anterior antes de rolar outro.';
    }
    // The server turns every option off for a combatant that cannot act (incapacitated, lethargy): say so here too.
    const off = (this.options()?.options?.attacks ?? []).find(
      (a) =>
        !a.enabled &&
        (a.reason?.code === DisabledReasonCode.INCAPACITATED ||
          a.reason?.code === DisabledReasonCode.EFFECT_LETHARGY),
    );
    return off ? reasonText(off.reason, true) : '';
  });
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
  protected readonly stats = computed(() => {
    const c = this.subject();
    return {
      hp:
        c.hitPointsMax !== undefined ? { now: c.hitPointsCurrent ?? 0, max: c.hitPointsMax } : null,
      ac: c.armorClass,
      speed: {
        meters: metersFixed(c.speedDft / 10),
        squares: this.theatre() ? '' : squaresText(reachSquares(c.speedFt)),
      },
      movement: this.theatre()
        ? restamText(c.movementLeftDft)
        : joinDots([
            metersFixed(c.movementLeftDft / 10),
            squaresText(reachSquares(c.movementLeftFt)),
          ]),
    };
  });
  protected readonly result = computed(() => {
    const l = this.last();
    if (!l) {
      return null;
    }
    const target = this.encounter().combatants.find((c) => c.id === l.roll.targetId);
    // Escudo can turn a hit into a miss after the roll (the master's answer, or the player's).
    const stopped = this.reactionStopped();
    return {
      total: l.roll.d20?.total ?? 0,
      formula: l.roll.d20 ? rollFormula(l.roll.d20) : '',
      physical: l.roll.d20?.physical ?? false,
      // A reaction holds the roll (Palavras de Interrupção): no outcome until it is answered.
      held: l.roll.heldForReaction,
      word: l.roll.heldForReaction
        ? 'Esperando a reação'
        : stopped
          ? `Errou: o ${this.shieldName()} segurou`
          : outcomeWord(l.roll.outcome),
      hit: !l.roll.heldForReaction && !stopped && isHit(l.roll.outcome),
      against:
        l.roll.targetArmorClass !== undefined
          ? `contra CA ${l.roll.targetArmorClass} ${article(target?.label ?? '') === 'a' ? 'da' : 'do'} ${target?.label ?? ''}`
          : '',
    };
  });
  /** The mode the server works out for this attack on this target (advantage, disadvantage, normal): the d20 the master
   * types follow it, one face or two, as the player's own sheet does (SRD 5.1, "Advantage and Disadvantage"). */
  protected readonly rollMode = computed(() =>
    orNormal(
      this.targets().find((t) => t.combatantId === this.targetId())?.rollMode ?? RollMode.NORMAL,
    ),
  );
  /** Why the roll has advantage or disadvantage ("Vantagem: atacante não visto"), so the master knows which die counts and why. */
  protected readonly modeLines = computed(() => {
    const mode = this.rollMode();
    if (mode === RollMode.NORMAL) {
      return [];
    }
    const sources = this.targets().find((t) => t.combatantId === this.targetId())?.sources ?? [];
    return sources.length
      ? sources.map((src) => sourceLine(src))
      : [`${modeWord(mode)} neste ataque.`];
  });
  protected readonly faceCount = computed(() => d20Count(this.rollMode()));
  protected readonly d20Fields: readonly RollField[] = [
    { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
    { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
  ];
  protected readonly combine = computed(() =>
    this.rollMode() === RollMode.DISADVANTAGE ? 'lower' : 'higher',
  );
  protected readonly pairHint = computed(
    () =>
      `Role os dois d20 e digite os dois números, na ordem em que saíram (1 a 20 cada). Conta o ${this.combine() === 'lower' ? 'menor' : 'maior'}.`,
  );
  protected readonly rollLabel = computed(() => {
    const a = this.attack();
    return a
      ? `Role 1d20 para ${attackName(a)} (${a.attackBonus < 0 ? '−' : '+'}${Math.abs(a.attackBonus)})`
      : '';
  });

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

  /** " · Meia cobertura (do mapa)": the cover the target has against this attacker. */
  protected coverNote(t: TargetInReach): string {
    const text = coverText(t.cover, t.coverSource);
    return text ? ` · ${text}` : '';
  }

  protected distance(ft: number | undefined, tooFar: boolean): string {
    const parts: string[] = [];
    if (ft !== undefined) {
      parts.push(`a ${metersFixed(ft)}`);
    }
    if (tooFar) {
      parts.push('fora do alcance');
    }
    return parts.join(', ');
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
        await this.api.standUp(this.campaignId(), this.encounter().id, this.subject().id),
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
      const res = await this.api.rollAttack(
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
      // The master's own screen shows this total and the word; a roll a reaction holds shows the face alone.
      const held = res.roll.heldForReaction;
      const hit = isHit(res.roll.outcome);
      const show = showOfDice(`Ataque com ${attackName(a)}`, res.roll.d20, {
        withTotal: true,
        outcome: held ? undefined : { word: outcomeWord(res.roll.outcome), good: hit },
        critical: !held && res.roll.outcome === AttackOutcome.CRITICAL_HIT,
        fumble: !held && !hit,
      });
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
