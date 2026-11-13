import { SpellAreaShape } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  SpellZoneMovement,
  SpellZoneVisibility,
  WallSide,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter, spellZone } from './combat-testing';
import {
  zoneView as rawView,
  zoneViews as rawViews,
  type ZoneView,
  type ZoneViewer,
} from './zones';

// The app keeps a number and its unit (and a dot and its word) together with no-break spaces; the specs read plain spaces.
const plain = <T>(v: T): T => JSON.parse(JSON.stringify(v).replace(/\u00a0/g, ' ')) as T;
const zoneView = (...args: Parameters<typeof rawView>): ZoneView => plain(rawView(...args));
const zoneViews = (...args: Parameters<typeof rawViews>): ZoneView[] => plain(rawViews(...args));

const master: ZoneViewer = { master: true, combatants: [] };
const player: ZoneViewer = { master: false, combatants: [] };

describe('the label and the detail line of a zone', () => {
  it('names the spell and its caster', () => {
    expect(zoneView(spellZone({ id: 'z' }), master).label).toBe('Raio Lunar · Bartolo');
  });

  it('carries the spell name alone, for the legend', () => {
    expect(zoneView(spellZone({ id: 'z' }), master).spellName).toBe('Raio Lunar');
  });

  it('says the shape and size in meters, the save, the damage and the half', () => {
    const v = zoneView(spellZone({ id: 'z' }), master);
    expect(v.pieces).toEqual([
      'Cilindro de 1,5 m de raio',
      'Constituição CD 14',
      '2d10 radiante, metade se passar',
    ]);
    expect(v.detail).toBe(
      'Cilindro de 1,5 m de raio · Constituição CD 14 · 2d10 radiante, metade se passar',
    );
  });

  it('says a cube by its side and leaves the half out when a pass takes all the damage', () => {
    const v = zoneView(
      spellZone({ id: 'z', shape: SpellAreaShape.CUBE, sizeFt: 20, halfOnSave: false }),
      master,
    );
    expect(v.pieces[0]).toBe('Cubo de 6 m');
    expect(v.pieces[2]).toBe('2d10 radiante');
  });

  it('names the ability from its key when the server sends the key and an empty name', () => {
    const v = zoneView(spellZone({ id: 'z', saveAbility: 'con', saveAbilityNamePt: '' }), master);
    expect(v.pieces[1]).toBe('Constituição CD 14');
    expect(v.detail).toBe(
      'Cilindro de 1,5 m de raio · Constituição CD 14 · 2d10 radiante, metade se passar',
    );
  });

  it('shows the save without a number when the server sent no DC', () => {
    const v = zoneView(spellZone({ id: 'z', saveDc: undefined }), master);
    expect(v.pieces[1]).toBe('Constituição');
  });

  it('says "sem teste" for damage with no save, and nothing for a zone with neither', () => {
    const noSave = zoneView(
      spellZone({
        id: 'z',
        saveAbility: '',
        saveAbilityNamePt: '',
        saveDc: undefined,
        halfOnSave: false,
        damageDice: '4d4',
        damageTypeNamePt: 'cortante',
      }),
      master,
    );
    expect(noSave.pieces).toEqual(['Cilindro de 1,5 m de raio', '4d4 cortante, sem teste']);
    const web = zoneView(
      spellZone({ id: 'z', damageDice: '', damageTypeNamePt: '', damageTypeKey: '' }),
      master,
    );
    expect(web.pieces).toEqual(['Cilindro de 1,5 m de raio', 'Constituição CD 14']);
  });

  it('joins the move per turn to the shape and lists the notes in words', () => {
    const v = zoneView(
      spellZone({
        id: 'z',
        movesPerTurnFt: 10,
        notes: ['restrains', 'difficult_terrain', 'speed_halved', 'unknown'],
      }),
      master,
    );
    expect(v.pieces[0]).toBe('Cilindro de 1,5 m de raio, anda 3 m por turno');
    expect(v.pieces.slice(3)).toEqual(['contido', 'terreno difícil', 'velocidade pela metade']);
  });
});

