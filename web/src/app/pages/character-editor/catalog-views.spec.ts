import type {
  BackgroundOptionVm,
  ClassOptionVm,
  RaceOptionVm,
  RulesCatalogVm,
  SubraceOptionVm,
  WeaponOptionVm,
} from './character-editor.types';
import type { CasterSection } from './class-blocks';
import {
  bonusWords,
  catalogNameOf,
  chosenSkillsOf,
  choiceBonusesHintOf,
  sizeChoicesOf,
  choicesSubtitleOf,
  finalConstitutionOf,
  grantedSkillsOf,
  sectionHeadingOf,
  skillNoteOf,
  subclassHintOf,
  subclassWaits,
  weaponDetail,
  weaponGroups,
} from './catalog-views';

const SRD = { fromTable: false, archived: false, off: false };

const klass = (over: Partial<ClassOptionVm> = {}): ClassOptionVm => ({
  key: 'class:wizard',
  namePt: 'Mago',
  hitDie: 6,
  isCaster: true,
  preparation: 'spellbook',
  subclasses: [],
  subclassLevel: 2,
  spellcastingFirstLevel: 1,
  maxSpellLevelByLevel: [],
  skillChoose: 2,
  savingThrows: ['int', 'wis'],
  spellListClassKey: 'class:wizard',
  ...SRD,
  ...over,
});
const subrace = (over: Partial<SubraceOptionVm> = {}): SubraceOptionVm => ({
  key: 'subrace:rock',
  namePt: 'Gnomo da Rocha',
  constitutionBonus: 1,
  ...SRD,
  ...over,
});
const race = (over: Partial<RaceOptionVm> = {}): RaceOptionVm => ({
  key: 'race:gnome',
  namePt: 'Gnomo',
  constitutionBonus: 2,
  choiceBonuses: [],
  subraces: [subrace()],
  ...SRD,
  ...over,
});
const background = (over: Partial<BackgroundOptionVm> = {}): BackgroundOptionVm => ({
  key: 'background:acolyte',
  namePt: 'Acólito',
  equipmentPt: '',
  ...SRD,
  ...over,
});

describe('bonusWords', () => {
  it('lists the bonuses with a plus sign and an "e"', () => {
    expect(bonusWords([2, 1])).toBe('+2 e +1');
    expect(bonusWords([1])).toBe('+1');
  });
});

describe('catalogNameOf', () => {
  const catalog = {
    races: [race()],
    classes: [
      klass({
        subclasses: [
          { key: 'subclass:evo', namePt: 'Evocação', casting: null, alwaysPrepared: [], ...SRD },
        ],
      }),
    ],
    backgrounds: [background()],
    spells: [{ key: 'spell:shield', namePt: 'Escudo Arcano', level: 1, classKeys: [], ...SRD }],
  } as unknown as RulesCatalogVm;

  it('names any entry of the lists, subraces and subclasses included, and nothing else', () => {
    expect(catalogNameOf(catalog, 'race:gnome')).toBe('Gnomo');
    expect(catalogNameOf(catalog, 'subrace:rock')).toBe('Gnomo da Rocha');
    expect(catalogNameOf(catalog, 'class:wizard')).toBe('Mago');
    expect(catalogNameOf(catalog, 'subclass:evo')).toBe('Evocação');
    expect(catalogNameOf(catalog, 'background:acolyte')).toBe('Acólito');
    expect(catalogNameOf(catalog, 'spell:shield')).toBe('Escudo Arcano');
    expect(catalogNameOf(catalog, 'spell:none')).toBeUndefined();
    expect(catalogNameOf(undefined, 'race:gnome')).toBeUndefined();
  });
});

describe('skillNoteOf', () => {
  it('says how many skills the class picks and the saving throws it gives', () => {
    expect(skillNoteOf(klass())).toBe(
      'O Mago escolhe 2 perícias ao começar e dá proficiência nos testes de resistência de Inteligência e Sabedoria.',
    );
  });

  it('leaves out the saving throws when there are none, and says "perícia" for one', () => {
    expect(skillNoteOf(klass({ skillChoose: 1, savingThrows: [] }))).toBe(
      'O Mago escolhe 1 perícia ao começar.',
    );
  });

  it('says nothing without a class or with no skills to pick', () => {
    expect(skillNoteOf(undefined)).toBe('');
    expect(skillNoteOf(klass({ skillChoose: 0 }))).toBe('');
  });
});

