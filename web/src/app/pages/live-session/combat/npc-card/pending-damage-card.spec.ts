import { PendingDamageStatus } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter, spellZone } from '../../../../core/combat/combat-testing';
import { effectLine, pendingCard, trapLabel } from './pending-damage-card';

describe('pending damage card', () => {
  const pending = (over: object = {}) =>
    ({
      id: 'p',
      attackerId: 'a',
      targetId: 't',
      status: PendingDamageStatus.ROLLED,
      diceCount: 2,
      diceSides: 6,
      bonus: 1,
      criticalMax: 0,
      amount: 5,
      damageTypeKey: 'fire',
      damageTypePt: 'fogo',
      ...over,
    }) as never;

  it('numbers the cards of a trap by type, and leaves a lone card unnumbered', () => {
    const a = pending({ id: '1', trapPointId: 'pt' });
    const b = pending({ id: '2', trapPointId: 'pt' });
    expect(trapLabel(a, [a, b])).toBe('Armadilha: fogo (1 de 2)');
    expect(trapLabel(b, [a, b])).toBe('Armadilha: fogo (2 de 2)');
    expect(trapLabel(a, [a])).toBe('Armadilha: fogo');
    expect(trapLabel(pending(), [])).toBe('');
  });

  it('previews the hit points, the beast first, and says nothing without a maximum', () => {
    const plain = combatant({ label: 'Ana', hitPointsCurrent: 10, hitPointsMax: 12 } as never);
    expect(effectLine(plain, 4)).toContain('depois 6');
    expect(effectLine(combatant({} as never), 4)).toBe('');
    const druid = combatant({
      label: 'Nina',
      hitPointsCurrent: 20,
      hitPointsMax: 20,
      wildShapeBeastKey: 'wolf',
      wildShapeBeastNamePt: 'Lobo',
      wildShapeHitPointsCurrent: 3,
      wildShapeHitPointsMax: 11,
    } as never);
    expect(effectLine(druid, 5)).toContain('o resto (2) passa para Nina');
  });

  it('builds the card of a rolled damage and of one that waits for the player', () => {
    const e = encounter({
      combatants: [
        combatant({ id: 'a', label: 'Goblin' }),
        combatant({ id: 't', label: 'Ana', hitPointsCurrent: 9, hitPointsMax: 9 } as never),
      ],
    });
    const card = pendingCard(pending(), e, []);
    expect(card).toMatchObject({ total: 5, rolled: true, attacker: 'Goblin', target: 'Ana' });
    expect(card.effect).toContain('depois 4');
    const waiting = pendingCard(pending({ status: PendingDamageStatus.AWAITING_ROLL }), e, []);
    expect(waiting).toMatchObject({
      rolled: false,
      effect: '',
      label: 'Role 2d6 para o dano: some os dois',
    });
  });

  it('offers Tirar on every choosable extra, alone in its type or in a typed group, never on the weapon', () => {
    const e = encounter({
      combatants: [combatant({ id: 'a', label: 'A' }), combatant({ id: 't', label: 'T' })],
    });
    const part = (key: string) => ({ key, labelPt: key, choosable: key !== 'weapon' });
    const roll = (partKey: string, type: string, sum: number) => ({
      partKey,
      labelPt: partKey,
      damageTypeKey: type,
      diceCount: 1,
      faces: [],
      sum,
      flat: 0,
      counted: true,
    });
    const removable = (rolls: object[], keys: string[]) =>
      [
        ...pendingCard(pending({ parts: keys.map(part), partRolls: rolls }), e, []).removable,
      ].sort();
    // Alone in its type, the sum on the part: it comes out.
    expect(
      removable([roll('sneak', 'piercing', 7), roll('edge', 'fire', 3)], ['sneak', 'edge']),
    ).toEqual(['edge', 'sneak']);
    // Two parts share the piercing sum: the weapon stays, the extra comes out (taking it out asks the sum again).
    expect(
      removable(
        [roll('weapon', 'piercing', 7), roll('sneak', 'piercing', 0), roll('edge', 'fire', 3)],
        ['sneak', 'weapon', 'edge'],
      ),
    ).toEqual(['edge', 'sneak']);
    // The holder that is an extra comes out too, and so does the follower.
    expect(
      removable([roll('sneak', 'piercing', 7), roll('edge', 'piercing', 0)], ['sneak', 'edge']),
    ).toEqual(['edge', 'sneak']);
  });

  it('labels a damage the zone of a spell owes, in generic words (it carries no reason)', () => {
    const e = encounter({
      combatants: [combatant({ id: 'a', label: 'Bilbo' }), combatant({ id: 't', label: 'Tor' })],
      zones: [
        spellZone({
          id: 'z',
          spellKey: 'spell:cloud-of-daggers',
          spellNamePt: 'Nuvem de Adagas',
          ownerCombatantId: 'a',
        }),
      ],
    } as never);
    const zoned = pending({
      attackKey: 'spell:cloud-of-daggers',
      status: PendingDamageStatus.AWAITING_ROLL,
    });
    expect(pendingCard(zoned, e, [zoned]).zoneLabel).toBe('Zona de magia: Nuvem de Adagas');
    const plainHit = pending({ attackKey: 'spell:cloud-of-daggers', castId: 'c1' });
    expect(pendingCard(plainHit, e, [plainHit]).zoneLabel).toBe('');
    const other = pending({ attackKey: 'dagger' });
    expect(pendingCard(other, e, [other]).zoneLabel).toBe('');
  });
});
