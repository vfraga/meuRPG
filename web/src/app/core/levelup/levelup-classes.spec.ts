import { create } from '@bufbuild/protobuf';

import {
  LevelUpDiceRule,
  LevelUpFeatureChoiceSchema,
  LevelUpSpellsKind,
  LevelUpSubclassSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  Ability,
  SkillSchema,
  SpellSchema,
  type Spell,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { LevelUpDraft } from './levelup-draft';
import { spellOptions, stepsFor, totalsFor } from './levelup-flow';
import { SKILLS, SPELLS, WIZARD_KEYS, fighterOptions, wizardOptions } from './levelup-testing';

// Classes other than the wizard: the lists and the counts are the server's, so every class goes through the same code.

const forClass = (key: string, spells: Spell[] = SPELLS): Spell[] =>
  spells.map((s) => create(SpellSchema, { ...s, classKeys: [key] }));

const BARD_SPELLS: Spell[] = [
  ...forClass(
    'class:bard',
    SPELLS.filter((s) => s.level >= 1 && s.level <= 3),
  ),
  // Not on the Bard's list: a cleric's.
  create(SpellSchema, {
    key: 'spell:bless',
    namePt: 'Bênção',
    level: 1,
    schoolNamePt: 'Encantamento',
    classKeys: ['class:cleric'],
  }),
  create(SpellSchema, {
    key: 'spell:bane',
    namePt: 'Perdição',
    level: 1,
    schoolNamePt: 'Encantamento',
    classKeys: ['class:cleric'],
  }),
  create(SpellSchema, {
    key: 'spell:guiding-bolt',
    namePt: 'Raio Guiador',
    level: 1,
    schoolNamePt: 'Evocação',
    classKeys: ['class:cleric'],
  }),
];

describe('a Bard at level 3: the college, and Expertise', () => {
  const lore = create(LevelUpSubclassSchema, {
    key: 'subclass:lore',
    namePt: 'Colégio do Conhecimento',
    skillChoices: 3,
  });
  const valor = create(LevelUpSubclassSchema, {
    key: 'subclass:valor',
    namePt: 'Colégio da Bravura',
    featureChoices: [
      create(LevelUpFeatureChoiceSchema, {
        feature: { key: 'feature:style', namePt: 'Estilo' },
        choose: 1,
        options: [
          { key: 'o:a', namePt: 'A' },
          { key: 'o:b', namePt: 'B' },
        ],
      }),
    ],
  });
  const bard = () =>
    fighterOptions({
      classKey: 'class:bard',
      classNamePt: 'Bardo',
      fromLevel: 2,
      toLevel: 3,
      totalFromLevel: 2,
      totalToLevel: 3,
      hitDie: 8,
      hitPointAverage: 5,
      subclassDue: true,
      subclasses: [lore, valor],
      expertiseChoices: 2,
      spells: 1,
      spellsKind: LevelUpSpellsKind.KNOWN,
      spellListClassKey: 'class:bard',
      maxSpellLevel: 2,
      diceRule: LevelUpDiceRule.PLAYER_CHOOSES,
    });
  const have = {
    ...WIZARD_KEYS,
    known: [],
    prepared: [],
    skills: ['skill:arcana', 'skill:history'],
  };
  const moreSkills = [
    ...SKILLS,
    create(SkillSchema, {
      key: 'skill:acrobatics',
      namePt: 'Acrobacia',
      ability: Ability.DEXTERITY,
    }),
    create(SkillSchema, { key: 'skill:deception', namePt: 'Enganação', ability: Ability.CHARISMA }),
  ];
  const draft = () => new LevelUpDraft(bard(), have, { spells: BARD_SPELLS, skills: moreSkills });

  it('has Vida, Escolhas (the college and Expertise), Magias and Resumo', () => {
    const d = draft();
    expect(d.steps()).toEqual(['hp', 'picks', 'spells', 'summary']);
    expect(d.missing().map((m) => m.id)).toEqual(['subclass', 'expertise', 'spells']);
  });

  it('keeps the Expertise when the player changes the college', () => {
    const d = draft();
    d.setSubclass('subclass:lore');
    d.toggleExpertise('skill:arcana');
    d.toggleExpertise('skill:history');
    d.setSubclass('subclass:valor');
    expect([...d.expertise()]).toEqual(['skill:arcana', 'skill:history']);
    d.setSubclass('subclass:lore');
    expect([...d.expertise()]).toEqual(['skill:arcana', 'skill:history']);
  });

  it("asks for the College of Lore's three skills, and drops them when the college changes (the level asks for none)", () => {
    const d = draft();
    d.setSubclass('subclass:lore');
    expect(d.skillsAsked()).toBe(3);
    expect(d.missing().find((m) => m.id === 'skills')?.text).toBe('Faltam escolher 3 perícias.');
    d.toggleSkill('skill:stealth');
    d.toggleSkill('skill:perception');
    expect(d.skills().size).toBe(2);
    d.setSubclass('subclass:valor');
    expect(d.skills().size).toBe(0);
    expect(d.missing().some((m) => m.id === 'skills')).toBe(false);
  });

  it("drops only the previous subclass's feature options, and the expertise picked in a skill from a subclass's skills goes with them", () => {
    const d = draft();
    d.setSubclass('subclass:valor');
    d.toggleFeature('o:a', ['o:a', 'o:b'], 1);
    expect([...d.features()]).toEqual(['o:a']);
    d.setSubclass('subclass:lore');
    expect(d.features().size).toBe(0);
    d.toggleSkill('skill:stealth');
    d.toggleExpertise('skill:stealth');
    d.setSubclass('subclass:valor');
    // Trimmed to what the level still asks (2 expertise), never wiped.
    expect(d.expertiseAsked()).toBe(2);
  });
});

describe('a wizard whose subclass gives a cantrip and a skill', () => {
  it('asks for them only once the subclass is chosen', () => {
    const o = wizardOptions({
      abilityScoreImprovement: false,
      cantrips: 0,
      spells: 0,
      prepares: false,
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
    const d = new LevelUpDraft(o, WIZARD_KEYS, { spells: SPELLS, skills: SKILLS });
    expect(d.steps()).toEqual(['hp', 'picks', 'summary']);
    d.setSubclass('sub:land');
    expect(d.steps()).toEqual(['hp', 'picks', 'spells', 'summary']);
    expect(d.missing().map((m) => m.id)).toEqual(['skills', 'cantrips']);
    d.toggleSkill('skill:stealth');
    d.toggleCantrip('spell:prestidigitation');
    expect(d.missing()).toEqual([]);
  });
});

describe('a cleric: it prepares from the class list, with no book', () => {
  const clericSpells = forClass(
    'class:cleric',
    SPELLS.filter((s) => s.level >= 1 && s.level <= 3),
  );
  const cleric = (maxAfter: number) =>
    wizardOptions({
      classKey: 'class:cleric',
      classNamePt: 'Clérigo',
      abilityScoreImprovement: false,
      cantrips: 0,
      spells: 0,
      spellsKind: LevelUpSpellsKind.UNSPECIFIED,
      spellListClassKey: 'class:cleric',
      preparedMax: 5,
      preparedMaxAfter: maxAfter,
    });
  const have = {
    cantrips: [],
    known: [],
    prepared: ['spell:magic-missile', 'spell:detect-magic'],
    skills: [],
    expertise: [],
  };

  it('offers every spell of the class list that is not prepared yet, circle by circle', () => {
    const d = new LevelUpDraft(cleric(4), have, { spells: clericSpells, skills: SKILLS });
    expect(d.steps()).toEqual(['hp', 'spells', 'summary']);
    expect(d.preparedItems().map((i) => i.name)).toEqual([
      'Curar Ferimentos',
      'Onda Trovejante',
      'Invisibilidade',
      'Passo Nebuloso',
      'Reflexos',
    ]);
  });

  it('prepares exactly one more when the maximum goes up by one (one place: a second pick replaces the first)', () => {
    const d = new LevelUpDraft(cleric(3), have, { spells: clericSpells, skills: SKILLS });
    expect(d.preparedAsked()).toBe(1);
    expect(d.missing().map((m) => m.text)).toEqual(['Falta preparar 1 magia.']);
    d.togglePrepared('spell:thunderwave');
    d.togglePrepared('spell:misty-step');
    expect([...d.prepared()]).toEqual(['spell:misty-step']);
    expect(d.missing()).toEqual([]);
    expect(d.choices()).toMatchObject({
      preparedSpellKeys: ['spell:misty-step'],
      knownSpellKeys: [],
    });
  });

  it('has no Magias step when the maximum did not move', () => {
    const d = new LevelUpDraft(cleric(2), have, { spells: clericSpells, skills: SKILLS });
    expect(d.steps()).toEqual(['hp', 'summary']);
  });
});

describe("a Bard's Magical Secrets (level 10)", () => {
  const bard10 = () =>
    wizardOptions({
      classKey: 'class:bard',
      classNamePt: 'Bardo',
      abilityScoreImprovement: false,
      cantrips: 0,
      prepares: false,
      spells: 4,
      anyClassSpells: 2,
      spellsKind: LevelUpSpellsKind.KNOWN,
      spellListClassKey: 'class:bard',
      maxSpellLevel: 3,
    });
  const have = { cantrips: [], known: [], prepared: [], skills: [], expertise: [] };

  it('marks the rows from another class and says where they come from', () => {
    const items = spellOptions(bard10(), BARD_SPELLS, have);
    expect(items.find((i) => i.key === 'spell:bless')).toMatchObject({ outside: true });
    expect(items.find((i) => i.key === 'spell:bless')?.sub.replace(/\u00a0/g, ' ')).toContain(
      'de outra classe',
    );
    expect(items.find((i) => i.key === 'spell:thunderwave')?.outside).toBe(false);
  });

  it('lets only two of the four come from outside the list, and turns the other rows off with the reason', () => {
    const d = new LevelUpDraft(bard10(), have, { spells: BARD_SPELLS, skills: SKILLS });
    d.toggleSpell('spell:bless');
    d.toggleSpell('spell:bane');
    d.toggleSpell('spell:guiding-bolt');
    expect([...d.spells()]).toEqual(['spell:bless', 'spell:bane']);
    expect(d.spellItems().find((i) => i.key === 'spell:guiding-bolt')?.disabled).toBe(
      'Limite de 2 de outra classe',
    );
    // The class's own spells still go in, up to the four.
    d.toggleSpell('spell:thunderwave');
    d.toggleSpell('spell:misty-step');
    expect(d.spells().size).toBe(4);
    expect(d.missing().filter((m) => m.step === 'spells')).toEqual([]);
    // Dropping an outside one frees the other outside rows.
    d.toggleSpell('spell:bless');
    expect(d.spellItems().find((i) => i.key === 'spell:guiding-bolt')?.disabled).toBeUndefined();
  });

  it('has the steps and the totals of the level', () => {
    const o = bard10();
    expect(stepsFor(o, totalsFor(o, ''), 0)).toEqual(['hp', 'spells', 'summary']);
  });
});

describe('adopt: the picks that survive a re-read of the sheet', () => {
  it('carries over what is still valid and drops what the new lists no longer have', () => {
    const catalog = { spells: SPELLS, skills: SKILLS };
    const old = new LevelUpDraft(wizardOptions({ preparedMaxAfter: 4 }), WIZARD_KEYS, catalog);
    old.toggleAbility('int');
    old.setHpCard('roll');
    old.rolled.set({ kind: 'app', value: 5 });
    old.toggleCantrip('spell:prestidigitation');
    old.toggleSpell('spell:misty-step');
    old.toggleSpell('spell:mirror-image');
    // The master changed the sheet meanwhile: Prestidigitação is already a cantrip, and the book gained Passo Nebuloso.
    const fresh = new LevelUpDraft(
      wizardOptions({ preparedMaxAfter: 4, keptHitPointRoll: 5 }),
      {
        ...WIZARD_KEYS,
        cantrips: [...WIZARD_KEYS.cantrips, 'spell:prestidigitation'],
        known: [...WIZARD_KEYS.known, 'spell:misty-step'],
      },
      catalog,
    );
    fresh.adopt(old);
    expect([...fresh.abilityKeys()]).toEqual(['int']);
    expect(fresh.hpCard()).toBe('roll');
    expect(fresh.rolled()).toEqual({ kind: 'app', value: 5 });
    expect(fresh.cantrips().size).toBe(0);
    expect([...fresh.spells()]).toEqual(['spell:mirror-image']);
  });
});

describe("a third caster's subclass picked at its level (slice 10.3's LevelUpSubclass fields 8 to 13)", () => {
  const WIZARD_LIST = SPELLS.filter((s) => s.classKeys.includes('class:wizard'));
  const ink = create(LevelUpSubclassSchema, {
    key: 'subclass:ink@mesa',
    namePt: 'Lâmina de Tinta',
    cantrips: 2,
    spells: 3,
    spellsKind: LevelUpSpellsKind.KNOWN,
    spellListClassKey: 'class:wizard',
    maxSpellLevel: 1,
  });
  const champion = create(LevelUpSubclassSchema, { key: 'subclass:champion', namePt: 'Campeão' });
  // A fighter at level 3: nothing casts before the subclass is chosen.
  const fighter = () =>
    fighterOptions({
      fromLevel: 2,
      toLevel: 3,
      totalFromLevel: 2,
      totalToLevel: 3,
      subclassDue: true,
      subclasses: [champion, ink],
      spellListClassKey: '',
      maxSpellLevel: 0,
      spells: 0,
      cantrips: 0,
    });
  const have = { cantrips: [], known: [], prepared: [], skills: [], expertise: [] };
  const draft = () =>
    new LevelUpDraft(fighter(), have, {
      spells: SPELLS,
      skills: SKILLS,
      classes: [
        { key: 'class:wizard', namePt: 'Mago' },
        { key: 'class:fighter', namePt: 'Guerreiro' },
      ],
    });

  it('has no spells step until the subclass that casts is picked', () => {
    const d = draft();
    expect(d.steps()).toEqual(['hp', 'picks', 'summary']);
    d.setSubclass('subclass:champion');
    expect(d.steps()).toEqual(['hp', 'picks', 'summary']);
    d.setSubclass('subclass:ink@mesa');
    expect(d.steps()).toEqual(['hp', 'picks', 'spells', 'summary']);
  });

  it("asks for the subclass's cantrips and spells, from the list it casts from, up to its highest circle", () => {
    const d = draft();
    d.setSubclass('subclass:ink@mesa');
    expect(d.cantripsAsked()).toBe(2);
    expect(d.spellsAsked()).toBe(3);
    expect(d.listName()).toBe('Mago');
    // The wizard's cantrips and 1st-circle spells: nothing from the 2nd circle, nothing of another class.
    expect(d.cantripItems().map((i) => i.key)).toEqual(
      [
        'spell:light',
        'spell:mage-hand',
        'spell:prestidigitation',
        'spell:fire-bolt',
        'spell:shocking-grasp',
      ].sort(
        (a, b) =>
          d.cantripItems().findIndex((i) => i.key === a) -
          d.cantripItems().findIndex((i) => i.key === b),
      ),
    );
    expect(
      d
        .spellItems()
        .map((i) => i.key)
        .sort(),
    ).toEqual(['spell:detect-magic', 'spell:magic-missile', 'spell:thunderwave']);
    expect(d.spellItems().every((i) => !i.outside)).toBe(true);
    expect(
      d
        .missing()
        .filter((m) => m.step === 'spells')
        .map((m) => m.id),
    ).toEqual(['cantrips', 'spells']);
    d.toggleCantrip('spell:light');
    d.toggleCantrip('spell:mage-hand');
    d.toggleSpell('spell:magic-missile');
    d.toggleSpell('spell:detect-magic');
    d.toggleSpell('spell:thunderwave');
    expect(d.missing().filter((m) => m.step === 'spells')).toEqual([]);
    expect(d.choices()).toMatchObject({
      subclassKey: 'subclass:ink@mesa',
      cantripKeys: ['spell:light', 'spell:mage-hand'],
      knownSpellKeys: ['spell:magic-missile', 'spell:detect-magic', 'spell:thunderwave'],
    });
  });

  it('drops the picks of the subclass when another is chosen, and the spells step goes with them', () => {
    const d = draft();
    d.setSubclass('subclass:ink@mesa');
    d.toggleCantrip('spell:light');
    d.toggleSpell('spell:magic-missile');
    d.setSubclass('subclass:champion');
    expect(d.steps()).toEqual(['hp', 'picks', 'summary']);
    expect(d.cantripsAsked()).toBe(0);
    expect(d.cantrips().size).toBe(0);
    expect(d.spells().size).toBe(0);
  });

  it("leaves the server's options as they are when the subclass casts nothing", () => {
    const d = draft();
    d.setSubclass('subclass:champion');
    expect(d.effective()).toBe(d.options);
    expect(WIZARD_LIST.length).toBeGreaterThan(0);
  });

  it('prepares instead of knowing when the subclass prepares (a rogue-like third caster), up to the maximum it states', () => {
    const preparing = create(LevelUpSubclassSchema, {
      key: 'subclass:prep@mesa',
      namePt: 'Tecelão',
      spells: 0,
      prepares: true,
      preparedMaxAfter: 3,
      spellListClassKey: 'class:wizard',
      maxSpellLevel: 1,
      spellsKind: LevelUpSpellsKind.KNOWN,
    });
    const o = fighterOptions({ subclassDue: true, subclasses: [preparing] });
    const d = new LevelUpDraft(o, have, { spells: SPELLS, skills: SKILLS });
    expect(d.steps()).toEqual(['hp', 'picks', 'summary']);
    d.setSubclass('subclass:prep@mesa');
    expect(d.preparedMaxAfter()).toBe(3);
    expect(d.more()).toBe(3);
    expect(d.steps()).toEqual(['hp', 'picks', 'spells', 'summary']);
    expect(
      d
        .preparedItems()
        .map((i) => i.key)
        .sort(),
    ).toEqual(['spell:detect-magic', 'spell:magic-missile', 'spell:thunderwave']);
    expect(d.preparedAsked()).toBe(3);
  });
});
