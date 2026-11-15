import { describe, expect, it } from 'vitest';

import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';
import { type MountChoice, MountRefusal } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  dismountCard,
  dismountLead,
  dismountRefusalText,
  mountedDetail,
  mountedNotice,
  mountRefusalText,
  mountRows,
  nearestLanding,
  rideDetail,
} from './mount-plan';

const combatant = (id: string, label: string, col = 0, row = 0, mountedOnId = '') =>
  ({ id, label, col, row, mountedOnId }) as unknown as Combatant;
const choice = (fields: Partial<MountChoice>) =>
  ({
    combatantId: 'c',
    costDft: 125,
    refusal: MountRefusal.UNSPECIFIED,
    missingDft: 0,
    ...fields,
  }) as MountChoice;
const here = { col: 0, row: 0 };
/** What a person reads: a no-break space is a space. */
const plain = (text: string) => text.replace(/ /g, ' ');

describe('mount rows', () => {
  const cs = [
    combatant('m', 'Mastim', 1, 0),
    combatant('c', 'Cavalo de guerra', 3, 0),
    combatant('t', 'Tor', 3, 0, 'c'),
  ];
  const why = (fields: Partial<MountChoice>, all = cs) =>
    plain(mountRefusalText(choice(fields), all, here));

  it('says the cost of a mount that can be mounted', () => {
    const [row] = mountRows([choice({ combatantId: 'm' })], cs, here);
    expect({ ...row, line: plain(row.line) }).toEqual({
      id: 'm',
      name: 'Mastim',
      initial: 'M',
      line: 'Ao lado · custa 3,8 m',
      refused: false,
    });
  });

  it('words each refusal the server gives', () => {
    expect(why({ refusal: MountRefusal.TOO_SMALL })).toBe(
      'Pequeno demais para você: precisa ser maior.',
    );
    expect(why({ refusal: MountRefusal.TOO_FAR })).toBe('A 4,5 m: chegue ao lado dele primeiro.');
    expect(why({ refusal: MountRefusal.NO_MOVEMENT, costDft: 125, missingDft: 50 })).toBe(
      'Falta movimento: custa 3,8 m, restam 2,3 m.',
    );
    expect(why({ refusal: MountRefusal.MOVE_USED })).toBe(
      'Você já montou ou desmontou neste movimento.',
    );
    expect(why({ refusal: MountRefusal.TAKEN })).toBe('Já leva Tor.');
  });

  it('does not name a rider the viewer does not see', () => {
    expect(why({ refusal: MountRefusal.TAKEN }, [combatant('c', 'Cavalo de guerra')])).toBe(
      'Já leva alguém.',
    );
  });

  it('marks a refused row, and leaves out a mount the page has no combatant for', () => {
    const [row] = mountRows([choice({ combatantId: 'c', refusal: MountRefusal.TAKEN })], cs, here);
    expect(row.refused).toBe(true);
    expect(mountRows([choice({ combatantId: 'gone' })], cs, here)).toEqual([]);
  });

  it('words why getting off is refused', () => {
    const off = (r: MountRefusal, cost = 0, missing = 0) =>
      plain(dismountRefusalText(r, cost, missing, 'Tor'));
    expect(off(MountRefusal.NO_ROOM)).toBe('Nenhum quadrado livre ao lado do cavalo.');
    expect(off(MountRefusal.NO_MOVEMENT, 150, 30)).toBe(
      'Falta movimento: desmontar custa 4,5 m, restam 3,6 m a Tor.',
    );
    expect(off(MountRefusal.UNSPECIFIED)).toBe('');
  });
});

describe('the words of a ride and of a landing', () => {
  it('says whose movement it is and what is left for getting off', () => {
    expect(plain(mountedNotice('Cavalo de guerra'))).toBe('Montado no Cavalo de guerra.');
    expect(plain(mountedNotice('Aranha'))).toBe('Montado na Aranha.');
    expect(mountedDetail('Tor')).toBe('O movimento é dele, e Tor vai junto no mesmo quadrado.');
    expect(plain(rideDetail(300, 'Cavalo de guerra', 90, 'Tor'))).toBe(
      'Depois restam 9,0 m ao Cavalo de guerra. Os 2,7 m de Tor ficam para desmontar.',
    );
  });

  it('words the landing card and its lead', () => {
    const card = dismountCard(150, 300, 'Tor', true);
    expect(plain(card.title)).toBe('Desmontar aqui · 4,5 m');
    expect(plain(card.detail)).toBe(
      'Depois restam 4,5 m a Tor, a pé. O cavalo volta à iniciativa dele.',
    );
    expect(plain(dismountCard(150, 300, 'Tor', false).detail)).toBe(
      'Depois restam 4,5 m a Tor, a pé.',
    );
    expect(plain(dismountLead(150, 'Tor'))).toBe(
      'Desmontar custa 4,5 m, metade do deslocamento de Tor. Toque num quadrado ao lado do cavalo.',
    );
  });

  it('picks the landing square nearest to where the arrows pointed', () => {
    const landing = [
      { col: 2, row: 2 },
      { col: 4, row: 2 },
    ];
    expect(nearestLanding({ col: 5, row: 2 }, landing)).toEqual({ col: 4, row: 2 });
    expect(nearestLanding({ col: 0, row: 0 }, [])).toBeNull();
  });
});
