import { Component, computed, effect, input, output, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  Combatant,
  Encounter,
  HeldRollOffer,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { effectivePreference, preferenceLabel } from '../../../../core/campaigns/dice-labels';
import {
  combatantInitial,
  initiativeFormulaParts,
  isPlayer,
  ownCombatant,
  signed,
} from '../../../../core/combat/combat-view';
import { initiativeMark } from '../../../../core/combat/natural-mark';
import { groupName } from '../../../../core/combat/creature-names';
import { ownCreatures } from '../../../../core/combat/mine';
import type { HeldAnswer } from '../../../../core/held-rolls/held-roll-offer';
import type { InitiativeManeuver } from '../../../../core/combat/combat-client-types';
import { rollDiceFor } from '../../../../core/effects/roll-die';
import type { RollDie } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { RollDieOffer } from '../../../../shared/roll-die-offer/roll-die-offer';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import { HeldRollPrompt } from '../../../../shared/held-roll-prompt/held-roll-prompt';

/** One initiative to roll for the player's creatures: a casting rolls once for its group (RN-18). */
export interface CreatureInitiative {
  /** The group (or the creature alone). */
  readonly key: string;
  /** The combatant the roll is sent for: any member sets the whole group. */
  readonly id: string;
  /** "Lobos atrozes (2)" or the creature's name. */
  readonly label: string;
  readonly bonus: number;
  readonly total: number | undefined;
}

/**
 * The player's initiative (E6-03, RN-18, RN-19): before rolling, the bonus
 * and the way(s) to roll the d20 that the campaign allows; after, the total
 * in 92px and "Esperando o mestre começar o combate". The way follows the
 * campaign's dice mode and the player's choice: in the app ("Rolar no app",
 * `roll_in_app`), or a face typed from a physical die ("Digitar o resultado",
 * `d20_face`). Only the one that is the player's choice is the filled
 * button. A player rolls once: nobody rolls again until the result is good,
 * so the screen has no second try (the master may change it). While the
 * campaign lets each player choose, every roll offers both ways and the saved
 * choice only makes one of them the filled button (E6-03, E6-08); a forced
 * mode offers only its way. "Mudar" opens the saved choice (E6-18).
 */
@Component({
  selector: 'app-player-initiative',
  imports: [CombatantToken, HeldRollPrompt, MatButtonModule, MatIconModule, RollDieOffer],
  templateUrl: './player-initiative.html',
  styleUrl: './player-initiative.scss',
})
export class PlayerInitiative {
  readonly encounter = input.required<Encounter>();
  readonly diceMode = input.required<DiceMode>();
  readonly dicePreference = input.required<DicePreference>();
  readonly busy = input(false);
  /** The table dice the character may add to a roll (`DerivedSheet.roll_dice`); the initiative offers those of "initiative". */
  readonly rollDice = input<readonly RollDie[]>([]);
  /** A refused roll's message (the page maps the typed error). */
  readonly error = input('');

  /** The question the d20 kept for the player's own roll asks (Lucky's 1, a Bardic Inspiration die), while it waits. */
  readonly held = input<HeldRollOffer | null>(null);
  /** The answer to that question. */
  readonly answer = output<HeldAnswer>();

  /** "Rolar no app". */
  readonly rollInApp = output<void>();
  /** "Confirmar": the face typed from a physical die. */
  readonly typeFace = output<number>();
  /** "Rolar no app" for a creature (or a group): its combatant id. */
  readonly rollCreatureInApp = output<string>();
  /** "Confirmar" for a creature (or a group): the combatant id and the typed face. */
  readonly typeCreatureFace = output<{ id: string; face: number }>();
  /** "Mudar": the dice choice. */
  readonly changeDice = output<void>();
  /** The table die ticked for this initiative, with its face when typed; `null` for none. The page sends it with the roll. */
  readonly maneuverChange = output<InitiativeManeuver | null>();

  protected readonly typing = signal(false);
  protected readonly dice = computed(() => rollDiceFor(this.rollDice(), { kind: 'initiative' }));
  /** The key of the die ticked, and the face typed for it (physical dice). */
  protected readonly pick = signal('');
  protected readonly typedDie = signal('');
  protected readonly die = computed(() => this.dice().find((d) => d.key === this.pick()) ?? null);
  protected readonly dieFace = computed(() => {
    const sides = this.die()?.sides ?? 0;
    const text = this.typedDie().trim();
    return /^\d{1,2}$/.test(text) && +text >= 1 && +text <= sides ? +text : null;
  });
  /** The typed d20 cannot go while the ticked die has no face. */
  protected readonly dieMissing = computed(() => this.die() !== null && this.dieFace() === null);
  protected readonly typed = signal('');
  /** The key of the creature whose result is being typed, if any. */
  protected readonly typingCreature = signal<string | null>(null);
  protected readonly typedCreature = signal('');

  /** The player's creatures, one row per casting (RN-18), in the order of the combat. */
  protected readonly creatures = computed<CreatureInitiative[]>(() => {
    const groups = new Map<string, Combatant[]>();
    for (const c of ownCreatures(this.encounter())) {
      const key = c.summonGroupId || c.id;
      groups.set(key, [...(groups.get(key) ?? []), c]);
    }
    return [...groups].map(([key, members]) => ({
      key,
      id: members[0].id,
      label: members.length > 1 ? `${groupName(members)} (${members.length})` : members[0].label,
      bonus: members[0].initiativeBonus ?? 0,
      total: members.find((m) => m.initiative !== undefined)?.initiative,
    }));
  });
  protected readonly creatureFace = computed(() => this.faceOf(this.typedCreature()));
  protected readonly creatureInvalid = computed(
    () => this.typedCreature().trim() !== '' && this.creatureFace() === null,
  );

  protected readonly own = computed(() => ownCombatant(this.encounter()));
  protected readonly bonus = computed(() => this.own()?.initiativeBonus ?? 0);
  /** "1d20 + 2" (or "− 1"). */
  protected readonly diceText = computed(
    () => `1d20 ${this.bonus() < 0 ? '−' : '+'} ${Math.abs(this.bonus())}`,
  );
  protected readonly rolled = computed(() => this.own()?.initiative !== undefined);
  protected readonly formula = computed(() => {
    const own = this.own();
    return own ? (initiativeFormulaParts(own)?.line ?? '') : '';
  });
  /** "d20: 3 → 12 (Pé de Vento)": what a table minimum did to the d20, under the formula; empty when nothing raised it. */
  protected readonly raisedNote = computed(() => {
    const own = this.own();
    return own ? (initiativeFormulaParts(own)?.note ?? '') : '';
  });
  /** "20 natural" / "1 natural": the player's own roll only; information, initiative has no automatic result. */
  protected readonly mark = computed(() => {
    const own = this.own();
    return own ? initiativeMark(own) : '';
  });
  protected readonly players = computed(() => this.encounter().combatants.filter(isPlayer));

  /** How this player rolls: the campaign's mode, then their own choice. */
  protected readonly way = computed(() =>
    effectivePreference(this.diceMode(), this.dicePreference()),
  );
  protected readonly inApp = computed(() => this.way() === DicePreference.APP);
  /** Both ways while the campaign lets each player choose; the one way a
   * forced mode allows otherwise (RN-18). The saved choice only decides
   * which button is the filled one. */
  protected readonly canApp = computed(() => this.diceMode() !== DiceMode.PHYSICAL);
  protected readonly canType = computed(() => this.diceMode() !== DiceMode.APP);
  protected readonly showTyping = computed(() => this.typing() || !this.canApp());
  protected readonly chooses = computed(() => this.diceMode() === DiceMode.PLAYERS_CHOOSE);
  protected readonly wayLabel = computed(() => preferenceLabel(this.way()));

  protected readonly face = computed(() => this.faceOf(this.typed()));
  protected readonly invalid = computed(() => this.typed().trim() !== '' && this.face() === null);
  protected readonly sum = computed(() => {
    const face = this.face();
    if (face === null) {
      return 'Digite o número que saiu no dado.';
    }
    const sign = this.bonus() < 0 ? '−' : '+';
    return `${face} ${sign} ${Math.abs(this.bonus())} = ${face + this.bonus()} · dado físico`;
  });

  constructor() {
    effect(() => {
      const die = this.die();
      this.maneuverChange.emit(
        die ? { key: die.key, ...(this.showTyping() ? { face: this.dieFace() ?? 0 } : {}) } : null,
      );
    });
  }

  protected onTypeDie(event: Event): void {
    this.typedDie.set((event.target as HTMLInputElement).value);
  }

  private faceOf(text: string): number | null {
    const value = text.trim();
    return /^\d{1,2}$/.test(value) && +value >= 1 && +value <= 20 ? +value : null;
  }

  protected startTyping(key: string): void {
    this.typedCreature.set('');
    this.typingCreature.set(key);
  }

  protected onTypeCreature(event: Event): void {
    this.typedCreature.set((event.target as HTMLInputElement).value);
  }

  protected confirmCreature(id: string): void {
    const face = this.creatureFace();
    if (face !== null && !this.busy()) {
      this.typeCreatureFace.emit({ id, face });
      this.typingCreature.set(null);
    }
  }

  protected initial(label: string): string {
    return combatantInitial(label);
  }

  protected signed(n: number): string {
    return signed(n);
  }

  protected onType(event: Event): void {
    this.typed.set((event.target as HTMLInputElement).value);
  }

  protected confirm(): void {
    const face = this.face();
    if (face !== null && !this.dieMissing() && !this.busy()) {
      this.typeFace.emit(face);
    }
  }
}
