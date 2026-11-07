import { CombatLogKind, EncounterStatus } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from './combat-testing';
import { legendaryLeftLine, legendarySpentLine } from './legendary-left';

function legendary(label: string, left: number, over: object = {}) {
  return combatant({
    id: label,
    label,
    legendary: {
      actionsPerRound: 3,
      actionsLeft: left,
      resistancePerDay: 3,
      resistanceLeft: 3,
      usableNow: true,
      options: [{ key: 'detect', name: 'Detect', namePt: 'Detectar', cost: 1, kind: 'text' }],
      ...over,
    },
  } as never);
}

const tor = combatant({ id: 't', label: 'Tor' });

describe('the line under "Próximo turno"', () => {
  it('says what is left for the one creature that can spend now', () => {
    const e = encounter({
      combatants: [legendary('Dragão vermelho adulto', 2), tor],
      currentCombatantId: 't',
    });
    expect(legendaryLeftLine(e)).toBe('Restam 2 ações lendárias: Dragão vermelho adulto.');
  });

  it('says it in the singular for one action', () => {
    const e = encounter({
      combatants: [legendary('Dragão vermelho adulto', 1), tor],
      currentCombatantId: 't',
    });
    expect(legendaryLeftLine(e)).toBe('Restam 1 ação lendária: Dragão vermelho adulto.');
  });

  it('joins several creatures in one sentence, with no pronoun', () => {
    const e = encounter({
      combatants: [legendary('Dragão vermelho adulto', 2), legendary('Vampiro', 3), tor],
      currentCombatantId: 't',
    });
    expect(legendaryLeftLine(e)).toBe(
      'Restam ações lendárias: Dragão vermelho adulto (2), Vampiro (3).',
    );
  });

  it('is silent when nothing can be spent: none left, on turn, incapacitated, a stat block without options, no combat', () => {
    const none = encounter({ combatants: [legendary('Dragão', 0), tor], currentCombatantId: 't' });
    const own = encounter({
      combatants: [legendary('Dragão', 3), tor],
      currentCombatantId: 'Dragão',
    });
    const down = encounter({
      combatants: [legendary('Dragão', 3, { usableNow: false }), tor],
      currentCombatantId: 't',
    });
    const bare = encounter({
      combatants: [legendary('Dragão', 3, { options: [] }), tor],
      currentCombatantId: 't',
    });
    const ended = encounter({
      combatants: [legendary('Dragão', 3), tor],
      currentCombatantId: 't',
      status: EncounterStatus.ENDED,
    });
    expect([none, own, down, bare, ended].map(legendaryLeftLine)).toEqual(['', '', '', '', '']);
  });
});

describe('the line of the last spend', () => {
  const spend = {
    kind: CombatLogKind.LEGENDARY_SPENT,
    actorId: 'd',
    actorLabel: 'Dragão vermelho adulto',
    legendarySpent: { optionName: 'Wing Attack', cost: 2 },
  } as never;

  it('says the cost and the option, the card above it names the creature', () => {
    expect(legendarySpentLine(spend, undefined)).toBe('Gastou 2: Wing Attack.');
  });

  it('reads the option in Portuguese when it is known, and the cost in the singular', () => {
    const one = {
      ...(spend as object),
      legendarySpent: { optionName: 'Detect', cost: 1 },
    } as never;
    const names = new Map([['d', new Map([['Detect', 'Detectar']])]]);
    expect(legendarySpentLine(one, names)).toBe('Gastou 1: Detectar.');
  });

  it('is empty for another entry or none', () => {
    expect(legendarySpentLine({ kind: CombatLogKind.MOVED } as never, undefined)).toBe('');
    expect(legendarySpentLine(null, undefined)).toBe('');
  });
});
