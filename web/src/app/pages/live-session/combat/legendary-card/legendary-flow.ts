import { type WritableSignal, computed, effect, signal, untracked } from '@angular/core';

import type { Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { CombatClient } from '../../../../core/combat/combat-client';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import { type LegendaryNames, legendaryNames } from '../../../../core/combat/legendary-card';
import type { AttackOpen } from '../combat-sheets-context';
import type { ArmedMove, LegendaryPick } from './legendary-offers';

/** What `LegendaryFlow` reads from the screen it serves. */
export interface LegendaryFlowContext {
  readonly api: Pick<CombatClient, 'moves'>;
  readonly campaignId: () => string;
  readonly encounter: () => Encounter | null;
  /** The attack sheet of the page. */
  readonly attack: (req: AttackOpen) => void;
  /** Where a refusal is said (the page's notice). */
  readonly error: WritableSignal<string>;
  /** The move the master armed, owned by the move flow (the drag of the token sends it). */
  readonly armed: WritableSignal<ArmedMove | null>;
}

/** The names now known, or `known` itself when nothing changed: the signal keeps its value on every tick of the combat. */
function merged(known: LegendaryNames, next: LegendaryNames): LegendaryNames {
  const same =
    known.size === next.size &&
    [...next].every(([id, names]) => {
      const before = known.get(id);
      return before?.size === names.size && [...names].every(([en, pt]) => before.get(en) === pt);
    });
  return same ? known : next;
}

/**
 * What the page does for the legendary cards: the options' names the log reads, the legendary attack (the attack
 * sheet of the creature off its turn, with the option) and the legendary move (the drag of its token, armed with the
 * option). The spend itself, the fold and the undo live in `LegendaryOffers`. Build it in an injection context: it keeps
 * an effect.
 */
export class LegendaryFlow {
  /** The Portuguese names of the legendary options of every creature seen in this combat, kept when a creature leaves. */
  readonly names = signal<LegendaryNames>(new Map());
  /** The attack sheet of an option is being opened (the creature's `GetTurnOptions` is in the air): a second tap waits. */
  readonly opening = signal<{ combatantId: string; key: string } | null>(null);

  constructor(private readonly ctx: LegendaryFlowContext) {
    effect(() => {
      const e = this.ctx.encounter();
      if (e) {
        untracked(() => this.names.update((known) => merged(known, legendaryNames(e, known))));
      }
    });
    // A move armed for a turn is not carried into the next one: the card it came from is gone.
    const turn = computed(() => this.ctx.encounter()?.currentCombatantId ?? '');
    effect(() => {
      void turn();
      untracked(() => this.ctx.armed.set(null));
    });
  }

  /** "Atacar": the creature's own attack of the stat block, in the attack sheet, spending the option and nothing of its turn. */
  async attack(pick: LegendaryPick): Promise<void> {
    const e = this.ctx.encounter();
    const who = e?.combatants.find((c) => c.id === pick.combatantId);
    if (!e || !who || this.opening()) {
      return;
    }
    this.ctx.error.set('');
    this.opening.set({ combatantId: who.id, key: pick.row.key });
    try {
      const options = await this.ctx.api.moves.turnOptions(this.ctx.campaignId(), e.id, who.id);
      if (!options.options?.attacks.some((a) => a.attack?.key === pick.row.attackKey)) {
        this.ctx.error.set('Esse ataque não está na ficha da criatura. Nada foi gasto.');
        return;
      }
      const left = who.legendary?.actionsLeft ?? 0;
      this.ctx.attack({
        key: pick.row.attackKey,
        who,
        creatureOpts: options,
        legendary: {
          option: pick.row.key,
          title: `${pick.row.name} · ação lendária`,
          subtitle: `${who.label} · custa ${pick.row.cost} de ${left} restantes`,
          cost: pick.row.cost,
          label: who.label,
        },
      });
    } catch (err) {
      this.ctx.error.set(combatErrorMessage(err, 'abrir o ataque'));
    } finally {
      this.opening.set(null);
    }
  }

  /** A move row: the next drag of the creature's token on the map walks it with the option. */
  move(pick: LegendaryPick): void {
    this.ctx.armed.set({ combatantId: pick.combatantId, optionKey: pick.row.key });
  }
}
