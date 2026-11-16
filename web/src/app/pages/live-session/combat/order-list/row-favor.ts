import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { isPlayer } from '../../../../core/combat/combat-view';
import { type FavorState, favorHoldsText } from '../../../../core/play/favor-state';

/**
 * The master's Inspiração on an order row (RN-43, boards 581-01 and 581-07 item 12): whether "Dar Inspiração" is offered (a
 * player's character, in an open session; an NPC never holds it), whether the character has it (the badge and the dashed item),
 * and the tap, which does nothing for a holder or while a grant is on its way.
 */
export class RowFavor {
  constructor(readonly state: FavorState) {}

  canGive(c: Combatant): boolean {
    return this.state.campaignId() !== '' && isPlayer(c) && !!c.characterId;
  }

  holds(c: Combatant): boolean {
    return isPlayer(c) && !!c.characterId && this.state.has(c.characterId);
  }

  /** The character's own ask to pass it waits for the master, and the reader is its owner: the chip says so (581-07, item 4). */
  waiting(c: Combatant): boolean {
    return isPlayer(c) && !!c.characterId && this.state.waiting(c.characterId);
  }

  /** Off for a holder and while its own grant is on its way. */
  off(c: Combatant): boolean {
    return this.holds(c) || this.state.granting().has(c.characterId);
  }

  /** The reason under the dashed item: "Pensantus já tem Inspiração." */
  reason(c: Combatant): string {
    return favorHoldsText(c.label);
  }

  give(c: Combatant): void {
    if (!this.off(c)) {
      void this.state.grant(c.characterId, c.label);
    }
  }
}
