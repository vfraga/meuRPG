import { Component, computed, effect, input, output, signal, untracked } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant, Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../../../../core/combat/combat-log';
import { combatantInitial, isPlayer } from '../../../../core/combat/combat-view';
import { isCreature, kindWord, ofOwner } from '../../../../core/combat/creature-names';
import {
  type JointTurn,
  jointTurn,
  listNames,
  pairTurn,
  missingLine,
  partEconomy,
  turnMembers,
} from '../../../../core/combat/joint-turn';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import type { CombatantInfo } from '../combat-info';
import { PairCard } from '../pair-card/pair-card';
import { PartState } from './part-state';

/**
 * The master's card of a joint turn (E8-01, states 4 and 6): one block per
 * member of the group on turn, with its state in a word and an icon and what
 * it still has, and "Encerrar a parte da Brisa" in the block of each one who
 * still acts, so the master ends the part of a player who is away. There is no
 * "end the group's turn": it passes by itself when the last part ends. A live
 * line says who is missing and, when the turn passes, "A vez passou para ...".
 * It takes the place of the NPC card while a group of two or more is on turn;
 * a turn of one never shows it.
 */
@Component({
  selector: 'app-joint-card',
  imports: [CombatantToken, MatButtonModule, MatIconModule, PairCard, PartState],
  templateUrl: './joint-card.html',
  styleUrl: './joint-card.scss',
})
export class JointCard {
  readonly encounter = input.required<Encounter>();
  readonly info = input<ReadonlyMap<string, CombatantInfo>>(new Map());
  readonly busy = input(false);
  /** "Encerrar a parte da Brisa": the member's combatant ID. */
  readonly endPart = output<string>();

  /** A rider and the mount it directs, alone on turn: one pair card, no joint turn. */
  protected readonly pair = computed(() => pairTurn(this.encounter()));
  protected readonly joint = computed<JointTurn | null>(() =>
    this.pair() ? null : jointTurn(this.encounter()),
  );
  protected readonly title = computed(
    () => `Turno conjunto: ${listNames((this.joint()?.members ?? []).map((m) => m.label))}`,
  );
  protected readonly missing = computed(() => {
    const j = this.joint();
    return j
      ? missingLine(
          j.acting.map((m) => m.label),
          false,
        )
      : '';
  });
  /** The line the screen reader hears when the turn moves on. */
  private readonly passedTo = signal('');
  protected readonly live = computed(() => this.missing() || this.passedTo());

  constructor() {
    // When the joint turn is over, the line says who has the turn now; it goes when that turn is over too.
    let wasJoint = false;
    let announced = '';
    effect(() => {
      const joint = this.joint();
      const e = this.encounter();
      untracked(() => {
        const key = `${e.round}:${turnMembers(e)
          .map((m) => m.id)
          .join(',')}`;
        if (joint) {
          wasJoint = true;
          announced = '';
          this.passedTo.set('');
        } else if (wasJoint) {
          wasJoint = false;
          const next = turnMembers(e);
          announced = next.length > 0 ? key : '';
          this.passedTo.set(
            next.length > 0 ? `A vez passou para ${listNames(next.map((m) => m.label))}` : '',
          );
        } else if (announced !== key) {
          announced = '';
          this.passedTo.set('');
        }
      });
    });
  }

  protected initial(c: Combatant): string {
    return combatantInitial(c.label);
  }

  protected npc(c: Combatant): boolean {
    return !isPlayer(c) && !isCreature(c);
  }

  protected creature(c: Combatant): boolean {
    return isCreature(c);
  }

  protected sub(c: Combatant): string {
    const info = this.info().get(c.characterId);
    const ac = c.armorClass === undefined ? '' : `CA ${c.armorClass}`;
    if (isCreature(c)) {
      return [kindWord(c), ac, ofOwner(this.encounter(), c)].filter(Boolean).join(' · ');
    }
    const first = isPlayer(c) ? (info?.classSummary ?? '') : (info?.kindLabel ?? 'NPC');
    return [first, ac].filter(Boolean).join(' · ');
  }

  protected chips(c: Combatant): { name: string; word: string; used: boolean }[] {
    const e = partEconomy(c);
    return [
      { name: 'Ação', word: c.actionUsed ? 'Usada' : 'Disponível', used: c.actionUsed },
      {
        name: 'Ação bônus',
        word: c.bonusActionUsed ? 'Usada' : 'Disponível',
        used: c.bonusActionUsed,
      },
      { name: 'Reação', word: c.reactionUsed ? 'Usada' : 'Disponível', used: c.reactionUsed },
      {
        name: 'Movimento',
        word: e.movement ? `Restam ${e.movement}` : 'Sem movimento',
        used: !e.movement,
      },
    ];
  }

  /** "Encerrar a parte da Brisa". */
  protected endLabel(c: Combatant): string {
    return `Encerrar a parte ${article(c.label) === 'a' ? 'da' : 'do'} ${c.label}`;
  }

  protected ended(c: Combatant): boolean {
    return c.turnPartEnded;
  }
}
