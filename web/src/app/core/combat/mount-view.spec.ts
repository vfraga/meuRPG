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

  it('boxes a controlled pair for the master and the pair that follows for a player', () => {
    expect(boxedPair(tor, horse(), true)).toBe(true);
    expect(boxedPair(tor, horse('independent'), true)).toBe(false);
    // The player is not told the mark of a mount that is not theirs: the empty mark still boxes.
    expect(boxedPair(tor, horse(''), false)).toBe(true);
    expect(boxedPair(tor, skeleton, false)).toBe(false);
    // The rider's player reads the mark: an independent mount is not boxed for them either.
    expect(boxedPair(tor, horse('independent'), false)).toBe(false);
  });

  it("reads an empty mark as unmarked for whoever is told the mark: the rider's player and the mount's owner", () => {
    const mine = { ...tor, mine: true } as typeof tor;
    expect(boxedPair(mine, horse(''), false)).toBe(false);
    const owned = { ...horse(''), controlledByMe: true } as ReturnType<typeof horse>;
    expect(boxedPair(tor, owned, false)).toBe(false);
    expect(boxedPair(tor, horse(''), true)).toBe(false);
  });

  it('boxes, for a third party, a mount right after its rider whatever its mark (a known limit: the mark is not theirs)', () => {
    // An independent NPC mount reads as '' to them, so it is boxed like a controlled one.
    expect(boxedPair(wight, { ...skeleton, mountKind: '' } as typeof skeleton, false)).toBe(true);
  });

  it('says which mount follows its rider in a box', () => {
    const both = encounter({ combatants: [tor, horse(), wight, skeleton] });
    expect(followsRider(both, horse(), true)).toBe(true);
    expect(followsRider(both, skeleton, true)).toBe(false);
    expect(followsRider(both, tor, true)).toBe(false);
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

  it('never tells a player the mark they cannot read', () => {
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
