import { create } from '@bufbuild/protobuf';

import {
  TableBackgroundSchema,
  TableClassSchema,
  TableCreatureSchema,
  TableContentKind,
  TableEntrySchema,
  TableFeatSchema,
  TableRaceSchema,
  TableSpellSchema,
  TableSubclassSchema,
  TableSubraceSchema,
  type TableEntry,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { readEntry } from './content-read';
import { effectLine } from './content-read-parts';
import { catalog, feature } from './content-testing';

const cat = catalog();
const nameOf = (k: string) => cat.nameOf(k);
const flat = (v: unknown) =>
  JSON.parse(
    JSON.stringify(v, (_, x) => (typeof x === 'bigint' ? String(x) : x)).replace(/ /g, ' '),
  );

const dressed = (kind: TableContentKind, body: TableEntry['body']): TableEntry =>
  create(TableEntrySchema, { key: 'x@mesa', kind, namePt: 'X', body });

const race = (init: object) =>
  dressed(TableContentKind.RACE, {
    case: 'tableRace',
    value: create(TableRaceSchema, init as never),
  });
const subrace = (init: object) =>
  dressed(TableContentKind.SUBRACE, {
    case: 'tableSubrace',
    value: create(TableSubraceSchema, init as never),
  });
const background = (init: object) =>
  dressed(TableContentKind.BACKGROUND, {
    case: 'tableBackground',
    value: create(TableBackgroundSchema, init as never),
  });
const feat = (init: object) =>
  dressed(TableContentKind.FEAT, {
    case: 'tableFeat',
    value: create(TableFeatSchema, init as never),
  });
const spell = (init: object) =>
  dressed(TableContentKind.SPELL, {
    case: 'tableSpell',
    value: create(TableSpellSchema, init as never),
  });
const klass = (init: object) =>
  dressed(TableContentKind.CLASS, {
    case: 'tableClass',
    value: create(TableClassSchema, init as never),
  });
const subclass = (init: object) =>
  dressed(TableContentKind.SUBCLASS, {
    case: 'tableSubclass',
    value: create(TableSubclassSchema, init as never),
  });

const creature = (init: object) =>
  dressed(TableContentKind.CREATURE, {
    case: 'tableCreature',
    value: create(TableCreatureSchema, init as never),
  });

const slots = [4, 2];
const level = (n: number, over: object = {}) => ({
  level: n,
  profBonus: 2,
  features: [feature('Traço ' + n)],
  cantripsKnown: 2,
  spellsKnown: 1,
  slots,
  ...over,
});

const CASES: Record<string, () => unknown> = {
  'race with no numbers and no traits': () => readEntry(race({ size: 'Odd', speedFt: 25 }), nameOf),
  'race with choices and languages': () =>
    readEntry(
      race({
        size: 'Small',
        speedFt: 30,
        choiceBonuses: [1, 2],
        abilityBonuses: { wisdom: 1 },
        languageChoices: 2,
        darkvisionFt: 60,
      }),
      nameOf,
    ),
  'race with only a choice of bonus': () =>
    readEntry(race({ size: 'Large', speedFt: 30, choiceBonuses: [1] }), nameOf),
  'subrace with a race': () =>
    readEntry(
      subrace({ raceKey: 'race:human', abilityBonuses: { strength: 1 }, traits: [feature('A')] }),
      nameOf,
    ),
  'subrace with nothing': () => readEntry(subrace({}), nameOf),
  'background with everything': () =>
    readEntry(
      background({
        skills: ['skill:arcana'],
        tools: ['proficiency:cartographers-tools'],
        languageChoices: 1,
        equipmentPt: 'Um mapa',
        feature: feature('Viajante'),
      }),
      nameOf,
    ),
  'background with an unnamed feature and no skills': () =>
    readEntry(background({ feature: { ...feature(''), namePt: '' } }), nameOf),
  'feat with ability increases': () =>
    readEntry(
      feat({
        descPt: ['Texto.'],
        effects: [
          { type: 'ability_increase', count: 1, value: '1', from: ['strength'] },
          { type: 'ability_increase', count: 2, value: '2', from: [] },
          { type: 'ability_increase', count: 0 },
          { type: 'sense' },
        ],
      }),
      nameOf,
    ),
  'spell with classes': () =>
    readEntry(
      spell({ level: 0, classKeys: ['class:wizard'], descPt: ['A'], higherLevelPt: ['B'] }),
      nameOf,
    ),
  'spell with no classes': () => readEntry(spell({ level: 2 }), nameOf),
  'class with nothing': () => readEntry(klass({ hitDie: 8 }), nameOf),
  'class in full': () =>
    readEntry(
      klass({
        hitDie: 8,
        savingThrows: [1, 2],
        skillChoose: 2,
        skillFrom: ['skill:arcana', 'skill:perception'],
        proficiencies: [
          'proficiency:light-armor',
          'proficiency:shields',
          'proficiency:simple-weapons',
          'proficiency:cartographers-tools',
        ],
        casting: {
          kind: 'pact',
          ability: 4,
          preparation: 'known',
          ritual: true,
          listFrom: 'class:wizard',
        },
        subclassLevel: 2,
        minimums: { strength: 13 },
        anyOf: { dexterity: 13, wisdom: 12 },
        asiLevels: [4],
        levels: [level(1), level(2), level(3), level(4, { profBonus: 0, features: [] })],
      }),
      nameOf,
    ),
  'class that casts on its own list': () =>
    readEntry(
      klass({
        hitDie: 10,
        casting: { kind: 'other', ability: 5, preparation: 'prepared' },
        levels: [level(1, { cantripsKnown: 0, spellsKnown: 0, slots: [] })],
      }),
      nameOf,
    ),
  'subclass with nothing, level from the class': () =>
    readEntry(subclass({ classKey: 'class:wizard' }), nameOf, () => 2),
  'subclass chosen at its own level': () =>
    readEntry(subclass({ classKey: 'class:wizard', level: 3, descPt: ['Texto'] }), nameOf),
  'third caster subclass': () =>
    readEntry(
      subclass({
        classKey: 'class:paladin',
        level: 3,
        casting: {
          kind: 'third',
          ability: 4,
          preparation: 'known',
          listFrom: 'class:wizard',
          startLevel: 5,
        },
        levels: [level(3), level(5), level(7)],
        alwaysPrepared: [{ classLevel: 3, spellKey: 'spell:light' }],
      }),
      nameOf,
    ),
  'third caster subclass at the default start, with no list': () =>
    readEntry(
      subclass({
        classKey: 'class:paladin',
        casting: { kind: 'third', ability: 4, preparation: 'prepared' },
        levels: [level(2, { features: [] }), level(3, { features: [] })],
      }),
      nameOf,
    ),
  'creature with only a base': () => readEntry(creature({ baseKey: 'monster:wolf' }), nameOf),
  'creature in full': () =>
    readEntry(
      creature({
        baseKey: 'monster:wolf',
        hitPoints: 40,
        armorClass: 17,
        damageBonus: -2,
        hitPointsFormula: '4 + 4 * level()',
        armorClassFormula: '13 + prof()',
        damageBonusFormula: 'prof()',
        attackBonusFormula: 'mod("wis") + prof()',
        traits: [{ namePt: 'Elo', textPt: 'Entende o dono.' }],
        attacks: [
          { namePt: 'Investida', melee: true, diceCount: 2, diceSides: 6 },
          { namePt: 'Uivo', diceCount: 1, diceSides: 8, toHit: 'prof()' },
        ],
        damageMods: [
          {
            kind: 'resist',
            damageTypes: ['damage-type:bludgeoning', 'damage-type:slashing'],
            qualifier: 'nonmagical_unless_silvered',
          },
          { kind: 'vulnerable', damageTypes: ['damage-type:fire'] },
        ],
      }),
      nameOf,
    ),
  'an entry with no body': () =>
    readEntry(dressed(TableContentKind.CLASS, { case: undefined } as never), nameOf),
};

const EFFECTS: Record<string, [Parameters<typeof effectLine>[0], string]> = {
  'skill proficiency': [{ type: 'proficiency', proficiency: 'skill:arcana' } as never, ''],
  'any skill': [{ type: 'proficiency', proficiency: 'skill:*' } as never, ''],
  'other proficiency': [{ type: 'proficiency', proficiency: 'proficiency:shields' } as never, ''],
  'sense seen': [{ type: 'sense', sense: 'darkvision', rangeFt: 60 } as never, ''],
  'sense unknown': [{ type: 'sense', sense: 'x-ray', rangeFt: 30 } as never, ''],
  'sense with no range': [{ type: 'sense', sense: 'darkvision', rangeFt: 0 } as never, ''],
  'resource once': [{ type: 'resource', max: '1', recharge: 'dawn' } as never, ''],
  'resource many': [{ type: 'resource', max: '3', recharge: 'short_rest' } as never, ''],
  'resource on initiative': [{ type: 'resource', max: '2', recharge: 'initiative' } as never, ''],
  'resource one per short rest': [
    { type: 'resource', max: '3', recharge: 'restore_one_on_short_rest' } as never,
    '',
  ],
  'resource one per long rest': [
    { type: 'resource', max: '3', recharge: 'restore_one_on_long_rest' } as never,
    '',
  ],
  'resource unknown recharge': [{ type: 'resource', max: '2', recharge: 'zz' } as never, ''],
  'resource formula': [{ type: 'resource', max: 'prof()', recharge: 'dawn' } as never, ''],
  'extra attack': [{ type: 'extra_attack', count: 2 } as never, ''],
  'no extra attack': [{ type: 'extra_attack', count: 0 } as never, ''],
  'another type': [{ type: 'modifier' } as never, ''],
};

const EXPECTED: Record<string, unknown> = {
  'race with no numbers and no traits': {
    rows: [
      { label: 'Habilidades', value: 'Nenhum' },
      { label: 'Tamanho', value: 'Odd' },
      { label: 'Deslocamento', value: '7,5 m' },
    ],
    sections: [],
    text: [],
  },
  'race with choices and languages': {
    rows: [
      { label: 'Habilidades', value: 'Sabedoria +1, +1 e +2 à escolha' },
      { label: 'Tamanho', value: 'Pequeno' },
      { label: 'Deslocamento', value: '9 m' },
      { label: 'Visão no escuro', value: '18 m' },
      { label: 'Idiomas', value: '2 à escolha' },
    ],
    sections: [],
    text: [],
  },
  'race with only a choice of bonus': {
    rows: [
      { label: 'Habilidades', value: '+1 à escolha' },
      { label: 'Tamanho', value: 'Grande' },
      { label: 'Deslocamento', value: '9 m' },
    ],
    sections: [],
    text: [],
  },
  'subrace with a race': {
    rows: [
      { label: 'Raça', value: 'Humano' },
      { label: 'Habilidades', value: 'Força +1' },
    ],
    sections: [{ title: 'Traços', items: [{ title: 'A', text: 'Texto.' }] }],
    text: [],
  },
  'subrace with nothing': {
    rows: [{ label: 'Habilidades', value: 'Nenhum' }],
    sections: [],
    text: [],
  },
  'background with everything': {
    rows: [
      { label: 'Perícias', value: 'Arcanismo' },
      { label: 'Ferramentas', value: 'Ferramentas de cartógrafo' },
      { label: 'Idiomas à escolha', value: '1' },
      { label: 'Equipamento', value: 'Um mapa' },
    ],
    sections: [{ title: 'Característica', items: [{ title: 'Viajante', text: 'Texto.' }] }],
    text: [],
  },
  'background with an unnamed feature and no skills': {
    rows: [{ label: 'Perícias', value: '—' }],
    sections: [],
    text: [],
  },
  'feat with ability increases': {
    rows: [
      { label: 'Pré-requisito', value: 'Nenhum' },
      { label: 'Aumento de habilidade', value: '+1 em uma habilidade à escolha: Força' },
      { label: 'Aumento de habilidade', value: '+2 em 2 habilidades à escolha: ' },
    ],
    sections: [],
    text: ['Texto.'],
  },
  'spell with classes': {
    rows: [
      { label: 'Tempo', value: '1 ação' },
      { label: 'Alcance', value: '—' },
      { label: 'Alvo', value: 'Uma criatura' },
      { label: 'Componentes', value: 'Nenhum' },
      { label: 'Duração', value: 'Instantânea' },
      { label: 'Classes', value: 'Mago' },
    ],
    sections: [],
    text: ['A', 'B'],
  },
  'spell with no classes': {
    rows: [
      { label: 'Tempo', value: '1 ação' },
      { label: 'Alcance', value: '—' },
      { label: 'Alvo', value: 'Uma criatura' },
      { label: 'Componentes', value: 'Nenhum' },
      { label: 'Duração', value: 'Instantânea' },
    ],
    sections: [],
    text: [],
  },
  'class with nothing': {
    rows: [
      { label: 'Dado de vida', value: 'd8' },
      { label: 'Testes de resistência', value: '' },
      { label: 'Escolhe a subclasse', value: 'no nível 3' },
    ],
    sections: [],
    text: [],
  },
  'class in full': {
    rows: [
      { label: 'Dado de vida', value: 'd8' },
      { label: 'Testes de resistência', value: 'Força, Destreza' },
      { label: 'Perícias', value: '2 de 2: Arcanismo, Percepção' },
      { label: 'Armaduras', value: 'Armadura leve, Escudos' },
      { label: 'Armas', value: 'Armas simples' },
      { label: 'Outras proficiências', value: 'Ferramentas de cartógrafo' },
      { label: 'Conjuração', value: 'Pacto · Inteligência · conhecidas · rituais' },
      { label: 'Lista de magias', value: 'A lista do Mago' },
      { label: 'Escolhe a subclasse', value: 'no nível 2' },
      { label: 'Para multiclasse', value: 'Força 13' },
      { label: 'Para multiclasse (uma delas)', value: 'Destreza 13, Sabedoria 12' },
    ],
    sections: [
      {
        title: 'Características',
        items: [
          { title: 'Nível 1 · Traço 1', text: 'Texto.' },
          { title: 'Nível 2 · Traço 2', text: 'Texto.' },
          { title: 'Nível 3 · Traço 3', text: 'Texto.' },
        ],
      },
      {
        title: 'Tabela dos níveis',
        items: [
          {
            title: 'Nível 1',
            text: '+2 · Traço 1 · 2 truques · 1 magia conhecida · 4 de 1º (pacto), 2 de 2º (pacto)',
          },
          {
            title: 'Nível 2',
            text: '+2 · Traço 2, Escolha de subclasse · 2 truques · 1 magia conhecida · 4 de 1º (pacto), 2 de 2º (pacto)',
          },
          {
            title: 'Nível 3',
            text: '+2 · Traço 3 · 2 truques · 1 magia conhecida · 4 de 1º (pacto), 2 de 2º (pacto)',
          },
          {
            title: 'Nível 4',
            text: 'Incremento no Valor de Habilidade · 2 truques · 1 magia conhecida · 4 de 1º (pacto), 2 de 2º (pacto)',
          },
        ],
      },
    ],
    text: [],
  },
  'class that casts on its own list': {
    rows: [
      { label: 'Dado de vida', value: 'd10' },
      { label: 'Testes de resistência', value: '' },
      { label: 'Conjuração', value: 'other · Sabedoria · preparadas' },
      { label: 'Lista de magias', value: 'A própria lista da classe' },
      { label: 'Escolhe a subclasse', value: 'no nível 3' },
    ],
    sections: [
      { title: 'Características', items: [{ title: 'Nível 1 · Traço 1', text: 'Texto.' }] },
      { title: 'Tabela dos níveis', items: [{ title: 'Nível 1', text: '+2 · Traço 1' }] },
    ],
    text: [],
  },
  'subclass with nothing, level from the class': {
    rows: [
      { label: 'Subclasse de', value: 'Mago' },
      { label: 'Escolhida no nível', value: '2' },
    ],
    sections: [],
    text: [],
  },
  'subclass chosen at its own level': {
    rows: [
      { label: 'Subclasse de', value: 'Mago' },
      { label: 'Escolhida no nível', value: '3' },
    ],
    sections: [],
    text: ['Texto'],
  },
  'third caster subclass': {
    rows: [
      { label: 'Subclasse de', value: 'Paladino' },
      { label: 'Escolhida no nível', value: '3' },
      { label: 'Conjuração', value: 'Um terço · Inteligência · conhecidas' },
      { label: 'Lista de magias', value: 'A lista do Mago' },
    ],
    sections: [
      {
        title: 'Características',
        items: [
          { title: 'Nível 3 · Traço 3', text: 'Texto.' },
          { title: 'Nível 5 · Traço 5', text: 'Texto.' },
          { title: 'Nível 7 · Traço 7', text: 'Texto.' },
        ],
      },
      {
        title: 'Conjuração por nível',
        items: [
          { title: 'Nível 5', text: '2 truques · 1 magia conhecida · 4 de 1º, 2 de 2º' },
          { title: 'Nível 7', text: '2 truques · 1 magia conhecida · 4 de 1º, 2 de 2º' },
        ],
      },
      { title: 'Sempre preparadas', items: [{ title: 'Nível 3', text: 'Luz' }] },
    ],
    text: [],
  },
  'third caster subclass at the default start, with no list': {
    rows: [
      { label: 'Subclasse de', value: 'Paladino' },
      { label: 'Conjuração', value: 'Um terço · Inteligência · preparadas' },
    ],
    sections: [
      {
        title: 'Conjuração por nível',
        items: [{ title: 'Nível 3', text: '2 truques · 1 magia conhecida · 4 de 1º, 2 de 2º' }],
      },
    ],
    text: [],
  },
  'creature with only a base': {
    rows: [
      { label: 'Base', value: 'monster:wolf' },
      { label: 'Pontos de vida', value: 'os da base' },
      { label: 'Classe de armadura', value: 'a da base' },
    ],
    sections: [],
    text: [],
  },
  'creature in full': {
    rows: [
      { label: 'Base', value: 'monster:wolf' },
      { label: 'Pontos de vida', value: '40 · com dono, 4 + 4 × nível do dono' },
      { label: 'Classe de armadura', value: '17 · com dono, 13 + bônus de proficiência' },
      { label: 'Bônus no dano', value: '−2 · com dono, bônus de proficiência' },
      {
        label: 'Bônus de ataque',
        value: 'com dono, modificador de Sabedoria + bônus de proficiência',
      },
    ],
    sections: [
      {
        title: 'Resistências, imunidades e vulnerabilidades',
        items: [
          {
            title: 'Resistência a damage-type:bludgeoning e damage-type:slashing',
            text: 'de ataque não mágico nem prateado.',
          },
          { title: 'Vulnerabilidade a fogo', text: 'Todo dano desse tipo.' },
        ],
      },
      { title: 'Traços', items: [{ title: 'Elo', text: 'Entende o dono.' }] },
      {
        title: 'Ataques',
        items: [
          { title: 'Investida', text: '2d6, acerto e dano da criatura.' },
          { title: 'Uivo', text: '1d8.' },
        ],
      },
    ],
    text: [],
  },
  'an entry with no body': { rows: [], sections: [], text: [] },
};
const EXPECTED_EFFECTS: Record<string, string> = {
  'skill proficiency': 'Proficiência em Arcanismo',
  'any skill': '',
  'other proficiency': '',
  'sense seen': 'Visão no escuro 18 m',
  'sense unknown': 'x-ray 9 m',
  'sense with no range': '',
  'resource once': '1 uso, voltam ao amanhecer',
  'resource many': '3 usos, voltam num descanso curto',
  'resource on initiative': '2 usos, um uso volta ao rolar iniciativa, se não restar nenhum',
  'resource one per short rest':
    '3 usos, um descanso curto devolve um uso e um longo devolve todos',
  'resource one per long rest': '3 usos, um descanso longo devolve um uso',
  'resource unknown recharge': '2 usos',
  'resource formula': '',
  'extra attack': 'Ataque extra: 2 ataques',
  'no extra attack': '',
  'another type': '',
};

describe('what a player reads of each kind, branch by branch', () => {
  it.each(Object.keys(CASES))('%s', (name) => {
    const got = flat(CASES[name]());
    expect(got).toEqual(EXPECTED[name]);
  });

  it.each(Object.keys(EFFECTS))('says the effect line of %s', (name) => {
    const got = effectLine(EFFECTS[name][0], nameOf);
    expect(got).toBe(EXPECTED_EFFECTS[name]);
  });
});
