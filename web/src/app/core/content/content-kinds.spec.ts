import { create } from '@bufbuild/protobuf';

import {
  TableContentKind,
  TableFeatSchema,
  TableSubclassSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import {
  CONTENT_NAV,
  entryState,
  entrySupport as entrySupportRaw,
  filterEntries,
  KIND_WORDS,
  limitLine,
  navBySlug,
  countOfNav,
  summaryLine,
  savedSentence,
  usageSentence,
} from './content-kinds';
import { entry, mirathel } from './content-testing';

describe('the list of the table content (E10-01 states 1, 2 and 10)', () => {
  const entries = mirathel();
  const nav = (slug: string) => navBySlug(slug)!;

  it('counts every entry of each kind, archived ones included, and says the limit in the nav', () => {
    const counts = Object.fromEntries(CONTENT_NAV.map((n) => [n.slug, countOfNav(entries, n)]));
    expect(counts).toEqual({
      classes: 2,
      subclasses: 2,
      races: 1,
      backgrounds: 1,
      spells: 1,
      feats: 0,
      options: 0,
      classfeatures: 0,
    });
    expect(limitLine(entries)).toBe('7 de 1000 entradas · o limite de uma campanha');
    expect(summaryLine(entries)).toBe('7 entradas, 1 arquivada');
    expect(summaryLine(entries.slice(0, 1))).toBe('1 entrada');
  });

  it('counts a campaign at the limit of 1000 entries', () => {
    const full = Array.from({ length: 1000 }, (_, i) => ({
      ...entries[0],
      key: `class:c${i}@mesa`,
    }));
    expect(limitLine(full)).toBe('1000 de 1000 entradas · o limite de uma campanha');
  });

  it('lists a kind with the archived entries at the end', () => {
    expect(filterEntries(entries, nav('classes'), '', 'all').map((e) => e.namePt)).toEqual([
      'Guardião do Vale',
      'Bardo das Cinzas',
    ]);
  });

  it('filters by "Mostrar": in use, without sheets, archived', () => {
    const classes = nav('classes');
    expect(filterEntries(entries, classes, '', 'inUse').map((e) => e.namePt)).toEqual([
      'Guardião do Vale',
      'Bardo das Cinzas',
    ]);
    expect(filterEntries(entries, classes, '', 'archived').map((e) => e.namePt)).toEqual([
      'Bardo das Cinzas',
    ]);
    expect(filterEntries(entries, classes, '', 'unused')).toEqual([]);
    expect(filterEntries(entries, nav('subclasses'), '', 'unused').map((e) => e.namePt)).toEqual([
      'Tradição da Tinta',
      'Domínio do Caminho',
    ]);
  });

  it('searches the name with no case and no accents', () => {
    expect(
      filterEntries(entries, nav('backgrounds'), 'cartografo', 'all').map((e) => e.namePt),
    ).toEqual(['Cartógrafo do Vale']);
    expect(filterEntries(entries, nav('spells'), 'LÂMINA', 'all')).toHaveLength(1);
    expect(filterEntries(entries, nav('spells'), 'xyz', 'all')).toEqual([]);
  });

  it('says what was saved with the noun of the kind, so the copy agrees', () => {
    expect(savedSentence(TableContentKind.SPELL, 'Lâmina de Nanquim')).toBe(
      'A magia Lâmina de Nanquim foi salva.',
    );
    expect(savedSentence(TableContentKind.RACE, 'Corujeiro')).toBe('A raça Corujeiro foi salva.');
    expect(savedSentence(TableContentKind.BACKGROUND, 'Cartógrafo do Vale')).toBe(
      'O antecedente Cartógrafo do Vale foi salvo.',
    );
    expect(savedSentence(TableContentKind.SUBRACE, 'Da Colina')).toBe(
      'A sub-raça Da Colina foi salva.',
    );
    expect(savedSentence(TableContentKind.CREATURE, 'Lobo da Mesa')).toBe(
      'A criatura Lobo da Mesa foi salva.',
    );
    expect(KIND_WORDS[TableContentKind.CREATURE]).toBe('Criatura');
  });

  it('says the state in words, for the master only', () => {
    expect(entryState(entries[0], true)).toEqual({ text: 'Em uso por 2 fichas', archived: false });
    expect(entryState(entries[1], true)).toEqual({
      text: 'Arquivada · 1 ficha usa',
      archived: true,
    });
    // The word agrees with the kind: a background is "arquivado".
    expect(
      entryState(
        entry(TableContentKind.BACKGROUND, 'Cartógrafo', { archived: true, charactersUsing: 2 }),
        true,
      )?.text,
    ).toBe('Arquivado · 2 fichas usam');
    expect(entryState(entries[2], true)).toBeNull();
    expect(entryState(entry(TableContentKind.CLASS, 'X', { charactersUsing: 1 }), true)?.text).toBe(
      'Em uso por 1 ficha',
    );
    // A player never gets a count or an archived entry: nothing is said.
    expect(entryState(entries[0], false)).toBeNull();
    expect(usageSentence('Corujeiro', 2)).toBe('2 fichas usam Corujeiro agora.');
    expect(usageSentence('Corujeiro', 0)).toBe('Nenhuma ficha usa Corujeiro agora.');
  });

  it('writes the support line of a row from the body', () => {
    const name = (key: string) => (key === 'class:wizard' ? 'Mago' : key);
    const entrySupport = (
      e: Parameters<typeof entrySupportRaw>[0],
      n: (k: string) => string,
      lvl?: (k: string) => number,
    ) => entrySupportRaw(e, n, lvl).replace(/\u00a0/g, ' ');
    expect(entrySupport(entries[0], name)).toBe('d10 · sem conjuração · subclasse no nível 3');
    expect(entrySupport(entries[2], name)).toBe('Subclasse de Mago · nível 2');
    // Level 0 is the class's own level of choosing: the catalog says it.
    const zero = entry(TableContentKind.SUBCLASS, 'Zero', {
      body: {
        case: 'tableSubclass',
        value: create(TableSubclassSchema, { namePt: 'Zero', classKey: 'class:wizard', level: 0 }),
      },
    });
    expect(entrySupport(zero, name, (k) => (k === 'class:wizard' ? 2 : 0))).toBe(
      'Subclasse de Mago · nível 2',
    );
    expect(entrySupport(zero, name)).toBe('Subclasse de Mago');
    expect(entrySupport(entries[4], name)).toBe('Médio · 9 m');
    expect(entrySupport(entries[5], name)).toBe('2 perícias');
    expect(entrySupport(entries[6], name)).toBe('1º nível');
  });

  it('has the feats among the kinds, with the masculine words of "o talento"', () => {
    const feat = entry(TableContentKind.FEAT, 'Mestre das Cordas', { archived: true });
    expect(nav('feats')).toMatchObject({
      plural: 'Talentos',
      newLabel: 'Novo talento',
      createSegment: 'feat',
    });
    expect(filterEntries([feat], nav('feats'), '', 'all')).toHaveLength(1);
    expect(savedSentence(TableContentKind.FEAT, 'Mestre das Cordas')).toBe(
      'O talento Mestre das Cordas foi salvo.',
    );
    expect(entryState(feat, true)?.text).toBe('Arquivado');
  });

  it('writes the support line of a feat from its prerequisite', () => {
    const none = entry(TableContentKind.FEAT, 'Livre', {
      body: { case: 'tableFeat', value: create(TableFeatSchema, { namePt: 'Livre' }) },
    });
    expect(entrySupportRaw(entry(TableContentKind.FEAT, 'Lutador'), (k) => k)).toBe('Força 13');
    expect(entrySupportRaw(none, (k) => k)).toBe('Sem pré-requisito');
  });
});
