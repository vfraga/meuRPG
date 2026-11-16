import { CombatantKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from './combat-testing';
import { canMarkMount, mountMarkView } from './mount-mark';

describe("the master's mount mark (#588, boards 588-08 and 588-09)", () => {
  const tor = combatant({ id: 'tor', label: 'Tor', kind: CombatantKind.PLAYER, mountedOnId: 'h' });
  const horse = (over: object = {}) =>
    combatant({ id: 'h', label: 'Cavalo de guerra', ...over } as never);

  it('offers the three marks with what each means, the current one picked', () => {
    const v = mountMarkView(encounter({ combatants: [horse({ mountKind: 'independent' })] }), 'h');
    expect(v?.current).toBe('independent');
    expect(v?.options).toEqual([
      { kind: '', label: 'Nenhuma', hint: 'Não aceita cavaleiro.', disabled: false },
      {
        kind: 'controlled',
        label: 'Adestrada',
        hint: 'Age logo depois de quem monta e só usa Disparada, Desengajar e Esquivar.',
        disabled: false,
      },
      {
        kind: 'independent',
        label: 'Independente',
        hint: 'Mantém a própria vez e leva quem monta.',
        disabled: false,
      },
    ]);
    expect(v?.hint).toBe('Mantém a própria vez e leva quem monta.');
    expect(v?.why).toBe('');
  });

  it('names the rider in the line of a mount that carries one, and holds "Nenhuma" until the rider is off', () => {
    const e = encounter({ combatants: [tor, horse({ mountKind: 'controlled', riderId: 'tor' })] });
    const v = mountMarkView(e, 'h');
    expect(v?.hint).toBe('Age logo depois de Tor e só usa Disparada, Desengajar e Esquivar.');
    expect(v?.options[0].disabled).toBe(true);
    expect(v?.why).toBe('Nenhuma: só depois que Tor desmontar.');
    const free = mountMarkView(
      encounter({ combatants: [horse({ mountKind: 'independent', riderId: 'tor' }), tor] }),
      'h',
    );
    expect(free?.hint).toBe('Mantém a própria vez e leva Tor.');
  });

  it('reads an unmarked combatant as "Nenhuma"', () => {
    const v = mountMarkView(encounter({ combatants: [horse()] }), 'h');
    expect(v?.current).toBe('');
    expect(v?.hint).toBe('Não aceita cavaleiro.');
  });

  it("is for an NPC or a creature, never for a player's character", () => {
    expect(canMarkMount(horse())).toBe(true);
    expect(canMarkMount(horse({ kind: CombatantKind.CREATURE }))).toBe(true);
    expect(canMarkMount(tor)).toBe(false);
    expect(mountMarkView(encounter({ combatants: [tor] }), 'tor')).toBeNull();
  });
});
