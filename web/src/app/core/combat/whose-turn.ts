import type { Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import { theName } from '../format/article';
import { currentCombatant, isPlayer } from './combat-view';

/** "A vez do Zumbi 1", "A vez de Tor", "A sua vez" (the reader's own combatant is `selfId`), or "A vez" when the one on turn is hidden from the reader. A character is named with no article. */
export function whoseTurn(e: Encounter, selfId: string): string {
  const on = currentCombatant(e);
  if (!on) {
    return 'A vez';
  }
  if (on.id === selfId) {
    return 'A sua vez';
  }
  return `A vez ${isPlayer(on) ? `de ${on.label}` : theName(on.label, 'de')}`;
}