describe('choiceBonusesHintOf', () => {
  it("tells where a race's free bonuses go", () => {
    expect(choiceBonusesHintOf(race({ choiceBonuses: [2, 1] }))).toBe(
      'Dá +2 e +1 nas habilidades que você escolher, uma diferente para cada valor: escolha no passo Escolhas.',
    );
  });

  it('says nothing under a subrace that swaps the race bonuses whole', () => {
    expect(
      choiceBonusesHintOf(
        race({ choiceBonuses: [2, 1] }),
        subrace({ replacesAbilityBonuses: true }),
      ),
    ).toBe('');
    expect(choiceBonusesHintOf(race({ choiceBonuses: [2, 1] }), subrace())).not.toBe('');
  });

  it('says nothing for a race with none, or no race', () => {
    expect(choiceBonusesHintOf(race())).toBe('');
    expect(choiceBonusesHintOf(undefined)).toBe('');
  });

  it('lists the sets of a race that offers several, to pick one', () => {
    expect(
      choiceBonusesHintOf(
        race({
          choiceBonusSets: [
            [2, 1],
            [1, 1, 1],
          ],
        }),
      ),
    ).toBe(
      'Dá um destes grupos, nas habilidades que você escolher (uma por valor): +2 e +1 ou +1, +1 e +1. Escolha no passo Escolhas.',
    );
  });
});

describe('sizeChoicesOf', () => {
  it("offers the race's own size and the other one when the race offers a real choice", () => {
    expect(sizeChoicesOf(race({ sizes: ['Small', 'Medium'], size: 'Medium' }))).toEqual([
      { value: '', label: 'Médio (da raça)' },
      { value: 'Small', label: 'Pequeno' },
    ]);
  });

  it('shows no picker for a race with only its own size, or no race', () => {
    expect(sizeChoicesOf(race())).toEqual([]);
    expect(sizeChoicesOf(race({ sizes: ['Medium'], size: 'Medium' }))).toEqual([]);
    expect(sizeChoicesOf(undefined)).toEqual([]);
  });
});

describe('subclassHintOf', () => {
  it('says at which level the class chooses it, until the level gets there', () => {
    expect(subclassHintOf(klass(), 1)).toBe('O Mago escolhe a subclasse no nível 2.');
    expect(subclassHintOf(klass(), 2)).toBe('');
    expect(subclassHintOf(klass({ subclassLevel: 0 }), 1)).toBe('');
    expect(subclassHintOf(undefined, 1)).toBe('');
  });

  it('takes a level that is not a number as short of it', () => {
    expect(subclassHintOf(klass(), Number.NaN)).toBe('O Mago escolhe a subclasse no nível 2.');
  });
});

describe('subclassWaits', () => {
  it('waits for the level, unless a subclass is already chosen', () => {
    expect(subclassWaits(klass(), 1, '')).toBe(true);
    expect(subclassWaits(klass(), 2, '')).toBe(false);
    expect(subclassWaits(klass(), 1, 'subclass:evo')).toBe(false);
    expect(subclassWaits(klass({ subclassLevel: 0 }), 1, '')).toBe(false);
    expect(subclassWaits(undefined, 1, '')).toBe(false);
  });
});

describe('choicesSubtitleOf', () => {
  it('joins the name, the class with its level and the subrace or the race', () => {
    expect(choicesSubtitleOf(' Tharn ', klass(), 3, race(), subrace())).toBe(
      'Tharn · Mago 3 · Gnomo da Rocha',
    );
    expect(choicesSubtitleOf('Tharn', klass(), 3, race(), undefined)).toBe(
      'Tharn · Mago 3 · Gnomo',
    );
  });

  it('leaves out what is missing', () => {
    expect(choicesSubtitleOf('', undefined, 1, undefined, undefined)).toBe('');
    expect(choicesSubtitleOf('Tharn', undefined, 1, undefined, undefined)).toBe('Tharn');
  });
});

describe('finalConstitutionOf', () => {
  it('adds the typed score, the race, the subrace and the manual bonus', () => {
    expect(finalConstitutionOf(14, race(), subrace(), 2)).toBe(19);
  });

  it("leaves out the race's Constitution when the subrace swaps the bonuses whole", () => {
    expect(finalConstitutionOf(14, race(), subrace({ replacesAbilityBonuses: true }), 2)).toBe(17);
  });

  it('counts what is missing as 0', () => {
    expect(finalConstitutionOf(undefined, undefined, undefined, undefined)).toBe(0);
    expect(finalConstitutionOf(10, undefined, undefined, undefined)).toBe(10);
  });
});

