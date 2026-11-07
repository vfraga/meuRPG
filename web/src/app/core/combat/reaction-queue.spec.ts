import { CombatantKind, ReactionKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter, reactionWindow } from './combat-testing';
import { masterQueue } from './reaction-queue';
import { reactionCards } from './reaction-master';

const tor = combatant({ id: 't', label: 'Tor', kind: CombatantKind.PLAYER });
const dragon = combatant({
  id: 'd',
  label: 'Dragão vermelho adulto',
  legendary: {
    actionsPerRound: 3,
    actionsLeft: 3,
    resistancePerDay: 3,
    resistanceLeft: 3,
    usableNow: true,
    options: [{ key: 'a', name: 'A', namePt: 'Detectar', cost: 1, kind: 'text', affordable: true }],
  },
} as never);
const captain = combatant({ id: 'c', label: 'Capitão bandido' });

const parry = reactionWindow({
  id: 'p',
  kind: ReactionKind.PARRY,
  reactorId: 'c',
  reactorLabel: 'Capitão bandido',
  groupId: 'g1',
  answerNow: true,
  trigger: { actorId: 't', actorLabel: 'Tor', attackTotal: 16, targetArmorClass: 15 } as never,
  prompt: {
    case: 'parry',
    value: {
      attackerLabel: 'Tor',
      attackNamePt: 'Espada longa',
      acBonus: 2,
      armorClassWithParry: 17,
    },
  },
} as never);
const lr = reactionWindow({
  id: 'lr',
  kind: ReactionKind.LEGENDARY_RESISTANCE,
  groupId: 'g2',
  answerNow: false,
  trigger: { actorId: 'pen', actorLabel: 'Pensantus', actionNamePt: 'Relâmpago' } as never,
  prompt: {
    case: 'legendaryResistance',
    value: {
      offers: [
        {
          combatantId: 'd',
          label: 'Dragão vermelho adulto',
          d20: 4,
          total: 10,
          dc: 15,
          usesLeft: 3,
          usesPerDay: 3,
        },
      ],
    },
  },
} as never);

describe('the master queue "Esperando você"', () => {
  const e = encounter({
    combatants: [dragon, tor, captain],
    currentCombatantId: 't',
    reactionWindows: [parry, lr],
  });

  it('gives Parry and Legendary Resistance their own cards', () => {
    expect(reactionCards(e).map((c) => c.type)).toEqual(['parry', 'legendaryResistance']);
  });

  it('counts the blocking windows, marks one "Agora" and the rest "Depois"', () => {
    const q = masterQueue(e);
    expect(q.count).toBe(2);
    expect(q.header).toBe('Esperando você (2)');
    expect(q.hint).toBe('uma de cada vez');
    expect(q.items.map((i) => i.badge)).toEqual(['Agora', 'Depois']);
    expect(q.items[1].waiting).toBe('Depois da janela acima.');
    expect(q.legendary[0].badge).toBe('Depois');
    expect(q.legendary[0].waiting).toContain('Depois das duas janelas acima');
  });

  it('says "trava a mesa" for one window and "uma de cada vez" for several', () => {
    const one = encounter({
      combatants: [dragon, tor, captain],
      currentCombatantId: 't',
      reactionWindows: [parry],
    } as never);
    expect(masterQueue(one).count).toBe(1);
    expect(masterQueue(one).hint).toBe('trava a mesa');
    expect(masterQueue(e).hint).toBe('uma de cada vez');
  });

  it('puts the window to answer now first even when the server lists it later', () => {
    const swapped = encounter({
      combatants: [dragon, tor, captain],
      currentCombatantId: 't',
      reactionWindows: [
        { ...lr, answerNow: true } as never,
        { ...parry, answerNow: false } as never,
      ],
    });
    expect(masterQueue(swapped).items.map((i) => i.card.type)).toEqual([
      'legendaryResistance',
      'parry',
    ]);
  });

  it('shows no header and an optional legendary card when nothing blocks the table', () => {
    const q = masterQueue(encounter({ combatants: [dragon, tor], currentCombatantId: 't' }));
    expect(q.count).toBe(0);
    expect(q.header).toBe('');
    expect(q.legendary[0].badge).toBe('Opcional');
  });

  it('says a player holds the table when the master has nothing to answer, and the header stays empty', () => {
    const playerWindow = reactionWindow({
      id: 'sh',
      kind: ReactionKind.SHIELD,
      reactorId: 't',
      reactorLabel: 'Tor',
      reactorIsPlayer: true,
      groupId: 'g3',
      prompt: { case: 'shield', value: { slots: [] } },
    } as never);
    const q = masterQueue(
      encounter({
        combatants: [dragon, tor],
        currentCombatantId: 't',
        reactionWindows: [playerWindow],
      }),
    );
    expect(q.count).toBe(0);
    expect(q.header).toBe('');
    expect(q.legendary[0].waiting).toBe('Esperando a reação de um jogador.');
    expect(q.legendary[0].waiting).not.toContain('janela acima');
  });

  it('honours the answerNow of a queue row and of a single card', () => {
    const npc = (id: string, now: boolean, group: string) =>
      reactionWindow({
        id,
        kind: ReactionKind.SHIELD,
        reactorId: 'c',
        reactorLabel: 'Capitão bandido',
        groupId: group,
        answerNow: now,
        prompt: { case: 'shield', value: { slots: [] } },
      } as never);
    const q = masterQueue(
      encounter({
        combatants: [dragon, tor, captain],
        currentCombatantId: 't',
        reactionWindows: [npc('a', false, 'ga'), npc('b', true, 'gb')],
      }),
    );
    expect(q.items.map((i) => (i.card.type === 'single' ? i.card.row.window.id : ''))).toEqual([
      'b',
      'a',
    ]);
    expect(q.items[0].badge).toBe('Agora');
  });
});
