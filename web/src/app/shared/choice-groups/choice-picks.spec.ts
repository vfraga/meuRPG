import { create } from '@bufbuild/protobuf';

import {
  ChoiceGroupSchema,
  ChoiceKind,
  ChoiceOrigin,
  ChoiceSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { isTraitChoice, missingSentence, originLine, pendingLabels } from './choice-picks';

describe('the open choices of a sheet', () => {
  const choice = (labelPt: string, missing: number, advisory = false) =>
    create(ChoiceSchema, { key: labelPt, labelPt, picks: 1, missing, advisory });
  const groups = [
    create(ChoiceGroupSchema, {
      choices: [
        choice('Estilo de Luta (Guerreiro, nível 1)', 1),
        choice('Idiomas (Acólito)', 2, true),
        choice('Ancestral (Draconato)', 0),
      ],
    }),
  ];

  it('lists every open choice, the advisory ones too, for the mark on the step', () => {
    expect(pendingLabels(groups)).toEqual([
      'Estilo de Luta (Guerreiro, nível 1)',
      'Idiomas (Acólito)',
    ]);
  });

  it('says only the choices that stop the sheet under "Criar personagem"', () => {
    expect(pendingLabels(groups, false)).toEqual(['Estilo de Luta (Guerreiro, nível 1)']);
    expect(missingSentence(groups)).toBe('Falta uma escolha: Estilo de Luta (Guerreiro, nível 1).');
  });

  it('says nothing when only a language or a tool is open', () => {
    expect(
      missingSentence([
        create(ChoiceGroupSchema, { choices: [choice('Idioma (Humano)', 1, true)] }),
      ]),
    ).toBe('');
  });
});

describe("the origin line of a race's trait choice", () => {
  const pick = (key: string, featureKey: string, picks = 1) =>
    create(ChoiceSchema, { key, featureKey, kind: ChoiceKind.OPTIONS, picks });
  const race = create(ChoiceGroupSchema, {
    origin: ChoiceOrigin.RACE,
    sourceNamePt: 'Draconato',
  });

  it('says "Traço de" for the dragonborn ancestry, an SRD trait with options', () => {
    const c = pick('trait:draconic-ancestry', 'trait:draconic-ancestry');
    expect(isTraitChoice(race, c)).toBe(true);
    expect(originLine(race, c)).toBe('Traço de Draconato · escolha uma vez');
  });

  it('keeps "Raça ·" for a feat or an ability choice of the race', () => {
    const feat = pick('trait:versatile#feat', 'trait:versatile');
    expect(isTraitChoice(race, feat)).toBe(false);
    expect(originLine(race, feat)).toBe('Raça · Draconato · 1 escolha');
    expect(isTraitChoice(race, pick('race:half-elf#abilities', 'race:half-elf'))).toBe(false);
  });
});
