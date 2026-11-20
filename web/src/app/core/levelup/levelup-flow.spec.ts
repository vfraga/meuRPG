import { create } from '@bufbuild/protobuf';

import {
  ChoiceGroupSchema,
  ChoiceSchema,
  LevelUpFeatureChoiceSchema,
  LevelUpProficiencyKind,
  LevelUpSubclassSchema,
  LevelUpSwapGroupSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  cantripOptions,
  expertiseOptions,
  needText,
  preparedMore,
  preparedOptions,
  skillOptions,
  spellOptions,
  stepsFor,
  totalsFor,
} from './levelup-flow';
import { SKILLS, SPELLS, WIZARD_KEYS, fighterOptions, wizardOptions } from './levelup-testing';

const plain = (s: string) => s.replace(/\u00a0/g, ' ');

const steps = (
  o: ReturnType<typeof wizardOptions>,
  sub = '',
  maxAfter = o.preparedMaxAfter,
  prepared = 7,
) => stepsFor(o, totalsFor(o, sub), preparedMore(o, maxAfter, prepared));

describe('the steps of a level-up (MR-040)', () => {
  it('gives Pensantus Habilidades, Vida, Magias and Resumo at Mago 4', () => {
    expect(steps(wizardOptions())).toEqual(['class', 'abilities', 'hp', 'spells', 'summary']);
  });

  it('drops Habilidades where the level has no increase, and Magias where nothing is chosen', () => {
    const fighter4 = fighterOptions({ abilityScoreImprovement: true });
    expect(steps(fighter4)).toEqual(['class', 'abilities', 'hp', 'summary']);
    // Toren at Guerreiro 5: Vida and Resumo only, the hit points never go.
    expect(steps(fighterOptions())).toEqual(['class', 'hp', 'summary']);
  });

  it('keeps Magias for a level that only lets the caster prepare more', () => {
    const cleric = wizardOptions({
      abilityScoreImprovement: false,
      cantrips: 0,
      spells: 0,
      spellsKind: 0,
      classKey: 'class:cleric',
    });
    expect(steps(cleric, '', 6, 4)).toEqual(['class', 'hp', 'spells', 'summary']);
    expect(steps(cleric, '', 4, 4)).toEqual(['class', 'hp', 'summary']);
  });

  it('adds Escolhas for the swap of a table feature at a level without Habilidades, and not otherwise', () => {
    const group = create(LevelUpSwapGroupSchema, {
      group: 'feature:infusoes@mesa',
      namePt: 'Infusões',
      picks: [{ key: 'a', namePt: 'A' }],
      pool: [{ key: 'b', namePt: 'B' }],
    });
    expect(steps(fighterOptions({ swapGroups: [group] }))).toEqual([
      'class',
      'hp',
      'picks',
      'summary',
    ]);
    // With the ability step, the panel is that step's own.
    expect(steps(fighterOptions({ abilityScoreImprovement: true, swapGroups: [group] }))).toEqual([
      'class',
      'abilities',
      'hp',
      'summary',
    ]);
  });

  it('adds Escolhas for a subclass, a feature option, a skill or expertise', () => {
    const due = fighterOptions({
      subclassDue: true,
      subclasses: [create(LevelUpSubclassSchema, { key: 'sub:champion', namePt: 'Campeão' })],
    });
    expect(steps(due)).toEqual(['class', 'hp', 'picks', 'summary']);
    const style = fighterOptions({
      featureChoices: [
        create(LevelUpFeatureChoiceSchema, {
          choose: 1,
          options: [{ key: 'style:defense', namePt: 'Defesa' }],
        }),
      ],
    });
    expect(steps(style)).toEqual(['class', 'hp', 'picks', 'summary']);
    expect(steps(fighterOptions({ skillChoices: 2 }))).toEqual(['class', 'hp', 'picks', 'summary']);
    expect(steps(fighterOptions({ expertiseChoices: 2 }))).toEqual([
      'class',
      'hp',
      'picks',
      'summary',
    ]);
  });

  it('puts the chosen subclass share into the counts, so Magias can appear after it', () => {
    const o = fighterOptions({
      subclassDue: true,
      subclasses: [
        create(LevelUpSubclassSchema, {
          key: 'sub:land',
          namePt: 'Círculo da Terra',
          cantrips: 1,
          skillChoices: 1,
        }),
      ],
    });
    expect(totalsFor(o, '')).toMatchObject({ cantrips: 0, skills: 0 });
    expect(totalsFor(o, 'sub:land')).toMatchObject({ cantrips: 1, skills: 1 });
    expect(steps(o)).not.toContain('spells');
    expect(steps(o, 'sub:land')).toContain('spells');
  });
});

describe('how many more to prepare', () => {
  it('is the new maximum less what is prepared, never below 0, and 0 for a class that does not prepare', () => {
    const o = wizardOptions();
    expect(preparedMore(o, 9, 7)).toBe(2);
    expect(preparedMore(o, 5, 7)).toBe(0);
    expect(preparedMore(wizardOptions({ prepares: false }), 9, 7)).toBe(0);
  });
});