describe('the skills the lists give', () => {
  it('names where each comes from, the first source keeping a repeated skill', () => {
    const granted = grantedSkillsOf(
      race({ skillKeys: ['skill:arcana'] }),
      subrace({ skillKeys: ['skill:arcana', 'skill:history'] }),
      background({ skillKeys: ['skill:history', 'skill:religion'] }),
      false,
      [],
    );
    expect([...granted]).toEqual([
      ['skill:arcana', 'da raça'],
      ['skill:history', 'da raça'],
      ['skill:religion', 'do antecedente'],
    ]);
  });

  it("leaves out the race's skills that the subrace replaces", () => {
    const granted = grantedSkillsOf(
      race({ skillKeys: ['skill:arcana', 'skill:perception'] }),
      subrace({ skillKeys: ['skill:history'], replacedSkillKeys: ['skill:perception'] }),
      undefined,
      false,
      [],
    );
    expect([...granted.keys()]).toEqual(['skill:arcana', 'skill:history']);
  });

  it("takes a custom background's skills from the two the person picked, not from the catalog", () => {
    const granted = grantedSkillsOf(
      undefined,
      undefined,
      background({ skillKeys: ['skill:x'] }),
      true,
      ['skill:a', 'skill:b'],
    );
    expect([...granted.keys()]).toEqual(['skill:a', 'skill:b']);
    expect(grantedSkillsOf(undefined, undefined, undefined, false, []).size).toBe(0);
  });

  it('leaves the given skills out of what the player picked', () => {
    const granted = new Map([['skill:arcana', 'da raça']]);
    expect([...chosenSkillsOf(new Set(['skill:arcana', 'skill:history']), granted)]).toEqual([
      'skill:history',
    ]);
  });
});

describe('sectionHeadingOf', () => {
  const section = (over: Partial<CasterSection>): CasterSection => ({
    index: 0,
    classKey: 'class:cleric',
    namePt: 'Clérigo',
    subclassNamePt: '',
    level: 1,
    listClassKey: 'class:cleric',
    preparation: 'prepared',
    maxCircle: null,
    firstLevel: 1,
    alwaysPrepared: [],
    alwaysSourcePt: '',
    ...over,
  });

  it('names the class, then the subclass, then how far the spells reach', () => {
    expect(sectionHeadingOf(section({}))).toBe('Clérigo');
    expect(sectionHeadingOf(section({ subclassNamePt: 'Vida', maxCircle: 0 }))).toBe(
      'Clérigo · Vida · só truques',
    );
    expect(sectionHeadingOf(section({ maxCircle: 2 }))).toBe('Clérigo · até o 2º nível de magia');
  });
});

describe('weaponGroups (#580 board 580-16)', () => {
  const w = (key: string, over: Partial<WeaponOptionVm> = {}): WeaponOptionVm => ({
    key,
    namePt: key,
    category: 'simple',
    damage: '1d4',
    ...SRD,
    ...over,
  });
  const weapons = [
    w('equipment:dagger'),
    w('weapon:machado@mesa', { fromTable: true }),
    w('weapon:lanca@mesa', { fromTable: true, off: true }),
    w('weapon:velha@mesa', { fromTable: true, archived: true }),
  ];

  it('puts the book first and the table apart, with the table weapons switched off for the players left out of theirs', () => {
    const g = weaponGroups(weapons, [], false);
    expect(g.book.map((x) => x.key)).toEqual(['equipment:dagger']);
    expect(g.table.map((x) => x.key)).toEqual(['weapon:machado@mesa']);
  });

  it('offers the master a weapon that is switched off, never an archived one', () => {
    expect(weaponGroups(weapons, [], true).table.map((x) => x.key)).toEqual([
      'weapon:machado@mesa',
      'weapon:lanca@mesa',
    ]);
  });

  it('keeps a weapon the sheet already carries, archived or not', () => {
    expect(weaponGroups(weapons, ['weapon:velha@mesa'], false).table.map((x) => x.key)).toContain(
      'weapon:velha@mesa',
    );
  });

  it('says the category and the damage dice of a row, and the category alone for a weapon with no damage', () => {
    expect(weaponDetail(w('equipment:dagger'))).toBe('Simples · 1d4');
    expect(weaponDetail(w('equipment:halberd', { category: 'martial', damage: '1d10' }))).toBe(
      'Marcial · 1d10',
    );
    expect(weaponDetail(w('equipment:net', { category: 'martial', damage: '' }))).toBe('Marcial');
  });
});
