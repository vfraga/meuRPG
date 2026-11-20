import { AreaPlacement, AttackOutcome } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { DisabledReasonCode } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { areaSpellOf } from '../cast-sheet/cast-sheet-data';
import {
  areaSpellsOf,
  attackShow,
  coverNoteOf,
  distanceText,
  pairHintOf,
  resultOf,
  rollLabelOf,
  rollWhyOf,
  rollableAttacks,
  statsOf,
} from './npc-card-view';

describe('npc card view', () => {
  const attack = { key: 'a', namePt: 'Espada', attackBonus: 4, saveDc: 0 };
  const options = (extra: object = {}) =>
    ({
      options: {
        attacks: [{ attack }, { attack: { ...attack, key: 'b', saveDc: 12 } }, {}],
        spells: [],
      },
      spellTargets: [],
      ...extra,
    }) as never;
  const nbsp = (t: string) => t.replace(/ /g, ' ');

  it('lists the attacks that roll to hit only', () => {
    expect(rollableAttacks(null)).toEqual([]);
    expect(rollableAttacks(options()).map((a) => a.key)).toEqual(['a']);
  });

  it('lists the area spells placed on a map, and none in theatre or without options', () => {
    const o = options({
      options: {
        attacks: [],
        spells: [
          { enabled: true, spell: { key: 'x', namePt: 'Bola', name: 'Ball' } },
          { enabled: true, spell: { key: 'y', namePt: '', name: 'Wall' } },
          { enabled: false, spell: { key: 'z', namePt: 'Off', name: 'Off' } },
          { enabled: true },
        ],
      },
      spellTargets: [
        { spellKey: 'x', placement: AreaPlacement.POINT },
        { spellKey: 'y', placement: AreaPlacement.POINT },
        { spellKey: 'z', placement: AreaPlacement.POINT },
      ],
    });
    expect(areaSpellsOf(o, false)).toEqual([
      { key: 'x', name: 'Bola' },
      { key: 'y', name: 'Wall' },
    ]);
    expect(areaSpellsOf(o, true)).toEqual([]);
    expect(areaSpellsOf(null, false)).toEqual([]);
  });

  it('lists the area spell an NPC has only as an innate grant by its reference, apart from the class option of the same spell', () => {
    const grant = { sourceKey: 'trait:inato', sourceNamePt: 'Dom inato' };
    const o = options({
      options: {
        attacks: [],
        spells: [
          { enabled: true, spell: { key: 'x', namePt: 'Bola', name: 'Ball' } },
          { enabled: true, spell: { key: 'x', namePt: 'Bola', name: 'Ball' }, grant },
          { enabled: true, spell: { key: 'w', namePt: 'Muro', name: 'Wall' }, grant },
        ],
      },
      spellTargets: [
        { spellKey: 'x', placement: AreaPlacement.POINT },
        { spellKey: 'x', placement: AreaPlacement.POINT },
        { spellKey: 'w', placement: AreaPlacement.POINT },
      ],
    });
    expect(areaSpellsOf(o, false)).toEqual([
      { key: 'x', name: 'Bola' },
      { key: 'x#granted=trait:inato', name: 'Bola · Dom inato' },
      { key: 'w#granted=trait:inato', name: 'Muro · Dom inato' },
    ]);
    for (const { key } of areaSpellsOf(o, false)) {
      expect(areaSpellOf(o as never, key)).not.toBeNull();
    }
  });

  it('holds the roll for damage owed and for a combatant that cannot act', () => {
    expect(rollWhyOf(options(), 1)).toContain('Role ou aplique o dano');
    expect(rollWhyOf(options(), 0)).toBe('');
    const off = options({
      options: {
        attacks: [{ enabled: false, reason: { code: DisabledReasonCode.INCAPACITATED } }],
      },
    });
    expect(rollWhyOf(off, 0)).toBe('Você não pode agir.');
  });

  it('builds the stats of a map and of a theatre', () => {
    const c = combatant({
      speedDft: 90,
      speedFt: 30,
      movementLeftDft: 0,
      movementLeftFt: 0,
      armorClass: 12,
      hitPointsMax: 9,
    } as never);
    expect(statsOf(c, false).hp).toEqual({ now: 0, max: 9 });
    expect(nbsp(statsOf(c, false).speed.squares)).toContain('quadrados');
    expect(statsOf(c, true).speed.squares).toBe('');
    expect(statsOf(c, true).movement).toBe('Sem movimento');
    expect(statsOf(combatant({} as never), false).hp).toBeNull();
  });

  it('builds the result box from the last roll', () => {
    const e = encounter({ combatants: [combatant({ id: 't', label: 'Brisa' })] });
    const last = (roll: object) =>
      ({
        roll: { targetId: 't', outcome: AttackOutcome.HIT, heldForReaction: false, ...roll },
        pending: null,
        subject: 'n',
      }) as never;
    expect(resultOf(null, e, false, 'Escudo')).toBeNull();
    // The head of the block names the attack and the target ("Cimitarra em Brisa"); without the attack in the options it is empty.
    const scimitar = { key: 'attack:scimitar', name: 'Scimitar', namePt: 'Cimitarra' } as never;
    expect(
      resultOf(last({ attackKey: 'attack:scimitar' }), e, false, 'Escudo', [scimitar])!.head,
    ).toBe('Cimitarra em Brisa');
    expect(resultOf(last({ attackKey: 'attack:scimitar' }), e, false, 'Escudo')!.head).toBe('');
    const hit = resultOf(last({ targetArmorClass: 13 }), e, false, 'Escudo')!;
    expect(hit).toMatchObject({
      word: 'Acertou',
      hit: true,
      against: 'contra CA 13 da Brisa',
      total: 0,
      formula: '',
    });
    expect(resultOf(last({}), e, true, 'Escudo Arcano')).toMatchObject({
      word: 'Errou: o Escudo Arcano segurou',
      hit: false,
    });
    expect(resultOf(last({ heldForReaction: true }), e, false, 'Escudo')).toMatchObject({
      word: 'Esperando a reação',
      held: true,
      hit: false,
    });
  });

  it('words the hint, the label, the cover and the distance', () => {
    expect(pairHintOf('lower')).toContain('menor');
    expect(pairHintOf('higher')).toContain('maior');
    expect(rollLabelOf(null)).toBe('');
    expect(rollLabelOf(attack as never)).toBe('Role 1d20 para Espada (+4)');
    expect(rollLabelOf({ ...attack, attackBonus: -2 } as never)).toBe('Role 1d20 para Espada (−2)');
    expect(coverNoteOf({ cover: 0, coverSource: 0 } as never)).toBe('');
    expect(distanceText(undefined, false)).toBe('');
    expect(distanceText(undefined, true)).toBe('fora do alcance');
  });

  it('plays the d20 with the outcome unless a reaction holds it', () => {
    const roll = (over: object) =>
      ({
        outcome: AttackOutcome.CRITICAL_HIT,
        heldForReaction: false,
        d20: { diceCount: 1, diceSides: 20, faces: [20], modifier: 0, total: 20, countedIndex: 0 },
        ...over,
      }) as never;
    expect(attackShow(attack as never, roll({}))).toMatchObject({
      label: 'Ataque com Espada',
      critical: true,
      outcome: { word: 'Crítico', good: true },
    });
    expect(attackShow(attack as never, roll({ heldForReaction: true }))?.outcome).toBeUndefined();
    expect(attackShow(attack as never, roll({ d20: undefined }))).toBeNull();
  });
});
