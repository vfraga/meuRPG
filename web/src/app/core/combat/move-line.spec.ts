import { create } from '@bufbuild/protobuf';

import { CombatantSchema } from '../../../gen/meurpg/play/v1/combat_pb';
import { moveLine } from './move-line';

const own = (over: Parameters<typeof create<typeof CombatantSchema>>[1] = {}) =>
  create(CombatantSchema, {
    label: 'Brisa',
    speedDft: 300,
    movementLeftDft: 300,
    movementLeftFt: 30,
    ...over,
  });

/** The text as the eye reads it: a no-break space is a space. */
const plain = (t: string) => t.replace(/\u00a0/g, ' ');

describe('moveLine', () => {
  it('reads the mount’s movement for a rider, and names the mount', () => {
    const horse = own({
      label: 'Cavalo de guerra',
      speedDft: 335,
      movementLeftDft: 335,
      movementLeftFt: 33,
    });
    const line = moveLine(own({ movementLeftDft: 150, movementLeftFt: 5 }), false, horse);
    expect(plain(line.lead)).toBe('Restam 10,1 m ao Cavalo de guerra');
    expect(plain(line.left)).toBe('10,1 m');
    expect(line.percent).toBe(100);
    expect(line.canMove).toBe(true);
  });

  it('says there is no movement when the mount has none left', () => {
    const horse = own({ label: 'Cavalo de guerra', movementLeftDft: 0, movementLeftFt: 0 });
    const line = moveLine(own(), false, horse);
    expect([line.lead, line.canMove]).toEqual(['Sem movimento', false]);
  });

  it('says the metres left with one decimal and the squares', () => {
    const line = moveLine(own(), false);
    expect(
      [line.lead, line.rest, line.left, line.canMove].map((v) =>
        typeof v === 'string' ? plain(v) : v,
      ),
    ).toEqual(['Restam 9,0 m', '6 quadrados', '9,0 m', true]);
    expect(line.percent).toBe(100);
  });

  it('reads tenths of a foot', () => {
    const line = moveLine(own({ movementLeftDft: 229, movementLeftFt: 22 }), false);
    expect(plain(line.lead)).toBe('Restam 6,9 m');
    expect(plain(line.rest)).toBe('4 quadrados');
    expect(Math.round(line.percent)).toBe(76);
  });

  it('says the squares do not provoke after Desengajar', () => {
    expect(plain(moveLine(own({ disengaged: true }), false).rest)).toBe(
      '6 quadrados · sem provocar',
    );
  });

  it('has no move left', () => {
    expect(moveLine(own({ movementLeftDft: 0, movementLeftFt: 0 }), false)).toMatchObject({
      lead: 'Sem movimento',
      canMove: false,
      percent: 0,
    });
  });

  it('says a grapple makes the movement unavailable', () => {
    expect(moveLine(own(), true)).toMatchObject({
      lead: 'Indisponível: Agarrado.',
      canMove: false,
    });
    expect(moveLine(own({ label: 'Toren' }), true).lead).toBe('Indisponível: Agarrado.');
  });
});
