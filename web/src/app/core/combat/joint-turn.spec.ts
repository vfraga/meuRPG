import { CombatantKind, EncounterStatus } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from './combat-testing';
import { turnBanner } from './combat-view';
import { distanceText } from '../units';
import {
  acts,
  actingIds,
  afterTurn,
  jointTurn,
  leftSentence,
  masterActor,
  missingLine,
  npcPlural,
  orderItems,
  passNote,
  playsBefore,
} from './joint-turn';

const P = CombatantKind.PLAYER;

describe('joint turns (MR-013)', () => {
  const brisa = combatant({ id: 'b', label: 'Brisa', kind: P, initiative: 19 });
  const toren = combatant({ id: 't', label: 'Toren', kind: P, initiative: 19, mine: true });
  const cap = combatant({ id: 'c', label: 'Capitão Goblin', initiative: 16 });
  const pens = combatant({ id: 'p', label: 'Pensantus', kind: P, initiative: 14 });
  const g1 = combatant({ id: 'g1', label: 'Goblin 1', initiative: 12 });
  const g2 = combatant({ id: 'g2', label: 'Goblin 2', initiative: 12 });
  const all = [brisa, toren, cap, pens, g1, g2];

  describe('orderItems', () => {
    it('boxes every run with the same total for the master, players and NPCs alike', () => {
      const items = orderItems(encounter({ combatants: all }), true);
      expect(
        items.map((i) =>
          i.kind === 'group' ? i.members.map((m) => m.label).join('+') : i.combatant.label,
        ),
      ).toEqual(['Brisa+Toren', 'Capitão Goblin', 'Pensantus', 'Goblin 1+Goblin 2']);
    });

    it('never boxes NPCs for a player: they have no total (RN-20)', () => {
      const noTotal = [
        brisa,
        toren,
        cap,
        pens,
        { ...g1, initiative: undefined },
        { ...g2, initiative: undefined },
      ] as typeof all;
      const items = orderItems(encounter({ combatants: noTotal }), false);
      expect(items.filter((i) => i.kind === 'group')).toHaveLength(1);
    });

    describe('a rider and its mount', () => {
      const rider = combatant({ id: 'r', label: 'Tor', kind: P, initiative: 17, mountedOnId: 'h' });
      const horse = (over = {}) =>
        combatant({ id: 'h', label: 'Cavalo de guerra', mountKind: 'controlled', ...over });
      const zombie = combatant({ id: 'z', label: 'Zumbi', initiative: 15 });
      const names = (e: ReturnType<typeof encounter>, master: boolean) =>
        orderItems(e, master).map((i) =>
          i.kind === 'group'
            ? `${i.mounted ? 'montado:' : ''}${i.members.map((m) => m.label).join('+')}`
            : i.combatant.label,
        );

      it('boxes the pair as "Montado", for the master and for a player who is not told the total of the mount', () => {
        const e = encounter({ combatants: [rider, horse(), zombie] });
        expect(names(e, true)).toEqual(['montado:Tor+Cavalo de guerra', 'Zumbi']);
        expect(names(e, false)).toEqual(['montado:Tor+Cavalo de guerra', 'Zumbi']);
        const box = orderItems(e, false)[0];
        expect(box.kind === 'group' ? box.total : null).toBe(17);
      });

      it('leaves a mount that keeps its own turn in its own row for the master', () => {
        const e = encounter({
          combatants: [rider, horse({ mountKind: 'independent', initiative: 9 }), zombie],
        });
        expect(names(e, true)).toEqual(['Tor', 'Cavalo de guerra', 'Zumbi']);
      });

      it('does not box a hidden mount: the link is not sent, so the rider is plain', () => {
        const plain = { ...rider, mountedOnId: '' } as typeof rider;
        expect(names(encounter({ combatants: [plain, zombie] }), false)).toEqual(['Tor', 'Zumbi']);
      });
    });

    it('marks the box on turn', () => {
      const e = encounter({ combatants: all, turnGroupIds: ['b', 't'], currentCombatantId: 'b' });
      const group = orderItems(e, true)[0];
      expect(group.kind === 'group' && group.onTurn && group.total === 19).toBe(true);
    });
  });

  describe('the turn that is running', () => {
    it('is null for a turn of one', () => {
      expect(
        jointTurn(encounter({ combatants: all, turnGroupIds: ['c'], currentCombatantId: 'c' })),
      ).toBeNull();
    });

    it('lists who acts and who ended', () => {
      const e = encounter({
        combatants: [brisa, { ...toren, turnPartEnded: true }, cap],
        turnGroupIds: ['b', 't'],
        currentCombatantId: 'b',
      });
      const joint = jointTurn(e)!;
      expect(joint.acting.map((c) => c.label)).toEqual(['Brisa']);
      expect(joint.ended.map((c) => c.label)).toEqual(['Toren']);
      expect(joint.npcOnly).toBe(false);
    });

    it('says the master is missing when a hidden member still acts', () => {
      const e = encounter({
        combatants: [{ ...toren, turnPartEnded: true }],
        turnGroupIds: ['t'],
        masterTurn: true,
      });
      expect(jointTurn(e)?.waitsForMaster).toBe(true);
      expect(missingLine([], true)).toBe(
        'Falta o mestre. O turno passa quando ele encerrar a parte dele.',
      );
    });
  });

  describe('the banner', () => {
    it('reads "Sua vez" for a member who acts, and "Você encerrou a sua parte" after', () => {
      const acting = encounter({
        combatants: all,
        turnGroupIds: ['b', 't'],
        currentCombatantId: 'b',
      });
      expect(turnBanner(acting).title).toBe('Sua vez, Toren');
      expect(turnBanner(acting).mine).toBe(true);
      const ended = encounter({
        combatants: [brisa, { ...toren, turnPartEnded: true }, cap],
        turnGroupIds: ['b', 't'],
        currentCombatantId: 'b',
      });
      expect(turnBanner(ended).title).toBe('Você encerrou a sua parte');
      expect(turnBanner(ended).mine).toBe(false);
    });

    it('names the group for a player outside it, and a group of NPCs by its plural, never one of them', () => {
      const outside = encounter({
        combatants: [brisa, { ...toren, mine: false }, pens, cap],
        turnGroupIds: ['b', 't'],
        currentCombatantId: 'b',
      });
      expect(turnBanner(outside).title).toBe('Vez de Brisa e Toren');
      const goblins = encounter({
        combatants: [{ ...pens, mine: true }, g1, g2],
        turnGroupIds: ['g1', 'g2'],
        currentCombatantId: '',
      });
      expect(turnBanner(goblins).title).toBe('Vez dos Goblins');
    });
  });

  it('plurals', () => {
    expect(npcPlural(['Goblin 1', 'Goblin 2'])).toBe('os Goblins');
    expect(npcPlural(['Capitão Goblin', 'Goblin 1'])).toBeNull();
    expect(npcPlural(['Goblin 1'])).toBeNull();
    expect(npcPlural(['Lutador 1', 'Lutador 2'])).toBe('os Lutadores');
    expect(npcPlural(['Capataz 1', 'Capataz 2'])).toBe('os Capatazes');
    expect(npcPlural(['Gás 1', 'Gás 2'])).toBe('os Gás');
    expect(npcPlural(['Espada voadora 2', 'Espada voadora 3'])).toBe('as Espadas voadoras');
    expect(npcPlural(['Cobra de pedra 1', 'Cobra de pedra 2'])).toBe('as Cobras de pedra');
  });

  it('lets act only the members whose part has not ended', () => {
    const e = encounter({
      combatants: [brisa, { ...toren, turnPartEnded: true }, cap],
      turnGroupIds: ['b', 't'],
      currentCombatantId: 'b',
    });
    expect(actingIds(e)).toEqual(['b']);
    expect(acts(e, brisa)).toBe(true);
    expect(acts(e, toren)).toBe(false);
    expect(acts(e, cap)).toBe(false);
  });

  it('says who comes after the group, naming an NPC-only group by its plural', () => {
    const e = encounter({
      combatants: [brisa, toren, pens, g1, g2],
      turnGroupIds: ['b', 't'],
      currentCombatantId: 'b',
      npcOnlyGroups: [{ combatantIds: ['g1', 'g2'] }] as never,
    });
    expect(afterTurn(e)?.name).toBe('Pensantus');
    const later = encounter({
      combatants: [pens, g1, g2, brisa],
      turnGroupIds: ['p'],
      currentCombatantId: 'p',
      npcOnlyGroups: [{ combatantIds: ['g1', 'g2'] }] as never,
    });
    expect(afterTurn(later)).toMatchObject({ name: 'os Goblins', plural: true });
    expect(playsBefore(encounter({ combatants: [cap, { ...pens, mine: true }] }))).toBe(
      'do Capitão Goblin',
    );
  });

  it('names the next group from the totals on the master copy, which has no npc_only_groups', () => {
    const e = encounter({
      combatants: [pens, g1, g2, brisa],
      turnGroupIds: ['p'],
      currentCombatantId: 'p',
    });
    expect(afterTurn(e)).toMatchObject({ name: 'os Goblins', plural: true, mine: false });
    const mixed = encounter({
      combatants: [pens, cap, { ...g1, initiative: 16 }, brisa],
      turnGroupIds: ['p'],
      currentCombatantId: 'p',
    });
    expect(afterTurn(mixed)?.name).toBe('Capitão Goblin e Goblin 1');
    const oneLeft = encounter({
      combatants: [pens, g1, { ...g2, defeated: true }, brisa],
      turnGroupIds: ['p'],
      currentCombatantId: 'p',
    });
    expect(afterTurn(oneLeft)).toMatchObject({ name: 'Goblin 1', plural: false });
  });

  it('writes the lines of the footer and the question', () => {
    expect(passNote(['Brisa'], false)).toBe('O turno passa quando você e a Brisa encerrarem.');
    expect(passNote(['Brisa'], true)).toBe(
      'O turno passa quando você, a Brisa e o mestre encerrarem.',
    );
    expect(missingLine(['Brisa'], false)).toBe(
      'Falta a Brisa. O turno passa quando ela encerrar a parte dela.',
    );
    expect(distanceText(30)).toBe('9 m · 6 quadrados');
    expect(
      leftSentence(
        combatant({ id: 'x', label: 'X', movementLeftFt: 30, speedFt: 30, reactionUsed: true }),
      ),
    ).toBe('Ação, Ação bônus e 9 m · 6 quadrados');
  });

  describe('masterActor, the NPC the master acts for in a mixed joint turn', () => {
    const jabuti = combatant({ id: 'j', label: 'Jabuti', kind: P, initiative: 18 });
    const gob = combatant({ id: 'g', label: 'Goblin 2', initiative: 18 });
    const mixed = (over: object = {}) =>
      encounter({
        status: EncounterStatus.ACTIVE,
        combatants: [jabuti, gob],
        turnGroupIds: ['j', 'g'],
        currentCombatantId: 'j',
        ...over,
      });

    it('is the NPC although the player is the current combatant', () => {
      expect(masterActor(mixed())?.id).toBe('g');
    });

    it('is nothing once the NPC part ended, or when only players act', () => {
      expect(
        masterActor(mixed({ combatants: [jabuti, { ...gob, turnPartEnded: true }] })),
      ).toBeNull();
      expect(masterActor(mixed({ turnGroupIds: ['j'] }))).toBeNull();
    });
  });
});
