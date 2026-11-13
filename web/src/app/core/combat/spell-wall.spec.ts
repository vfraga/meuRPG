import { WallSide } from '../../../gen/meurpg/play/v1/combat_pb';
import {
  WALL_SPELL_KEY,
  laysWall,
  lengthNote,
  lengthText,
  newWall,
  askable,
  reorient,
  sideChoices,
  sidePicked,
  stopsText,
  sideNote,
  stepLength,
  switchShape,
  wallBody,
  wallHint,
  wallZoneView,
} from './spell-wall';

const plain = (t: string) => t.replace(/\u00a0/g, ' ');

describe('spell-wall: what the spell lays', () => {
  it('knows the spell that lays a wall by its catalog key, and no other', () => {
    expect(laysWall('spell:wall-of-fire')).toBe(true);
    expect(laysWall('spell:fireball')).toBe(false);
  });

  it('starts a line eastward at the full 60 ft and a ring at 20 ft, with no side picked', () => {
    expect(newWall(false)).toEqual({
      ring: false,
      direction: { dx: 1, dy: 0 },
      lengthFt: 60,
      side: WallSide.UNSPECIFIED,
    });
    expect(newWall(true)).toEqual({
      ring: true,
      direction: null,
      lengthFt: 20,
      side: WallSide.UNSPECIFIED,
    });
  });

  it('steps a line a square (5 ft, 1,5 m) at a time from 5 to 60 ft, and a ring between its only two sizes, 10 and 20 ft (the server rounds 5 to 10 and 15 to 20 into one ring)', () => {
    const line = { ...newWall(false), lengthFt: 40 };
    expect(stepLength(line, -1).lengthFt).toBe(35);
    expect(stepLength(line, 1).lengthFt).toBe(45);
    expect(stepLength({ ...line, lengthFt: 5 }, -1).lengthFt).toBe(5);
    expect(stepLength({ ...line, lengthFt: 60 }, 1).lengthFt).toBe(60);
    const ring = newWall(true);
    expect(stepLength(ring, 1).lengthFt).toBe(20);
    expect(stepLength(ring, -1).lengthFt).toBe(10);
    expect(stepLength({ ...ring, lengthFt: 10 }, -1).lengthFt).toBe(10);
    expect(stepLength({ ...ring, lengthFt: 10 }, 1).lengthFt).toBe(20);
  });

  it('says the length in meters and how many squares it is', () => {
    expect(plain(lengthText({ ...newWall(false), lengthFt: 40 }))).toBe('12 m');
    expect(plain(lengthText({ ...newWall(false), lengthFt: 5 }))).toBe('1,5 m');
    expect(plain(lengthNote({ ...newWall(false), lengthFt: 40 }))).toBe(
      'De 1,5 m a 18 m: 8 quadrados.',
    );
    expect(plain(lengthNote(newWall(true)))).toBe('3 m ou 6 m de diâmetro.');
  });

  it('names the two sides of a line by where they lie on the screen, from the way it runs', () => {
    const labels = (dx: number, dy: number) =>
      sideChoices({ ...newWall(false), direction: { dx, dy } }).map((c) => [c.side, c.label]);
    expect(labels(1, 0)).toEqual([
      [WallSide.LEFT, 'Acima'],
      [WallSide.RIGHT, 'Abaixo'],
    ]);
    expect(labels(-1, 0)).toEqual([
      [WallSide.LEFT, 'Abaixo'],
      [WallSide.RIGHT, 'Acima'],
    ]);
    expect(labels(0, 1)).toEqual([
      [WallSide.LEFT, 'À direita'],
      [WallSide.RIGHT, 'À esquerda'],
    ]);
    expect(labels(0, -1)).toEqual([
      [WallSide.LEFT, 'À esquerda'],
      [WallSide.RIGHT, 'À direita'],
    ]);
    expect(labels(1, -1)).toEqual([
      [WallSide.LEFT, 'Acima, à esquerda'],
      [WallSide.RIGHT, 'Abaixo, à direita'],
    ]);
    expect(labels(1, 1)).toEqual([
      [WallSide.LEFT, 'Acima, à direita'],
      [WallSide.RIGHT, 'Abaixo, à esquerda'],
    ]);
    expect(labels(-1, 1)).toEqual([
      [WallSide.LEFT, 'Abaixo, à direita'],
      [WallSide.RIGHT, 'Acima, à esquerda'],
    ]);
    expect(labels(-1, -1)).toEqual([
      [WallSide.LEFT, 'Abaixo, à esquerda'],
      [WallSide.RIGHT, 'Acima, à direita'],
    ]);
  });

  it('offers Dentro and Fora for a ring, never left or right', () => {
    expect(sideChoices(newWall(true)).map((c) => [c.side, c.label])).toEqual([
      [WallSide.INNER, 'Dentro'],
      [WallSide.OUTER, 'Fora'],
    ]);
  });

  it('turning the wall takes the side back: the same way keeps it, another way asks again', () => {
    const picked = { ...newWall(false), side: WallSide.LEFT };
    expect(reorient(picked, { dx: 1, dy: 0 })).toBe(picked);
    expect(reorient(picked, { dx: 0, dy: 1 })).toEqual({
      ...picked,
      direction: { dx: 0, dy: 1 },
      side: WallSide.UNSPECIFIED,
    });
  });

  it('a side is picked or not, and a preview asks with a provisional side that is never sent as the cast', () => {
    expect(sidePicked(newWall(false))).toBe(false);
    expect(sidePicked({ ...newWall(false), side: WallSide.RIGHT })).toBe(true);
    expect(askable(newWall(false)).side).toBe(WallSide.LEFT);
    expect(askable(newWall(true)).side).toBe(WallSide.INNER);
    const picked = { ...newWall(false), side: WallSide.RIGHT };
    expect(askable(picked)).toBe(picked);
  });

  it('says a straight wall stopped at a wall of the map, in meters and squares', () => {
    const wall = { ...newWall(false), lengthFt: 40 };
    expect(plain(stopsText(wall, 8) ?? '')).toBe('');
    expect(plain(stopsText(wall, 6) ?? '')).toBe(
      'A muralha para na parede: ficou com 9 m (6 quadrados) em vez de 12 m.',
    );
    expect(plain(stopsText(wall, 1) ?? '')).toContain('ficou com 1,5 m (1 quadrado)');
    // A diagonal wall counts the squares the map's own distance allows: 60 ft are 9 along a diagonal.
    const diagonal = { ...newWall(false), direction: { dx: 1, dy: 1 } };
    expect(stopsText(diagonal, 9)).toBeNull();
    expect(stopsText(diagonal, 7)).not.toBeNull();
    expect(stopsText(newWall(true), 3)).toBeNull();
  });

  it('changing the shape resets the side and the direction and keeps the length inside the new size', () => {
    const line = { ...newWall(false), lengthFt: 40, side: WallSide.LEFT };
    const ring = switchShape(line, true);
    expect(ring).toEqual({ ring: true, direction: null, lengthFt: 20, side: WallSide.UNSPECIFIED });
    const back = switchShape({ ...ring, lengthFt: 10 }, false);
    expect(back.ring).toBe(false);
    expect(back.direction).toEqual({ dx: 1, dy: 0 });
    expect(back.side).toBe(WallSide.UNSPECIFIED);
    expect(back.lengthFt).toBe(10);
    // A line's length that is no ring's size lands on the nearest ring size.
    expect(switchShape({ ...line, lengthFt: 15 }, true).lengthFt).toBe(10);
    expect(switchShape({ ...line, lengthFt: 25 }, true).lengthFt).toBe(20);
    expect(switchShape(line, false)).toBe(line);
  });

  it('writes the request: a line has a direction, a ring has none, and the length is always explicit', () => {
    expect(
      wallBody({
        ...newWall(false),
        direction: { dx: 1, dy: -1 },
        lengthFt: 35,
        side: WallSide.RIGHT,
      }),
    ).toEqual({
      ring: false,
      direction: { dx: 1, dy: -1 },
      lengthFt: 35,
      side: WallSide.RIGHT,
    });
    const ring = wallBody({ ...newWall(true), side: WallSide.INNER });
    expect(ring).toEqual({ ring: true, lengthFt: 20, side: WallSide.INNER });
    expect('direction' in ring).toBe(false);
  });

  it('writes the note of the side with the dice of the slot, and the ring says which side', () => {
    expect(plain(sideNote({ ...newWall(false), side: WallSide.LEFT }, '5d8'))).toBe(
      'A seta no mapa mostra o lado: 5d8 de fogo para quem termina o turno ali, até 3 m.',
    );
    expect(plain(sideNote({ ...newWall(true), side: WallSide.INNER }, '6d8'))).toBe(
      'Quem termina o turno dentro do anel, ou entra na muralha, sofre 6d8 de fogo.',
    );
    expect(plain(sideNote(newWall(true), '6d8'))).toContain('no lado que queima');
    expect(plain(sideNote({ ...newWall(true), side: WallSide.OUTER }, ''))).toBe(
      'Quem termina o turno fora do anel, ou entra na muralha, sofre dano de fogo.',
    );
  });

  it('says how to place a line or a ring before and after the point is placed', () => {
    expect(wallHint(newWall(false), false)).toContain('Toque no começo e arraste para onde vai');
    expect(wallHint(newWall(false), true)).toContain('Começo marcado');
    expect(wallHint(newWall(true), false)).toContain('Toque no centro do anel');
    expect(wallHint(newWall(true), true)).toContain('Centro marcado');
  });

  it('makes the zone the map draws from the answer: the wall, the burning side and its side', () => {
    const view = wallZoneView(
      {
        origin: { col: 4, row: 3 },
        squares: [
          { col: 4, row: 3 },
          { col: 5, row: 3 },
        ],
        burningSquares: [{ col: 4, row: 4 }],
      },
      WallSide.LEFT,
    );
    expect(view.spellKey).toBe(WALL_SPELL_KEY);
    expect(view.origin).toEqual({ col: 4, row: 3 });
    expect(view.squares).toHaveLength(2);
    expect(view.burningSquares).toEqual([{ col: 4, row: 4 }]);
    expect(view.wallSide).toBe(WallSide.LEFT);
    // Until the side is picked the burning squares are not drawn.
    expect(
      wallZoneView(
        { squares: [{ col: 1, row: 1 }], burningSquares: [{ col: 1, row: 2 }] },
        WallSide.UNSPECIFIED,
      ).burningSquares,
    ).toEqual([]);
    expect(view.letter).toBe('');
  });
});
