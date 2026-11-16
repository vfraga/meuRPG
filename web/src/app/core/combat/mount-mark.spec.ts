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
    expect(v?.note).toBe('');
  });

  it('with no mark yet, says what marking does, naming the creature (board 588-09)', () => {
    const mastim = combatant({ id: 'm', label: 'Mastim' });
    expect(mountMarkView(encounter({ combatants: [mastim] }), 'm')?.note).toBe(
      'Marcado, o Mastim aceita um cavaleiro, que monta na própria vez. Você monta qualquer criatura.',
    );
    const aranha = combatant({ id: 'a', label: 'Aranha gigante' });
    expect(mountMarkView(encounter({ combatants: [aranha] }), 'a')?.note).toBe(
      'Marcada, a Aranha gigante aceita um cavaleiro, que monta na própria vez. Você monta qualquer criatura.',
    );
  });

  it('names the rider in the line of a mount that carries one, and holds "Nenhuma" until the rider is off', () => {
    const e = encounter({ combatants: [tor, horse({ mountKind: 'controlled', riderId: 'tor' })] });
    const v = mountMarkView(e, 'h');
    expect(v?.hint).toBe('Age logo depois de Tor e só usa Disparada, Desengajar e Esquivar.');
    expect(v?.options[0].disabled).toBe(true);
    // The notes' words (588-10 card 9): no label before them.
    expect(v?.why).toBe('Só depois que Tor desmontar.');
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
