import { create } from '@bufbuild/protobuf';

import { Ability } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableBackgroundSchema,
  TableClassLevelSchema,
  TableClassSchema,
  TableContentKind,
  TableFeatSchema,
  TableRaceSchema,
  TableSubclassSchema,
  TableSubraceSchema,
  TableSpellSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { readEntry } from './content-read';
import { ensureStop } from './content-read-parts';
import { catalog, entry, feature } from './content-testing';

// The names are the server's (`Content`): a player has no effect menu, so languages, tools, armor and the abilities come named.
const cat = catalog();
const nameOf = (k: string) => cat.nameOf(k);
const flat = (s: string) => s.replace(/\u00a0/g, ' ');

/** A key written as the server writes it ("language:common", "skill:arcana", "ability:1") is never on a screen. */
function noRawKeys(texts: readonly string[]): void {
  for (const t of texts) {
    expect(t).not.toMatch(/\b(language|proficiency|skill|class|ability|damage-type|spell):/);
  }
}

describe('what a player reads of an entry (E10-01 state 9)', () => {
  describe('a subrace and the race it comes from (T7)', () => {
    const RACE = 'race:corujeiro@mesa';
    const olhos = feature('Olhos de caçador', [
      { type: 'proficiency', proficiency: 'skill:perception' },
    ]);
    const parent = [
      olhos,
      feature('Planar'),
      { ...feature('Sentidos da noite'), options: [feature('Olhos de coruja')] },
    ];
    const names = (k: string) => (k === RACE ? 'Corujeiro' : nameOf(k));
    const sub = (init: object) =>
      entry(TableContentKind.SUBRACE, 'Corujeiro das Neves', {
        body: {
          case: 'tableSubrace',
          value: create(TableSubraceSchema, {
            raceKey: RACE,
            abilityBonuses: { constitution: 2, wisdom: 1 },
            traits: [feature('Pele de geada')],
            replacesTraitKeys: [olhos.key],
            replacesAbilityBonuses: true,
            ...init,
          }),
        },
      });

    it("lists its own traits first and the race's that stay, tagged with the race, and says what it replaces", () => {
      const read = readEntry(
        sub({}),
        names,
        () => 0,
        () => ({ traits: parent }),
      );
      expect(read.rows.map((r) => `${r.label}: ${flat(r.value)}`)).toEqual([
        'Raça: Corujeiro',
        'Habilidades: Constituição +2, Sabedoria +1, no lugar da raça',
        'Substitui: Olhos de caçador',
      ]);
      expect(read.sections).toHaveLength(1);
      expect(read.sections[0].items.map((i) => `${i.title}${i.tag ?? ''}`)).toEqual([
        'Pele de geada',
        'Planar(de Corujeiro)',
        'Sentidos da noite(de Corujeiro)',
      ]);
      expect(read.sections[0].items[2].options?.map((o) => o.title)).toEqual(['Olhos de coruja']);
    });

    it('splits the race\'s traits from its own in the editor\'s preview, with no "Substitui" row', () => {
      const read = readEntry(
        sub({}),
        names,
        () => 0,
        () => ({ traits: parent, split: true }),
      );
      expect(read.rows.map((r) => r.label)).toEqual(['Raça', 'Habilidades']);
      expect(read.sections.map((x) => x.title)).toEqual([
        'Traços de Corujeiro',
        'Traços da sub-raça',
      ]);
      expect(read.sections[0].items.map((i) => i.title)).toEqual(['Planar', 'Sentidos da noite']);
    });

    it('names no replaced trait the reader cannot see (RN-23) and adds no race trait', () => {
      const read = readEntry(sub({}), names);
      expect(read.rows.map((r) => r.label)).toEqual(['Raça', 'Habilidades']);
      expect(read.sections[0].items.map((i) => i.title)).toEqual(['Pele de geada']);
    });

    it("says the race's bonuses do not count when the subrace has none of its own and replaces them", () => {
      const read = readEntry(sub({ abilityBonuses: {} }), names);
      expect(read.rows[1].value).toBe('Nenhum; os da raça não valem');
    });

    it('keeps the bonuses plain when the subrace adds them to the race', () => {
      const read = readEntry(sub({ replacesAbilityBonuses: false }), names);
      expect(flat(read.rows[1].value)).toBe('Constituição +2, Sabedoria +1');
    });
  });

  it('reads a race in full: numbers, languages and each trait with its effect', () => {
    const body = create(TableRaceSchema, {
      namePt: 'Corujeiro',
      size: 'Medium',
      speedFt: 30,
      darkvisionFt: 60,
      abilityBonuses: { wisdom: 2, dexterity: 1 },
      languages: ['language:common', 'language:primordial'],
      traits: [
        feature('Olhos de caçador', [{ type: 'proficiency', proficiency: 'skill:perception' }]),
        feature('Planar'),
      ],
    });
    const read = readEntry(
      entry(TableContentKind.RACE, 'Corujeiro', { body: { case: 'tableRace', value: body } }),
      nameOf,
    );
    expect(read.rows.map((r) => `${r.label}: ${flat(r.value)}`)).toEqual([
      'Habilidades: Destreza +1, Sabedoria +2',
      'Tamanho: Médio',
      'Deslocamento: 9 m',
      'Visão no escuro: 18 m',
      'Idiomas: Comum, Primordial',
    ]);
    expect(read.sections[0].title).toBe('Traços');
    // The effect says what the text does not, and every sentence ends with a stop.
    expect(read.sections[0].items[0]).toEqual({
      title: 'Olhos de caçador',
      text: 'Texto. Proficiência em Percepção.',
    });
    expect(read.sections[0].items[1]).toEqual({ title: 'Planar', text: 'Texto.' });
    noRawKeys(read.rows.map((r) => r.value));
  });

  it("never repeats what the trait's text already says, and ends each part with a stop", () => {
    const body = create(TableRaceSchema, {
      namePt: 'X',
      size: 'Small',
      speedFt: 25,
      traits: [
        {
          ...feature('Olhos', [{ type: 'proficiency', proficiency: 'skill:perception' }]),
          descPt: ['Proficiência em Percepção'],
        },
      ],
    });
    const read = readEntry(
      entry(TableContentKind.RACE, 'X', { body: { case: 'tableRace', value: body } }),
      nameOf,
    );
    expect(read.sections[0].items[0].text).toBe('Proficiência em Percepção.');
    expect(ensureStop('Enxergam longe')).toBe('Enxergam longe.');
    expect(ensureStop('Já tem ponto.')).toBe('Já tem ponto.');
    expect(ensureStop('  ')).toBe('');
  });

  it('writes "+2 e +1 à escolha" for a race that lets the player place bonuses', () => {
    const body = create(TableRaceSchema, {
      namePt: 'Livre',
      size: 'Small',
      speedFt: 25,
      choiceBonuses: [2, 1],
    });
    const read = readEntry(
      entry(TableContentKind.RACE, 'Livre', { body: { case: 'tableRace', value: body } }),
      nameOf,
    );
    expect(read.rows[0]).toEqual({ label: 'Habilidades', value: '+2 e +1 à escolha' });
  });

  it('reads a class in full: hit die, "Testes de resistência" (never "Resistência"), skills, casting and each feature by level', () => {
    const levels = Array.from({ length: 20 }, (_, i) =>
      create(TableClassLevelSchema, {
        features:
          i === 0
            ? [
                feature('Vigília', [
                  { type: 'resource', resource: 'vigilia', max: '3', recharge: 'long_rest' },
                ]),
              ]
            : [],
      }),
    );
    const body = create(TableClassSchema, {
      namePt: 'Guardião do Vale',
      hitDie: 10,
      savingThrows: [Ability.STRENGTH, Ability.WISDOM],
      skillChoose: 2,
      skillFrom: ['skill:perception', 'skill:arcana'],
      proficiencies: [
        'proficiency:light-armor',
        'proficiency:medium-armor',
        'proficiency:shields',
        'proficiency:simple-weapons',
        'proficiency:martial-weapons',
      ],
      subclassLevel: 3,
      casting: {
        kind: 'half',
        ability: Ability.WISDOM,
        preparation: 'prepared',
        listFrom: 'class:wizard',
      },
      levels,
    });
    const read = readEntry(
      entry(TableContentKind.CLASS, 'Guardião do Vale', {
        body: { case: 'tableClass', value: body },
      }),
      nameOf,
    );
    const rows = Object.fromEntries(read.rows.map((r) => [r.label, flat(r.value)]));
    expect(rows['Dado de vida']).toBe('d10');
    expect(rows['Testes de resistência']).toBe('Força, Sabedoria');
    expect(rows['Perícias']).toBe('2 de 2: Percepção, Arcanismo');
    expect(rows['Armaduras']).toBe('Armadura leve, Armadura média, Escudos');
    expect(rows['Armas']).toBe('Armas simples, Armas marciais');
    expect(rows['Conjuração']).toBe('Metade · Sabedoria · preparadas');
    expect(rows['Lista de magias']).toBe('A lista do Mago');
    expect(read.rows.some((r) => r.label === 'Resistência' || r.label === 'Resistências')).toBe(
      false,
    );
    expect(read.sections[0].items).toEqual([
      { title: 'Nível 1 · Vigília', text: 'Texto. 3 usos, voltam num descanso longo.' },
    ]);
    noRawKeys(read.rows.map((r) => r.value));
  });

  it('reads the table of a class level by level, and the multiclass prerequisites, in words', () => {
    const levels = Array.from({ length: 20 }, (_, i) =>
      create(TableClassLevelSchema, {
        profBonus: 2 + Math.floor(i / 4),
        cantripsKnown: i === 1 ? 2 : 0,
        slots: i === 4 ? [4, 2] : i === 1 ? [2] : [],
        features: i === 4 ? [feature('Ataque extra', [{ type: 'extra_attack', count: 2 }])] : [],
      }),
    );
    const body = create(TableClassSchema, {
      namePt: 'Guardião do Vale',
      hitDie: 10,
      savingThrows: [Ability.STRENGTH, Ability.WISDOM],
      minimums: { wisdom: 13 },
      anyOf: { strength: 13, dexterity: 13 },
      asiLevels: [4, 8],
      casting: { kind: 'half', ability: Ability.WISDOM, preparation: 'prepared' },
      levels,
    });
    const read = readEntry(
      entry(TableContentKind.CLASS, 'Guardião do Vale', {
        body: { case: 'tableClass', value: body },
      }),
      nameOf,
    );
    const table = read.sections.find((s) => s.title === 'Tabela dos níveis')!;
    expect(table.items).toHaveLength(20);
    expect(table.items[0]).toEqual({ title: 'Nível 1', text: '+2' });
    expect(table.items[1]).toEqual({ title: 'Nível 2', text: '+2 · 2 truques · 2 de 1º' });
    expect(table.items[4]).toEqual({
      title: 'Nível 5',
      text: '+3 · Ataque extra · 4 de 1º, 2 de 2º',
    });
    expect(table.items[3].text).toBe('+2 · Incremento no Valor de Habilidade');
    expect(table.items[2].text).toBe('+2 · Escolha de subclasse');
    const rows = Object.fromEntries(read.rows.map((r) => [r.label, flat(r.value)]));
    expect(rows['Para multiclasse']).toBe('Sabedoria 13');
    expect(rows['Para multiclasse (uma delas)']).toBe('Força 13, Destreza 13');
  });

  it("reads a third caster's subclass in full: where it is chosen, the casting, the list, the table from its start, the spells it always has", () => {
    const body = create(TableSubclassSchema, {
      namePt: 'Tradição da Tinta',
      classKey: 'class:wizard',
      casting: {
        kind: 'third',
        ability: Ability.INTELLIGENCE,
        preparation: 'known',
        listFrom: 'class:wizard',
        startLevel: 3,
        ritual: true,
      },
      levels: [
        { level: 3, cantripsKnown: 2, spellsKnown: 3, slots: [2] },
        { level: 4, cantripsKnown: 2, spellsKnown: 3, slots: [3] },
      ],
      alwaysPrepared: [{ classLevel: 3, spellKey: 'spell:light' }],
    });
    const read = readEntry(
      entry(TableContentKind.SUBCLASS, 'Tradição da Tinta', {
        body: { case: 'tableSubclass', value: body },
      }),
      nameOf,
      () => 2,
    );
    const rows = Object.fromEntries(read.rows.map((r) => [r.label, flat(r.value)]));
    expect(rows['Escolhida no nível']).toBe('2');
    expect(rows['Conjuração']).toBe('Um terço · Inteligência · conhecidas · rituais');
    expect(rows['Lista de magias']).toBe('A lista do Mago');
    const table = read.sections.find((s) => s.title === 'Conjuração por nível')!;
    expect(table.items).toEqual([
      { title: 'Nível 3', text: '2 truques · 3 magias conhecidas · 2 de 1º' },
      { title: 'Nível 4', text: '2 truques · 3 magias conhecidas · 3 de 1º' },
    ]);
    expect(read.sections.find((s) => s.title === 'Sempre preparadas')!.items).toEqual([
      { title: 'Nível 3', text: 'Luz' },
    ]);
  });

  it('reads a background with its tools, skills and languages by name, never by key', () => {
    const body = create(TableBackgroundSchema, {
      namePt: 'Cartógrafo do Vale',
      skills: ['skill:investigation', 'skill:perception'],
      tools: ['proficiency:cartographers-tools'],
      languageChoices: 1,
      equipmentPt: 'Um estojo de mapas, tinta e 10 PO',
      feature: feature('Mapas na memória', [{ type: 'note', textPt: 'Lembra.' }]),
    });
    const read = readEntry(
      entry(TableContentKind.BACKGROUND, 'Cartógrafo do Vale', {
        body: { case: 'tableBackground', value: body },
      }),
      nameOf,
    );
    const rows = Object.fromEntries(read.rows.map((r) => [r.label, flat(r.value)]));
    expect(rows['Perícias']).toBe('Investigação, Percepção');
    expect(rows['Ferramentas']).toBe('Ferramentas de cartógrafo');
    expect(rows['Idiomas à escolha']).toBe('1');
    expect(rows['Equipamento']).toBe('Um estojo de mapas, tinta e 10 PO');
    expect(read.sections[0]).toEqual({
      title: 'Característica',
      items: [{ title: 'Mapas na memória', text: 'Texto.' }],
    });
    noRawKeys(read.rows.map((r) => r.value));
  });

  it("reads a spell with the same rows the master's preview has, with the damage type named by the server, and its text", () => {
    const body = create(TableSpellSchema, {
      namePt: 'Lâmina de Nanquim',
      level: 1,
      castingTime: { unit: 1, amount: 1 },
      range: { kind: 3, distanceFt: 60 },
      duration: { kind: 1 },
      target: { kind: 1 },
      attack: 'ranged',
      damage: [{ damageTypeKey: 'damage-type:necrotic', dice: '2d8' }],
      classKeys: ['class:wizard'],
      descPt: ['Um risco de tinta negra.'],
    });
    const read = readEntry(
      entry(TableContentKind.SPELL, 'Lâmina de Nanquim', {
        body: { case: 'tableSpell', value: body },
      }),
      nameOf,
    );
    const rows = Object.fromEntries(read.rows.map((r) => [r.label, flat(r.value)]));
    expect(rows['Alvo']).toBe('Uma criatura');
    expect(rows['Alcance']).toBe('18 m');
    expect(rows['Dano']).toBe('2d8 necrótico');
    expect(rows['Classes']).toBe('Mago');
    expect(read.text).toEqual(['Um risco de tinta negra.']);
    noRawKeys(read.rows.map((r) => r.value));
  });
});