describe('the pickers lists', () => {
  const o = wizardOptions();

  it('lists the class cantrips not known yet, in Portuguese order', () => {
    const items = cantripOptions(o, SPELLS, WIZARD_KEYS);
    expect(items.map((i) => i.name)).toEqual(['Prestidigitação', 'Toque Chocante']);
    expect(plain(items[0].sub)).toBe('Truque · Transmutação');
  });

  it('lists book spells up to the highest circle, minus the known ones and other classes', () => {
    const names = spellOptions(o, SPELLS, WIZARD_KEYS).map((i) => i.name);
    expect(names).toEqual(['Onda Trovejante', 'Invisibilidade', 'Passo Nebuloso', 'Reflexos']);
    expect(names).not.toContain('Bola de Fogo');
    expect(names).not.toContain('Curar Ferimentos');
  });

  it("lets a Bard's Magical Secrets take any class's list", () => {
    const names = spellOptions(wizardOptions({ anyClassSpells: 2 }), SPELLS, WIZARD_KEYS).map(
      (i) => i.name,
    );
    expect(names).toContain('Curar Ferimentos');
  });

  it('offers for preparing the book with the spells just copied, marked as new', () => {
    const items = preparedOptions(
      o,
      SPELLS,
      { ...WIZARD_KEYS, prepared: ['spell:magic-missile'] },
      new Set(['spell:misty-step']),
    );
    expect(items.map((i) => [i.name, plain(i.sub)])).toEqual([
      ['Detectar Magia', '1º nível · Adivinhação · ritual'],
      ['Passo Nebuloso', '2º nível · Conjuração · nova no livro'],
    ]);
  });

  it('offers new skills, and expertise only in trained ones (today or just picked)', () => {
    expect(skillOptions(SKILLS, WIZARD_KEYS).map((s) => s.name)).toEqual([
      'Furtividade',
      'Percepção',
    ]);
    expect(expertiseOptions(SKILLS, WIZARD_KEYS, new Set()).map((s) => s.name)).toEqual([
      'Arcanismo',
      'História',
    ]);
    expect(
      expertiseOptions(SKILLS, WIZARD_KEYS, new Set(['skill:stealth'])).map((s) => s.name),
    ).toContain('Furtividade');
    expect(
      expertiseOptions(SKILLS, { ...WIZARD_KEYS, expertise: ['skill:arcana'] }, new Set()).map(
        (s) => s.name,
      ),
    ).toEqual(['História']);
  });
});

describe('needText', () => {
  it('says what is missing in the singular and the plural', () => {
    expect(needText(1, 'magia', 'magias')).toBe('Falta escolher 1 magia.');
    expect(needText(2, 'magia', 'magias')).toBe('Faltam escolher 2 magias.');
    expect(needText(2, 'magia', 'magias', 'preparar')).toBe('Faltam preparar 2 magias.');
  });
});

describe('the steps of a level-up: choices an earlier level left open (PM-05)', () => {
  const group = (key: string) =>
    create(ChoiceGroupSchema, { choices: [create(ChoiceSchema, { key, picks: 1, missing: 1 })] });

  it('has the Escolhas step for a late choice or a new scoped one, and none without them', () => {
    expect(steps(fighterOptions())).toEqual(['class', 'hp', 'summary']);
    expect(steps(fighterOptions({ lateChoices: [group('c1')] }))).toEqual([
      'class',
      'hp',
      'picks',
      'summary',
    ]);
    expect(steps(fighterOptions({ newChoices: [group('c2')] }))).toEqual([
      'class',
      'hp',
      'picks',
      'summary',
    ]);
  });
});

describe('the class step and the picks of a new class', () => {
  it('opens every flow, even a level with nothing else to choose', () => {
    expect(steps(fighterOptions())[0]).toBe('class');
  });

  it('adds the skill of the multiclass table to the skills asked, and the instrument makes the picks step', () => {
    const rogue = fighterOptions({
      isNewClass: true,
      proficiencyChoices: [{ kind: LevelUpProficiencyKind.SKILL, count: 1, from: [] }],
    });
    expect(totalsFor(rogue, '').skills).toBe(1);
    expect(steps(rogue)).toEqual(['class', 'hp', 'picks', 'summary']);
    const bard = fighterOptions({
      isNewClass: true,
      proficiencyChoices: [{ kind: LevelUpProficiencyKind.INSTRUMENT, count: 1, from: [] }],
    });
    expect(steps(bard)).toEqual(['class', 'hp', 'picks', 'summary']);
  });

  it('lists the skills of the class list with the ones the character has turned off', () => {
    const o = fighterOptions({
      isNewClass: true,
      proficiencyChoices: [
        {
          kind: LevelUpProficiencyKind.SKILL,
          count: 1,
          from: [
            { key: 'skill:arcana', namePt: 'Arcanismo', alreadyHave: true },
            { key: 'skill:stealth', namePt: 'Furtividade', alreadyHave: false },
          ],
        },
      ],
    });
    expect(
      skillOptions(SKILLS, { ...WIZARD_KEYS, skills: [] }, o).map((i) => [i.key, i.disabled ?? '']),
    ).toEqual([
      ['skill:arcana', 'Você já tem'],
      ['skill:stealth', ''],
    ]);
  });
});
