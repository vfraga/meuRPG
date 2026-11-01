import { type MessageInitShape, create } from '@bufbuild/protobuf';

import {
  AttackOutcome,
  CombatLogEffectChange,
  CombatLogEntrySchema,
  CombatLogKind,
  CombatantState,
  ConcentrationSaveResultSchema,
  DeathSaveOutcome,
  DiceRollSchema,
  LuckyOfferSchema,
  SpellCastSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { EffectSaveResultSchema } from '../../../gen/meurpg/play/v1/lasting_effects_pb';
import { castRows } from './cast-flow';
import { logLine } from './combat-log';
import { INDOMITABLE_FEATURE, LUCKY_TRAIT, STROKE_FEATURE, luckyText, luckyWhat } from './lucky';
import { concentrationResultView } from './reactions';

const d20 = (faces: number[], rerolled: MessageInitShape<typeof DiceRollSchema>['rerolled'] = []) =>
  create(DiceRollSchema, { diceCount: faces.length, diceSides: 20, faces, modifier: 5, rerolled });

describe('the words of the halfling’s Lucky (Sortudo)', () => {
  it('says the d20 is rolled again, and with two dice names the one that showed the 1', () => {
    expect(luckyWhat(create(LuckyOfferSchema, { d20: d20([1]) }))).toBe(
      'Sortudo: você pode rolar esse d20 de novo e fica com o novo resultado.',
    );
    expect(luckyWhat(create(LuckyOfferSchema, { d20: d20([1, 13]) }))).toBe(
      'Sortudo: você pode rolar de novo o dado que deu 1 e fica com o novo resultado.',
    );
  });

  it('writes "Sortudo: 1 → 14" for the d20 Lucky rolled again, and nothing for Great Weapon Fighting or a kept 1', () => {
    expect(luckyText(d20([14], [{ index: 0, from: 1, to: 14, sourceKey: LUCKY_TRAIT }]))).toBe(
      'Sortudo: 1 → 14',
    );
    expect(luckyText(d20([4], [{ index: 0, from: 1, to: 4, sourceKey: '' }]))).toBe('');
    expect(luckyText(d20([1]))).toBe('');
    expect(luckyText(undefined)).toBe('');
  });

  it('puts the reroll among the notes of the attack line, where a reload still finds it', () => {
    const line = logLine(
      create(CombatLogEntrySchema, {
        id: 'x',
        kind: CombatLogKind.ATTACK,
        actorLabel: 'Pipo',
        targetLabel: 'Goblin',
        keyNamePt: 'Machado de batalha',
        key: 'equipment:battleaxe',
        outcome: AttackOutcome.HIT,
        attackRoll: d20([15], [{ index: 0, from: 1, to: 15, sourceKey: LUCKY_TRAIT }]),
      }),
    );
    expect(line?.notes).toContain('Sortudo: 1 → 15');
  });

  // The other d20 rolls of the combat (the held outside rolls): a saving throw against an effect, a spell attack, a
  // concentration save.
  const again = (to: number) => [{ index: 0, from: 1, to, sourceKey: LUCKY_TRAIT }];

  it('reads the reroll of a check or a save too: an effect’s result and a group check’s roll carry it the same way', () => {
    expect(luckyText(create(EffectSaveResultSchema, { d20: 12, rerolled: again(12) }))).toBe(
      'Sortudo: 1 → 12',
    );
    expect(luckyText(create(EffectSaveResultSchema, { d20: 12 }))).toBe('');
  });

  it('puts the reroll of an effect’s saving throw under its line, and nothing when no die was rolled again', () => {
    const entry = (rerolled: ReturnType<typeof again>) =>
      create(CombatLogEntrySchema, {
        id: 'x',
        kind: CombatLogKind.EFFECT,
        effect: {
          sourceKey: 'spell:hold-person',
          sourceNamePt: 'Imobilizar Pessoa',
          change: CombatLogEffectChange.SAVED,
          targetLabels: ['Pipo'],
          ability: 'wis',
          abilityNamePt: 'Sabedoria',
          d20: 14,
          total: 15,
          rerolled,
        },
      });
    const line = logLine(entry(again(14)));
    expect(line?.text).toContain('passou no teste de Sabedoria contra Imobilizar Pessoa');
    expect(line?.notes).toEqual(['Sortudo: 1 → 14']);
    expect(logLine(entry([]))?.notes).toBeUndefined();
  });

  it('words Stroke of Luck: a check counted as 20, or an attack that missed and hit', () => {
    expect(luckyText(d20([20], [{ index: 0, from: 6, to: 20, sourceKey: STROKE_FEATURE }]))).toBe(
      'Golpe de Sorte: 6 → 20',
    );
    expect(luckyText(d20([6], [{ index: 0, from: 6, to: 6, sourceKey: STROKE_FEATURE }]))).toBe(
      'Golpe de Sorte: o erro virou acerto',
    );
  });

  it('words a death save that Indomitable rolled again after the counts', () => {
    const entry = (rerolled: { index: number; from: number; to: number; sourceKey: string }[]) =>
      create(CombatLogEntrySchema, {
        id: 'x',
        kind: CombatLogKind.DEATH_SAVE,
        actorLabel: 'Toren',
        deathSave: {
          roll: d20([14], rerolled),
          outcome: DeathSaveOutcome.SUCCESS,
          successes: 1,
          failures: 0,
        },
      });
    const text = logLine(
      entry([{ index: 0, from: 5, to: 14, sourceKey: INDOMITABLE_FEATURE }]),
    )?.text;
    expect(text).toContain('sucesso');
    expect(text).toContain('Indomitável: 5 → 14');
    expect(logLine(entry([]))?.text).not.toContain('Indomitável');
  });

  it('puts the reroll of a spell attack’s d20 under the cast line', () => {
    const line = logLine(
      create(CombatLogEntrySchema, {
        id: 'x',
        kind: CombatLogKind.SPELL_CAST,
        actorLabel: 'Pipo',
        keyNamePt: 'Raio Guiador',
        key: 'spell:guiding-bolt',
        spell: {
          targets: [
            {
              targetLabel: 'Goblin',
              outcome: AttackOutcome.HIT,
              attackRoll: d20([16], again(16)),
            },
          ],
        },
      }),
    );
    expect(line?.notes).toEqual(['Sortudo: 1 → 16']);
  });

  it('writes the reroll among the lines of the cast result, after the d20', () => {
    const cast = create(SpellCastSchema, {
      spellKey: 'spell:guiding-bolt',
      targets: [{ combatantId: 'g', outcome: AttackOutcome.HIT, attackRoll: d20([16], again(16)) }],
    });
    const [row] = castRows(
      cast,
      new Map(),
      new Map([['g', { label: 'Goblin', state: CombatantState.UNHURT }]]),
    );
    expect(row.lines).toContain('Sortudo: 1 → 16');
    expect(row.lines.indexOf('Sortudo: 1 → 16')).toBe(1);
  });

  it('says the reroll on the concentration result, under the roll against the DC', () => {
    const view = concentrationResultView(
      create(ConcentrationSaveResultSchema, {
        save: d20([13], again(13)),
        dc: 10,
        kept: true,
        spellNamePt: 'Bênção',
      }),
      2,
    );
    expect(view.text).toHaveLength(2);
    expect(view.text[1]).toBe('Sortudo: 1 → 13');
    const without = concentrationResultView(
      create(ConcentrationSaveResultSchema, {
        save: d20([1]),
        dc: 10,
        kept: false,
        spellNamePt: 'Bênção',
      }),
      2,
    );
    expect(without.text).toHaveLength(1);
  });
});