describe("the master's calls", () => {
  const call = (key: string, viewer: ZoneViewer) =>
    zoneView(spellZone({ id: 'z', notes: [key] }), viewer).reminder;

  it('gives the master one line for Darkness, Fog Cloud and Stinking Cloud', () => {
    expect(call('master_light', master)).toContain('a luz de uma magia de nível 2 ou menor');
    expect(call('master_wind', master)).toContain('vento forte');
    expect(call('master_breath', master)).toContain('não precisa respirar');
  });

  it('gives a player none, and none for a note it does not know', () => {
    expect(call('master_light', player)).toBe('');
    expect(call('unknown', master)).toBe('');
  });
});

describe('the rounds left and the concentration', () => {
  const rounds = (n: number) => zoneView(spellZone({ id: 'z', roundsLeft: n }), master).roundsLeft;

  it('counts rounds under ten and minutes from ten (a round is 6 seconds)', () => {
    expect(rounds(1)).toBe('resta 1 rodada');
    expect(rounds(9)).toBe('restam 9 rodadas');
    expect(rounds(10)).toBe('restam 1 min');
    expect(rounds(100)).toBe('restam 10 min');
    expect(rounds(599)).toBe('restam 59 min');
  });

  it('says nothing for 0 (no count, or hidden): never "restam 0"', () => {
    expect(rounds(0)).toBe('');
  });

  it('names whose concentration holds it, or nothing for a spell that does not need it', () => {
    expect(zoneView(spellZone({ id: 'z' }), master).concentration).toBe('Concentração de Bartolo');
    expect(zoneView(spellZone({ id: 'z', concentration: false }), master).concentration).toBe('');
  });
});

describe('the occupants', () => {
  const tor = combatant({ id: 'tor', label: 'Tor' });
  const jabuti = combatant({ id: 'jabuti', label: 'Jabuti' });
  const hidden = combatant({ id: 'g', label: 'Goblin', hidden: true });
  const crowd = spellZone({ id: 'z', occupantIds: ['tor', 'jabuti', 'g', 'gone'] });

  it('lists the ones the viewer may see, in the order sent', () => {
    const viewer = { master: false, combatants: [tor, jabuti, hidden] };
    expect(zoneView(crowd, viewer).occupants).toBe('Aqui: Tor, Jabuti');
  });

  it('shows a hidden creature to the master only, and never a combatant it does not know', () => {
    const viewer = { master: true, combatants: [tor, jabuti, hidden] };
    expect(zoneView(crowd, viewer).occupants).toBe('Aqui: Tor, Jabuti, Goblin');
  });

  it('says nothing for an empty zone', () => {
    expect(zoneView(spellZone({ id: 'z' }), player).occupants).toBe('');
  });
});

describe('visibility and hidden details', () => {
  it('marks a zone only the master sees', () => {
    const v = zoneView(spellZone({ id: 'z', visibility: SpellZoneVisibility.MASTER_ONLY }), master);
    expect(v.masterOnly).toBe('Só você vê');
    expect(v.dashed).toBe(true);
    const open = zoneView(spellZone({ id: 'z' }), master);
    expect(open.masterOnly).toBe('');
    expect(open.dashed).toBe(false);
  });

  // The server empties these fields for a player reading another caster's zone; a client that
  // still received them (a bug) must not print them.
  const leaky = spellZone({
    id: 'z',
    detailsHidden: true,
    roundsLeft: 6,
    movesPerTurnFt: 10,
    notes: ['restrains'],
  });

  it('reads name, area and caster only when the details are hidden', () => {
    const v = zoneView(leaky, player);
    expect(v.label).toBe('Raio Lunar · Bartolo');
    expect(v.pieces).toEqual(['Cilindro de 1,5 m de raio']);
    expect(v.roundsLeft).toBe('');
    expect(v.concentration).toBe('Concentração de Bartolo');
    expect(JSON.stringify(v)).not.toMatch(/Constituição|CD 14|2d10|radiante|contido|restam|anda/);
  });

  it('is the same for the master reading hidden details (the server sends none)', () => {
    expect(zoneView(leaky, master).pieces).toEqual(['Cilindro de 1,5 m de raio']);
  });

  it('keeps the squares and the origin for the map', () => {
    const v = zoneView(spellZone({ id: 'z', squares: [{ col: 1, row: 2 }] } as never), player);
    expect(v.origin).toEqual({ col: 8, row: 6 });
    expect(v.squares).toEqual([{ col: 1, row: 2 }]);
  });
});

