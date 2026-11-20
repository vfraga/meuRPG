import { describe, expect, it } from 'vitest';

import { CombatantKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from './combat-testing';
import {
  boxedPair,
  directedMount,
  followsRider,
  isPair,
  mountLine,
  mountOf,
  mountedOn,
  ridingOn,
  riderOn,
} from './mount-view';

const tor = combatant({
  id: 'tor',
  label: 'Tor',
  mountedOnId: 'cav',
  kind: CombatantKind.PLAYER,
});
const horse = (kind = 'controlled') =>
  combatant({ id: 'cav', label: 'Cavalo de guerra', mountKind: kind });
const wight = combatant({ id: 'w', label: 'Wight', mountedOnId: 'esq' });
const skeleton = combatant({ id: 'esq', label: 'Esqueleto de cavalo', mountKind: 'independent' });
const spider = combatant({ id: 'ara', label: 'Aranha', mountKind: 'controlled' });
const rider = combatant({ id: 'r', label: 'Lia', mountedOnId: 'ara', kind: CombatantKind.PLAYER });

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

  it('reads the mount a rider directs, never an independent one', () => {
    expect(directedMount(e.combatants, tor)?.id).toBe('cav');
    expect(directedMount([tor, horse('independent')], tor)).toBeUndefined();
    expect(directedMount(e.combatants, horse())).toBeUndefined();
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

  it('names a creature that rides with its article, as the title does, and a character with none', () => {
    const goblin = combatant({
      id: 'g',
      label: 'Goblin 1',
      mountedOnId: 'cav',
      kind: CombatantKind.NPC,
    });
    const lia = combatant({
      id: 'l',
      label: 'Lia',
      mountedOnId: 'cav',
      kind: CombatantKind.PLAYER,
    });
    const withGoblin = encounter({ combatants: [goblin, horse()] });
    const withLia = encounter({ combatants: [lia, horse()] });
    expect(mountLine(withGoblin, horse(), false, true)).toBe('Montaria do Goblin 1');
    expect(mountLine(withGoblin, horse(), true, true)).toBe('Adestrada · leva o Goblin 1');
    expect(mountLine(withLia, horse(), false, true)).toBe('Montaria de Lia');
  });

  it("links the two rows of a mount that keeps its own turn, with the mount name's article", () => {
    expect(mountLine(e, wight, true, false)).toBe('Cavalgando o Esqueleto de cavalo');
    expect(mountLine(e, skeleton, true, false)).toBe('Leva o Wight · independente');
    expect(mountLine(e, rider, true, false)).toBe('Cavalgando a Aranha');
    expect(mountLine(e, spider, true, false)).toBe('Leva Lia · independente');
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
  it('opens a line with "Cavalgando o Cavalo de guerra" and goes on inside a sentence with "cavalgando"', () => {
    expect(mountedOn(horse())).toBe('Cavalgando o Cavalo de guerra');
    expect(ridingOn(horse())).toBe('cavalgando o Cavalo de guerra');
    expect(ridingOn(spider)).toBe('cavalgando a Aranha');
  });

  it('puts no article before a character and no gendered participle in the line', () => {
    const e = encounter({ combatants: [rider, spider] });
    expect(mountLine(e, spider, true, false)).toBe('Leva Lia · independente');
    expect(mountLine(e, spider, true, false)).not.toMatch(/Montad[oa]/);
  });
});