describe('what a player reads of a feat (MR-025)', () => {
  it('reads the prerequisite in words, the ability increase and the text', () => {
    const e = entry(TableContentKind.FEAT, 'Mestre das Cordas', {
      body: {
        case: 'tableFeat',
        value: create(TableFeatSchema, {
          namePt: 'Mestre das Cordas',
          descPt: ['Você prende quem quiser.'],
          prerequisite: { minimums: { strength: 13 }, proficiencyKey: 'proficiency:medium-armor' },
          effects: [
            { type: 'ability_increase', count: 1, from: ['strength', 'dexterity'], value: '1' },
          ],
        }),
      },
    });
    const read = readEntry(e, nameOf);
    expect(read.rows.map((r) => `${r.label}: ${flat(r.value)}`)).toEqual([
      'Pré-requisito: Força 13 e proficiência em Armadura média',
      'Aumento de habilidade: +1 em uma habilidade à escolha: Força, Destreza',
    ]);
    expect(read.text).toEqual(['Você prende quem quiser.']);
    noRawKeys(read.rows.map((r) => r.value));
  });

  it('names an SRD feature to lack by the name the server sends, for the master and the player', () => {
    const e = entry(TableContentKind.FEAT, 'Sem agarrar', {
      body: {
        case: 'tableFeat',
        value: create(TableFeatSchema, {
          namePt: 'Sem agarrar',
          prerequisite: { lacksFeatureKey: 'feat:grappler', lacksFeatureNamePt: 'Agarrador' },
        }),
      },
    });
    const read = readEntry(e, (k) => k);
    expect(flat(read.rows[0].value)).toBe('não ter Agarrador');
    noRawKeys(read.rows.map((r) => r.value));
  });

  it('reads a hidden "lacks" condition as words, never a key', () => {
    const e = entry(TableContentKind.FEAT, 'Sem a pele', {
      body: {
        case: 'tableFeat',
        value: create(TableFeatSchema, {
          namePt: 'Sem a pele',
          prerequisite: { lacksHidden: true },
        }),
      },
    });
    const read = readEntry(e, nameOf);
    expect(flat(read.rows[0].value)).toBe('uma condição que a mesa não mostra');
    noRawKeys(read.rows.map((r) => r.value));
  });

  it('reads what the player picks, as one row for each choice', () => {
    const read = (effects: object[], options: object[] = []) =>
      readEntry(
        entry(TableContentKind.FEAT, 'Escolhas', {
          body: {
            case: 'tableFeat',
            value: create(TableFeatSchema, { namePt: 'Escolhas', effects, options }),
          },
        }),
        nameOf,
      ).rows.filter((r) => r.label === 'Escolha');
    const own = [{ namePt: 'A' }, { namePt: 'B' }];
    expect(read([{ type: 'choice', choice: 'feature', count: 1 }], own)).toEqual([
      { label: 'Escolha', value: '1 de 2 opções' },
    ]);
    expect(read([{ type: 'choice', choice: 'feature', count: 1, from: ['x'] }], own)).toEqual([
      { label: 'Escolha', value: '1 de 1 opção' },
    ]);
    expect(read([{ type: 'choice', choice: 'feature', count: 1, set: 'fighting_style' }])).toEqual([
      { label: 'Escolha', value: '1 estilo de luta' },
    ]);
    expect(
      read([
        { type: 'choice', choice: 'ability', count: 1, name: 'habilidade' },
        { type: 'choice', choice: 'damage_type', count: 1, name: 'tipo' },
        { type: 'choice', choice: 'ability', count: 2 },
      ]).map((r) => r.value),
    ).toEqual(['1 habilidade', '1 tipo de dano', '2 habilidades']);
    expect(read([{ type: 'choice', choice: 'feature', count: 1 }])).toEqual([]);
    expect(read([{ type: 'modifier' }])).toEqual([]);
  });

  it('reads a minimum on the damage of the type the player picked, never the pick itself', () => {
    const read = readEntry(
      entry(TableContentKind.FEAT, 'Afinidade', {
        body: {
          case: 'tableFeat',
          value: create(TableFeatSchema, {
            namePt: 'Afinidade',
            effects: [
              {
                type: 'roll_minimum',
                rolls: ['damage'],
                value: '2',
                damageType: 'damage-type:@tipo',
              },
            ],
          }),
        },
      }),
      nameOf,
    );
    expect(read.effects?.[0].text).toContain('do tipo escolhido');
    expect(read.effects?.[0].text).not.toContain('@');
  });

  it('lists the options of a feat under its text, leaving out one with no name', () => {
    const read = readEntry(
      entry(TableContentKind.FEAT, 'Estilo', {
        body: {
          case: 'tableFeat',
          value: create(TableFeatSchema, {
            namePt: 'Estilo',
            descPt: ['Escolha um estilo.'],
            options: [
              { namePt: 'Guarda Alta', descPt: ['Classe de Armadura +1.'] },
              { namePt: '', descPt: ['Sem nome.'] },
            ],
          }),
        },
      }),
      nameOf,
    );
    expect(read.options?.map((o) => `${o.title}. ${o.text}`)).toEqual([
      'Guarda Alta. Classe de Armadura +1.',
    ]);
  });

  it('gives each option a few words of what it does only when the reader says them', () => {
    const entryOf = () =>
      entry(TableContentKind.FEAT, 'Estilo', {
        body: {
          case: 'tableFeat',
          value: create(TableFeatSchema, {
            namePt: 'Estilo',
            options: [
              { namePt: 'Guarda Alta', descPt: ['Você mantém a lâmina erguida.'] },
              { namePt: 'Mão Firme', descPt: ['Seu golpe cai no mesmo ponto.'] },
            ],
          }),
        },
      });
    const noSide = readEntry(entryOf(), nameOf);
    expect(noSide.options?.map((o) => o.side)).toEqual([undefined, undefined]);
    const withSide = readEntry(
      entryOf(),
      nameOf,
      () => 0,
      undefined,
      undefined,
      (o) => (o.namePt === 'Guarda Alta' ? 'Classe de Armadura +1' : ''),
    );
    expect(withSide.options?.map((o) => o.side)).toEqual(['Classe de Armadura +1', undefined]);
    expect(withSide.options?.[0].text).toBe('Você mantém a lâmina erguida.');
  });

  it('says "Nenhum" for a feat that asks nothing', () => {
    const e = entry(TableContentKind.FEAT, 'Livre', {
      body: { case: 'tableFeat', value: create(TableFeatSchema, { namePt: 'Livre' }) },
    });
    expect(readEntry(e, nameOf).rows).toEqual([{ label: 'Pré-requisito', value: 'Nenhum' }]);
  });
});

describe('what the master reads of a feature that summons (#578)', () => {
  const summoning = create(TableBackgroundSchema, {
    namePt: 'Guardião',
    feature: {
      namePt: 'Companheiro do Vale',
      descPt: ['Um lobo do vale luta ao seu lado.'],
      summon: {
        creatureKeys: ['creature:lobo@mesa', 'monster:unheard-of'],
        count: '1',
        resourceKey: 'x',
      },
    },
  });

  it('adds "Invoca: ..." to the feature, naming each creature', () => {
    const names = (k: string) => (k === 'creature:lobo@mesa' ? 'Lobo do Vale' : k);
    const read = readEntry(
      entry(TableContentKind.BACKGROUND, 'Guardião', {
        body: { case: 'tableBackground', value: summoning },
      }),
      names,
    );
    expect(read.sections[0].items[0].text).toBe(
      'Um lobo do vale luta ao seu lado. Invoca: Lobo do Vale, Unheard of.',
    );
  });
});