describe('the letter and the wall of a zone', () => {
  it('letters the zones A, B, C in the order of the list, then A2 after Z', () => {
    const zones = Array.from({ length: 28 }, (_, i) => spellZone({ id: `z${i}` }));
    const letters = zoneViews(encounter({ combatants: [], zones }), true).map((v) => v.letter);
    expect(letters.slice(0, 3)).toEqual(['A', 'B', 'C']);
    expect(letters[25]).toBe('Z');
    expect(letters[26]).toBe('A2');
    expect(letters[27]).toBe('B2');
  });

  it('carries the burning side and the wall side, empty for a zone that is no wall', () => {
    const plainZone = zoneView(spellZone({ id: 'z' }), master);
    expect(plainZone.burningSquares).toEqual([]);
    expect(plainZone.wallSide).toBe(WallSide.UNSPECIFIED);
    const wall = zoneView(
      spellZone({
        id: 'w',
        spellKey: 'spell:wall-of-fire',
        squares: [{ col: 2, row: 2 }],
        burningSquares: [
          { col: 2, row: 1 },
          { col: 3, row: 1 },
        ],
        wallSide: WallSide.LEFT,
      }),
      player,
    );
    expect(wall.burningSquares).toEqual([
      { col: 2, row: 1 },
      { col: 3, row: 1 },
    ]);
    expect(wall.wallSide).toBe(WallSide.LEFT);
  });
});

describe('who can move or end a zone', () => {
  const moves = spellZone({ id: 'z', movement: SpellZoneMovement.ACTION });

  it('lets the master move a zone that moves by an action, and end any zone', () => {
    const v = zoneView(moves, master);
    expect(v.canMove).toBe(true);
    expect(v.canEnd).toBe(true);
  });

  it('gives a player neither (this stage reads only the master as the one who acts)', () => {
    const v = zoneView(moves, player);
    expect(v.canMove).toBe(false);
    expect(v.canEnd).toBe(false);
  });

  it.each([
    SpellZoneMovement.FIXED,
    SpellZoneMovement.FOLLOWS_CASTER,
    SpellZoneMovement.DRIFTS,
    SpellZoneMovement.UNSPECIFIED,
  ])('does not offer a move for movement %s', (movement) => {
    const v = zoneView(spellZone({ id: 'z', movement }), master);
    expect(v.canMove).toBe(false);
    expect(v.canEnd).toBe(true);
  });
});

describe('the zones of an encounter', () => {
  it('reads encounter.zones in order, and nothing for no combat or no zones', () => {
    const e = encounter({
      zones: [
        spellZone({ id: 'a' }),
        spellZone({ id: 'b', spellNamePt: 'Teia', ownerLabel: 'Pensantus' }),
      ],
    });
    expect(zoneViews(e, true).map((z) => z.label)).toEqual([
      'Raio Lunar · Bartolo',
      'Teia · Pensantus',
    ]);
    expect(zoneViews(null, true)).toEqual([]);
    expect(zoneViews(encounter(), true)).toEqual([]);
  });

  it('resolves the occupants against the combatants of that encounter', () => {
    const e = encounter({
      combatants: [combatant({ id: 'tor', label: 'Tor' })],
      zones: [spellZone({ id: 'a', occupantIds: ['tor'] })],
    });
    expect(zoneViews(e, false)[0].occupants).toBe('Aqui: Tor');
  });
});
