import { type MessageInitShape, create } from '@bufbuild/protobuf';

import {
  AttackOutcome,
  CombatLogEffectChange,
  CombatLogEffectSchema,
  type CombatLogEntry,
  CombatLogEntrySchema,
  CombatLogKind,
  DiceRollSchema,
  PendingDamageStatus,
  SpellEffectKind,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { logLine } from './combat-log';
import { treatedFormula, treatedPreview, treatedSourceName } from './combat-dice';

// R8: the dice a roll's reader gets. The server sends the faces, the counted dice and the names only to the roller's player
// and the master, so these lines are written from what is in the entry and from nothing else.

const text = (e: CombatLogEntry) =>
  logLine(e, '', { master: false, players: new Set() }) ?? undefined;

const counted = (face: number, countedAs: number, name = 'Chama Insistente', index = 0) => ({
  index,
  face,
  countedAs,
  sourceKey: 'feature:insistent-flame',
  sourceName: name,
});

const damage = (over: MessageInitShape<typeof CombatLogEntrySchema>['damage'] = {}) => ({
  status: PendingDamageStatus.APPLIED,
  amount: 9,
  damageTypePt: 'fogo',
  ...over,
});

const spell = (dmg: ReturnType<typeof damage>) =>
  create(CombatLogEntrySchema, {
    id: 's1',
    kind: CombatLogKind.SPELL_CAST,
    actorLabel: 'Pensantus',
    keyNamePt: 'Raio de Fogo',
    spell: {
      effectKind: SpellEffectKind.UNSPECIFIED,
      targets: [{ targetLabel: 'Zumbi', outcome: AttackOutcome.HIT, damage: dmg }],
    },
  });

describe('the dice of a roll in the log (R8)', () => {
  it('writes the dice a floor raised and the feature that did it, under a spell’s line', () => {
    const line = text(
      spell(
        damage({
          roll: {
            diceCount: 2,
            diceSides: 10,
            faces: [1, 7],
            total: 9,
            countedDice: [counted(1, 2)],
          },
        }),
      ),
    );
    expect(line?.notes).toEqual(['2d10 (1 → 2, 7) = 9', 'Chama Insistente: o 1 contou como 2.']);
  });

  it('marks the die in the parts of a damage with parts, and names "uma característica" for a source with no name', () => {
    const line = text(
      spell(
        damage({
          parts: [
            {
              labelPt: 'Raio de Fogo',
              diceCount: 2,
              diceSides: 10,
              faces: [7, 1],
              sum: 9,
              counted: true,
              countedDice: [counted(1, 2, '', 1)],
            },
          ],
        }),
      ),
    );
    expect(line?.notes).toEqual([
      'Raio de Fogo 2d10 (7, 1 → 2) = 9',
      'uma característica: o 1 contou como 2.',
    ]);
  });

  it('joins two dice of one feature in one sentence', () => {
    const line = text(
      spell(
        damage({
          roll: {
            diceCount: 3,
            diceSides: 6,
            faces: [1, 1, 5],
            modifier: 2,
            total: 11,
            countedDice: [counted(1, 2), counted(1, 2, 'Chama Insistente', 1)],
          },
        }),
      ),
    );
    expect(line?.notes).toEqual([
      '3d6 (1 → 2, 1 → 2, 5) + 2 = 11',
      'Chama Insistente: o 1 contou como 2 e o 1 contou como 2.',
    ]);
  });

  it('writes nothing for a reader that got no counted dice and no faces', () => {
    const line = text(spell(damage({ roll: { diceCount: 2, diceSides: 10, total: 9 } })));
    expect(line?.notes ?? []).toEqual([]);
  });

  it('writes a reroll rule’s d20 with the roll it made, by the name the server sent', () => {
    const line = text(
      create(CombatLogEntrySchema, {
        id: 'a1',
        kind: CombatLogKind.ATTACK,
        outcome: AttackOutcome.HIT,
        actorLabel: 'Tor',
        targetLabel: 'Zumbi',
        attackRoll: {
          diceCount: 1,
          diceSides: 20,
          faces: [14],
          modifier: 5,
          total: 19,
          rerolled: [
            {
              index: 0,
              from: 2,
              to: 14,
              sourceKey: 'feature:stubborn-luck',
              sourceName: 'Sorte Teimosa',
            },
          ],
        },
      }),
    );
    expect(line?.notes).toEqual(['Sorte Teimosa: 2 → 14 · 1d20 (14) + 5 = 19']);
  });

  it('reads "uma característica" for a reroll with no source name, and keeps Lucky’s short line', () => {
    const rolled = (sourceKey: string, sourceName: string) =>
      text(
        create(CombatLogEntrySchema, {
          id: 'a2',
          kind: CombatLogKind.ATTACK,
          outcome: AttackOutcome.HIT,
          attackRoll: {
            diceCount: 1,
            diceSides: 20,
            faces: [14],
            modifier: 5,
            total: 19,
            rerolled: [{ index: 0, from: 1, to: 14, sourceKey, sourceName }],
          },
        }),
      )?.notes;
    expect(rolled('feature:stubborn-luck', '')).toEqual([
      'uma característica: 1 → 14 · 1d20 (14) + 5 = 19',
    ]);
    expect(rolled('trait:lucky', '')).toEqual(['Sortudo: 1 → 14']);
  });

  it('writes the d20 a feature raised on a lasting effect’s save, by the name the server sent', () => {
    const line = text(
      create(CombatLogEntrySchema, {
        id: 'e1',
        kind: CombatLogKind.EFFECT,
        effect: create(CombatLogEffectSchema, {
          sourceKey: 'spell:hold-person',
          sourceNamePt: 'Imobilizar Pessoa',
          targetLabels: ['bilbo'],
          change: CombatLogEffectChange.SAVED,
          d20: 3,
          total: 10,
          treatedAs: 8,
          treatedAsSource: 'feature:iron-will',
          treatedAsSourceName: 'Vontade de Ferro',
        }),
      }),
    );
    expect(line?.notes).toEqual(['d20: 3 → 8 (Vontade de Ferro) + 2 = 10']);
  });

  it('writes the d20 a feature raised on a spell target’s save', () => {
    const e = create(CombatLogEntrySchema, {
      id: 's2',
      kind: CombatLogKind.SPELL_CAST,
      actorLabel: 'Pensantus',
      keyNamePt: 'Bola de Fogo',
      spell: {
        targets: [
          {
            targetLabel: 'bilbo',
            save: {
              roll: {
                diceCount: 1,
                diceSides: 20,
                faces: [3],
                modifier: 2,
                total: 10,
                treatedAs: 8,
                treatedAsSource: 'feature:iron-will',
                treatedAsSourceName: 'Vontade de Ferro',
              },
            },
          },
        ],
      },
    });
    expect(text(e)?.notes).toEqual(['d20: 3 → 8 (Vontade de Ferro) + 2 = 10']);
  });
});

describe('the name of the feature that raised a d20 (treated_as_source_name)', () => {
  const roll = (name: string, source = 'feature:iron-will') =>
    create(DiceRollSchema, {
      diceCount: 1,
      diceSides: 20,
      faces: [3],
      modifier: 2,
      total: 10,
      treatedAs: 8,
      treatedAsSource: source,
      treatedAsSourceName: name,
    });

  it('reads the name the server sent', () => {
    expect(treatedFormula(roll('Vontade de Ferro'))).toBe('d20: 3 → 8 (Vontade de Ferro) + 2 = 10');
  });

  it('reads "uma característica" when the name is empty and the key is not one the app knows', () => {
    expect(treatedFormula(roll(''))).toBe('d20: 3 → 8 (uma característica) + 2 = 10');
  });

  it('keeps Talento Confiável working by its key, and lets the server’s name win', () => {
    expect(treatedSourceName('feature:reliable-talent')).toBe('Talento Confiável');
    expect(treatedFormula(roll('', 'feature:reliable-talent'))).toBe(
      'd20: 3 → 8 (Talento Confiável) + 2 = 10',
    );
    expect(treatedSourceName('feature:reliable-talent', 'Talento Confiável da Mesa')).toBe(
      'Talento Confiável da Mesa',
    );
    expect(
      treatedPreview(6, 9, { value: 10, source: 'feature:reliable-talent', name: '' })?.text,
    ).toBe('6 → 10 (Talento Confiável) + 9 = 19');
  });
});
