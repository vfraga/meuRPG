import { describe, expect, it } from 'vitest';

import { CombatantKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from './combat-testing';
import { whoseTurn } from './whose-turn';

describe('whoseTurn', () => {
  const tor = combatant({ id: 'tor', label: 'Tor', kind: CombatantKind.PLAYER });
  const zombie = combatant({ id: 'z', label: 'Zumbi 1' });
  const spider = combatant({ id: 'a', label: 'Aranha gigante' });

  it('names a character with no article and a creature with its own', () => {
    expect(whoseTurn(encounter({ combatants: [tor], currentCombatantId: 'tor' }), '')).toBe(
      'A vez de Tor',
    );
    expect(whoseTurn(encounter({ combatants: [zombie], currentCombatantId: 'z' }), '')).toBe(
      'A vez do Zumbi 1',
    );
    expect(whoseTurn(encounter({ combatants: [spider], currentCombatantId: 'a' }), '')).toBe(
      'A vez da Aranha gigante',
    );
  });

  it("says 'A sua vez' to the one on turn and 'A vez' when the one on turn is hidden", () => {
    const e = encounter({ combatants: [tor], currentCombatantId: 'tor' });
    expect(whoseTurn(e, 'tor')).toBe('A sua vez');
    expect(whoseTurn(encounter({ combatants: [tor] }), '')).toBe('A vez');
  });
});
