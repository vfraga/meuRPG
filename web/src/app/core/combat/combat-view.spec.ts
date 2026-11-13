import { CombatantKind, CombatantState } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from './combat-testing';
import { creatureTurnTitle } from './creature-names';
import {
  combatantInitial,
  ownCombatant,
  initiativeFormula,
  initiativeFormulaParts,
  isDown,
  playerWord,
  moveInGroup,
  nextCombatant,
  pendingFormula,
  stateWord,
  tieGroups,
  tieSentence,
  turnBanner,
} from './combat-view';

describe('combat view helpers', () => {
  it("finds the player's own combatant by `mine`, never a creature of theirs (MR-037)", () => {
    // The server marks a character's creature `controlledByMe`, never `mine`: a
    // combat with a familiar must not make the screen pick the owl as "me".
    const e = encounter({
      combatants: [
        combatant({
          id: 'nanquim',
          label: 'Nanquim',
          kind: CombatantKind.CREATURE,
          mine: false,
          controlledByMe: true,
          ownerCharacterId: 'pens',
        }),
        combatant({
          id: 'pens',
          label: 'Pensantus',
          kind: CombatantKind.PLAYER,
          mine: true,
          controlledByMe: true,
        }),
        combatant({ id: 'toren', label: 'Toren', kind: CombatantKind.PLAYER }),
      ],
    });
    expect(ownCombatant(e)?.label).toBe('Pensantus');
    // A player whose character is not in the fight but whose creature is has no "own" combatant.
    expect(
      ownCombatant(encounter({ combatants: [e.combatants[0]!, e.combatants[2]!] })),
    ).toBeNull();
  });

  it('says how hurt an NPC is in words, never numbers (RN-20)', () => {
    expect(stateWord(CombatantState.UNHURT)).toBe('Ileso');
    expect(stateWord(CombatantState.HURT)).toBe('Ferido');
    expect(stateWord(CombatantState.BADLY_HURT)).toBe('Muito ferido');
    expect(stateWord(CombatantState.DEFEATED)).toBe('Derrotado');
  });

  it('writes the word of the fallen in the gender of their name', () => {
    expect(stateWord(CombatantState.DOWN, 'Brisa')).toBe('Caída');
    expect(stateWord(CombatantState.DOWN, 'Toren')).toBe('Caído');
    expect(stateWord(CombatantState.DEAD, 'Brisa')).toBe('Morta');
    expect(stateWord(CombatantState.DEAD, 'Toren')).toBe('Morto');
  });

  it('counts the stable as down, and tells a player only what happened to a fallen one', () => {
    const state = (s: CombatantState) => combatant({ id: 'x', label: 'Toren', state: s });
    expect(isDown(state(CombatantState.STABLE))).toBe(true);
    expect(isDown(state(CombatantState.DYING))).toBe(true);
    expect(isDown(state(CombatantState.UNHURT))).toBe(false);
    expect(playerWord(state(CombatantState.STABLE))).toBe('Estável');
    expect(playerWord(state(CombatantState.DEAD))).toBe('Morto');
    expect(playerWord(state(CombatantState.HURT))).toBe('Jogador');
  });

  it('puts the number of a numbered copy on its token', () => {
    expect(combatantInitial('Goblin 2')).toBe('G2');
    expect(combatantInitial('Capitão Goblin')).toBe('C');
    expect(combatantInitial('pensantus')).toBe('P');
  });

  it('writes the initiative as the artboard does', () => {
    const c = combatant({
      id: 'a',
      label: 'Brisa',
      initiative: 19,
      initiativeFace: 15,
      initiativeBonus: 4,
    });
    expect(initiativeFormula(c)).toBe('1d20 (15) + 4 = 19');
    const low = combatant({
      id: 'b',
      label: 'Toren',
      initiative: 14,
      initiativeFace: 15,
      initiativeBonus: -1,
    });
    expect(initiativeFormula(low)).toBe('1d20 (15) − 1 = 14');
    const waiting = combatant({ id: 'c', label: 'Toren', initiativeBonus: 2 });
    expect(initiativeFormula(waiting)).toBeNull();
    const raised = {
      ...c,
      initiativeFace: 3,
      initiativeTreatedAs: 12,
      initiativeTreatedAsSource: 'content:feat-keen',
      initiativeTreatedAsSourceName: 'Olho Aguçado',
      initiativeBonus: 4,
      initiative: 16,
    } as typeof c;
    expect(initiativeFormula(raised)).toBe('d20: 3 → 12 (Olho Aguçado) + 4 = 16');
    const inspired = combatant({
      id: 'd',
      label: 'Toren',
      initiative: 13,
      initiativeFace: 7,
      initiativeBonus: 2,
    });
    expect(initiativeFormula(inspired)).toBe('1d20 (7) + 2 + 4 (dado) = 13');
    expect(pendingFormula(waiting)).toBe('1d20 + 2 = —');
  });

  it('splits the initiative into the total line and the note of the d20 a minimum raised', () => {
    const plain = combatant({
      id: 'a',
      label: 'Brisa',
      initiative: 19,
      initiativeFace: 15,
      initiativeBonus: 4,
    });
    // Nothing raised it: the line is the formula as it always was, with no note.
    expect(initiativeFormulaParts(plain)).toEqual({ line: '1d20 (15) + 4 = 19', note: null });
    expect(initiativeFormulaParts(combatant({ id: 'c', label: 'Toren' }))).toBeNull();
    const raised = {
      ...plain,
      initiativeFace: 3,
      initiativeTreatedAs: 12,
      initiativeTreatedAsSource: 'content:feat-keen',
      initiativeTreatedAsSourceName: 'Pé de Vento Muito Mais Que Longo',
      initiativeBonus: -1,
      initiative: 11,
    } as typeof plain;
    // The total keeps its place; the feature's name goes to the note.
    expect(initiativeFormulaParts(raised)).toEqual({
      line: '1d20 (12) − 1 = 11',
      note: 'd20: 3 → 12 (Pé de Vento Muito Mais Que Longo)',
    });
    const inspired = { ...raised, initiativeBonus: 3, initiative: 19 } as typeof plain;
    expect(initiativeFormulaParts(inspired)?.line).toBe('1d20 (12) + 3 + 4 (dado) = 19');
  });

  describe('ties', () => {
    const list = [
      combatant({ id: 'brisa', label: 'Brisa', initiative: 19, initiativeBonus: 4 }),
      combatant({
        id: 'g1',
        label: 'Goblin 1',
        initiative: 12,
        initiativeBonus: 2,
        tieUnresolved: true,
      }),
      combatant({
        id: 'g2',
        label: 'Goblin 2',
        initiative: 12,
        initiativeBonus: 2,
        tieUnresolved: true,
      }),
      combatant({ id: 'g3', label: 'Goblin 3', initiative: 9, initiativeBonus: 2 }),
    ];

    it('groups the unordered ties that follow each other', () => {
      expect(tieGroups(list)).toEqual([['g1', 'g2']]);
      expect(tieGroups(list.slice(0, 1))).toEqual([]);
    });

    it('keeps two ties on the same total but another bonus apart', () => {
      const two = [
        combatant({ id: 'a', label: 'A', initiative: 12, initiativeBonus: 1, tieUnresolved: true }),
        combatant({ id: 'b', label: 'B', initiative: 12, initiativeBonus: 3, tieUnresolved: true }),
      ];
      expect(tieGroups(two)).toEqual([['a'], ['b']]);
    });

    it('keeps two ties on different totals apart', () => {
      const two = [
        combatant({ id: 'a', label: 'A', initiative: 12, initiativeBonus: 1, tieUnresolved: true }),
        combatant({ id: 'b', label: 'B', initiative: 12, initiativeBonus: 1, tieUnresolved: true }),
        combatant({ id: 'c', label: 'C', initiative: 10, initiativeBonus: 0, tieUnresolved: true }),
        combatant({ id: 'd', label: 'D', initiative: 10, initiativeBonus: 0, tieUnresolved: true }),
      ];
      expect(tieGroups(two)).toEqual([
        ['a', 'b'],
        ['c', 'd'],
      ]);
    });

    it('moves a combatant inside its group only: the outer arrows do nothing', () => {
      expect(moveInGroup(['g1', 'g2'], 'g1', 1)).toEqual(['g2', 'g1']);
      expect(moveInGroup(['g1', 'g2'], 'g1', -1)).toEqual(['g1', 'g2']);
      expect(moveInGroup(['g1', 'g2'], 'g2', 1)).toEqual(['g1', 'g2']);
      expect(moveInGroup(['a', 'b', 'c'], 'b', -1)).toEqual(['b', 'a', 'c']);
    });

    it('says the tie as the artboard does', () => {
      expect(tieSentence(['Goblin 1', 'Goblin 2'], 12)).toBe(
        'Empate em 12: Goblin 1 e Goblin 2. Escolha a ordem com as setas.',
      );
      expect(tieSentence(['A', 'B', 'C'], 7)).toBe(
        'Empate em 7: A, B e C. Escolha a ordem com as setas.',
      );
    });
  });

  describe('the turn banner', () => {
    const order = [
      combatant({ id: 'brisa', label: 'Brisa', kind: CombatantKind.PLAYER }),
      combatant({ id: 'cap', label: 'Capitão Goblin' }),
      combatant({ id: 'pen', label: 'Pensantus', kind: CombatantKind.PLAYER, mine: true }),
      combatant({ id: 'g1', label: 'Goblin 1', defeated: true }),
    ];

    it('names the one on turn, and who is next when it is the player', () => {
      const banner = turnBanner(encounter({ combatants: order, currentCombatantId: 'cap' }));
      expect(banner.title).toBe('Vez do Capitão Goblin');
      expect(banner.nextIsMine).toBe(true);
      expect(banner.next?.label).toBe('Pensantus');
    });

    it('says "Sua vez" on the player\'s own turn', () => {
      const banner = turnBanner(encounter({ combatants: order, currentCombatantId: 'pen' }));
      expect(banner.title).toBe('Sua vez, Pensantus');
      expect(banner.mine).toBe(true);
    });

    it('tells the player who ended their part of a joint turn that they did', () => {
      const P = CombatantKind.PLAYER;
      const e = encounter({
        combatants: [
          combatant({
            id: 'b',
            label: 'Brisa',
            kind: P,
            initiative: 19,
            mine: true,
            turnPartEnded: true,
          }),
          combatant({ id: 't', label: 'Toren', kind: P, initiative: 19 }),
        ],
        turnGroupIds: ['b', 't'],
        currentCombatantId: 't',
      });
      expect(turnBanner(e)).toMatchObject({
        title: 'Você encerrou a sua parte',
        ownEnded: true,
        mine: false,
      });
    });

    it('skips a defeated combatant and goes round the order', () => {
      const e = encounter({ combatants: order, currentCombatantId: 'pen' });
      expect(nextCombatant(e)?.label).toBe('Brisa');
    });

    it('says "Vez do mestre" for a hidden turn, with no name and no next', () => {
      const banner = turnBanner(
        encounter({ combatants: order, currentCombatantId: '', masterTurn: true }),
      );
      expect(banner.title).toBe('Vez do mestre');
      expect(banner.who).toBeNull();
      expect(banner.next).toBeNull();
      expect(banner.nextIsMine).toBe(false);
    });

    it('names a woman, a creature and nobody on a turn of one', () => {
      const brisa = combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER });
      const wolf = combatant({
        id: 'w',
        label: 'Lobo atroz',
        kind: CombatantKind.CREATURE,
        ownerCharacterId: 'char-b',
      });
      const e = (current: string) =>
        encounter({ combatants: [brisa, wolf, order[1]], currentCombatantId: current });
      expect(turnBanner(e('b')).title).toBe('Vez da Brisa');
      expect(turnBanner(e('w')).title).toBe(creatureTurnTitle(e('w'), [wolf], false));
      expect(turnBanner(e('')).title).toBe('Ninguém está na vez');
      expect(turnBanner(e('')).who).toBeNull();
    });

    it('names a joint turn: the members, the copies of one NPC, and the own part that has not ended', () => {
      const P = CombatantKind.PLAYER;
      const joint = (members: ReturnType<typeof combatant>[]) =>
        encounter({
          combatants: members,
          turnGroupIds: members.map((m) => m.id),
          currentCombatantId: members[0].id,
        });
      const players = [
        combatant({ id: 'b', label: 'Brisa', kind: P, initiative: 19 }),
        combatant({ id: 't', label: 'Toren', kind: P, initiative: 19 }),
      ];
      expect(turnBanner(joint(players))).toMatchObject({
        title: 'Vez de Brisa e Toren',
        mine: false,
        ownEnded: false,
      });
      const goblins = [
        combatant({ id: 'g1', label: 'Goblin 1', initiative: 12 }),
        combatant({ id: 'g2', label: 'Goblin 2', initiative: 12 }),
      ];
      expect(turnBanner(joint(goblins)).title).toBe('Vez dos Goblins');
      const own = turnBanner(
        joint([
          combatant({ id: 'b', label: 'Brisa', kind: P, initiative: 19, mine: true }),
          players[1],
        ]),
      );
      expect(own).toMatchObject({ title: 'Sua vez, Brisa', mine: true, ownEnded: false });
      expect(own.who?.id).toBe('b');
      const creatures = [
        combatant({
          id: 'w1',
          label: 'Lobo 1',
          kind: CombatantKind.CREATURE,
          ownerCharacterId: 'x',
        }),
        combatant({
          id: 'w2',
          label: 'Lobo 2',
          kind: CombatantKind.CREATURE,
          ownerCharacterId: 'x',
        }),
      ];
      const ce = joint(creatures);
      expect(turnBanner(ce).title).toBe(creatureTurnTitle(ce, creatures, false));
    });

    it('has no next when it is the only one standing', () => {
      const alone = encounter({ combatants: [order[0]], currentCombatantId: 'brisa' });
      expect(nextCombatant(alone)).toBeNull();
    });
  });
});
