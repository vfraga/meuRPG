import { describe, expect, it } from 'vitest';

import { combatant, encounter } from './combat-testing';
import {
  boxedPair,
  followsRider,
  isPair,
  mountLine,
  mountOf,
  mountedOn,
  ridingOn,
  riderOn,
} from './mount-view';

const tor = combatant({ id: 'tor', label: 'Tor', mountedOnId: 'cav' });
const horse = (kind = 'controlled') =>
  combatant({ id: 'cav', label: 'Cavalo de guerra', mountKind: kind });
const wight = combatant({ id: 'w', label: 'Wight', mountedOnId: 'esq' });
const skeleton = combatant({ id: 'esq', label: 'Esqueleto de cavalo', mountKind: 'independent' });
const spider = combatant({ id: 'ara', label: 'Aranha', mountKind: 'controlled' });
const rider = combatant({ id: 'r', label: 'Lia', mountedOnId: 'ara' });

describe('the link of a rider and its mount', () => {
  const e = encounter({ combatants: [tor, horse()] });

  it('reads the mount of the rider and the rider of the mount', () => {
    expect(mountOf(e, tor)?.id).toBe('cav');
    expect(riderOn(e, horse())?.id).toBe('tor');
    expect(mountOf(e, horse())).toBeUndefined();
    expect(isPair(tor, horse())).toBe(true);
    expect(isPair(tor, skeleton)).toBe(false);
  });

  it('boxes a pair only when the mount is known to be a trained one', () => {
    expect(boxedPair(tor, horse())).toBe(true);
    expect(boxedPair(tor, horse('independent'))).toBe(false);
    expect(boxedPair(tor, skeleton)).toBe(false);
  });

  it('does not box a mount with no kind: the screens do not guess the order from adjacency', () => {
    expect(boxedPair(tor, horse(''))).toBe(false);
    expect(boxedPair(wight, { ...skeleton, mountKind: '' } as typeof skeleton)).toBe(false);
  });

  it('says which mount follows its rider in a box', () => {
    const both = encounter({ combatants: [tor, horse(), wight, skeleton] });
    expect(followsRider(both, horse())).toBe(true);
    expect(followsRider(both, skeleton)).toBe(false);
    expect(followsRider(both, tor)).toBe(false);
  });
});

describe('the line a row says about the link', () => {
  const e = encounter({ combatants: [tor, horse(), wight, skeleton, rider, spider] });

  it('names the rider on the mount row of a box, and says nothing on the rider row', () => {
    expect(mountLine(e, horse(), false, true)).toBe('Montaria de Tor');
    expect(mountLine(e, horse(), true, true)).toBe('Adestrada · leva Tor');
    expect(mountLine(e, tor, false, true)).toBe('');
  });

  it("links the two rows of a mount that keeps its own turn, with the mount name's article", () => {
    expect(mountLine(e, wight, true, false)).toBe('Montado no Esqueleto de cavalo');
    expect(mountLine(e, skeleton, true, false)).toBe('Leva o Wight · independente');
    expect(mountLine(e, rider, true, false)).toBe('Montado na Aranha');
    expect(mountLine(e, spider, true, false)).toBe('Leva a Lia · independente');
  });

  it('says "independente" only when the kind is known', () => {
    const other = encounter({
      combatants: [wight, { ...skeleton, mountKind: '' } as typeof skeleton],
    });
    expect(mountLine(other, { ...skeleton, mountKind: '' } as typeof skeleton, false, false)).toBe(
      'Leva o Wight',
    );
    expect(mountLine(e, skeleton, false, false)).toBe('Leva o Wight · independente');
  });

  it('says nothing about a combatant that is no part of a pair', () => {
    expect(
      mountLine(
        encounter({ combatants: [wight] }),
        combatant({ id: 'z', label: 'Zumbi' }),
        true,
        false,
      ),
    ).toBe('');
  });
});

describe('the words of who rides what', () => {
  it('opens a line with "Montado no Cavalo de guerra" and goes on inside a sentence with "montado no"', () => {
    expect(mountedOn(horse())).toBe('Montado no Cavalo de guerra');
    expect(ridingOn(horse())).toBe('montado no Cavalo de guerra');
    expect(ridingOn(spider)).toBe('montado na Aranha');
  });
});
