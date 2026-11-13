import { create } from '@bufbuild/protobuf';

import {
  TableClassLevelSchema,
  TableClassSchema,
  TableContentKind,
  TableContentViolationSchema,
  TableSubclassLevelSchema,
  TableSubclassSchema,
  TableRaceSchema,
  TableSubraceSchema,
  type TableContentViolation,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { entry, feature } from './content-testing';
import {
  NO_LEFT_OUT,
  backInSentence,
  buildLeftOut,
  leftOutNote,
  leftOutTitle,
  parentKeyOf,
  placeOfPath,
  togetherWith,
  usingLineOf,
  violationOfEntry,
} from './left-out';

function v(key: string, field: string, reason: string): TableContentViolation {
  return create(TableContentViolationSchema, { key, field, reason, message: 'english' });
}

const TARGET = 'table_class.levels[0].features[0].effects[0].target';

function guardiao(using = 1) {
  return entry(TableContentKind.CLASS, 'Guardião do Vale', {
    key: 'class:guardiao@mesa',
    charactersUsing: using,
    body: {
      case: 'tableClass',
      value: create(TableClassSchema, {
        namePt: 'Guardião do Vale',
        hitDie: 10,
        levels: [
          create(TableClassLevelSchema, {
            features: [
              feature('Vigília', [{ type: 'modifier', target: 'gone', mode: 'add', value: '2' }]),
            ],
          }),
        ],
      }),
    },
  });
}

describe('where a violation is, in words', () => {
  it('names the level, the feature, the effect and the field of a class feature effect', () => {
    expect(placeOfPath(guardiao(), TARGET)).toEqual({
      parts: ['Nível 1', 'Vigília', 'efeito 1'],
      field: 'O que muda',
    });
  });

  it('takes the level of a subclass from the level itself, not from its place in the list', () => {
    const sub = entry(TableContentKind.SUBCLASS, 'Juramento da Névoa', {
      body: {
        case: 'tableSubclass',
        value: create(TableSubclassSchema, {
          namePt: 'Juramento da Névoa',
          classKey: 'class:guardiao@mesa',
          levels: [
            create(TableSubclassLevelSchema, {
              level: 3,
              features: [feature('Véu', [{ type: 'modifier' }])],
            }),
          ],
        }),
      },
    });
    expect(placeOfPath(sub, 'table_subclass.levels[0].features[0].effects[0].value')).toEqual({
      parts: ['Nível 3', 'Véu', 'efeito 1'],
      field: 'Valor',
    });
  });

  it('names a race trait and its option, and says "traço 2" for a trait with no name', () => {
    const race = entry(TableContentKind.RACE, 'Corujeiro', {
      body: {
        case: 'tableRace',
        value: create(TableRaceSchema, {
          namePt: 'Corujeiro',
          traits: [feature('Olho de coruja'), { ...feature('x'), namePt: '', options: [] }],
        }),
      },
    });
    expect(placeOfPath(race, 'table_race.traits[0].effects[1].mode').parts).toEqual([
      'Olho de coruja',
      'efeito 2',
    ]);
    expect(placeOfPath(race, 'table_race.traits[1].name_pt')).toEqual({
      parts: ['traço 2'],
      field: 'Nome',
    });
    expect(placeOfPath(race, 'table_race.traits[0].options[3].name_pt')).toEqual({
      parts: ['Olho de coruja', 'opção 4'],
      field: 'Nome',
    });
  });

  it('labels a field of the entry itself, and uses the label of the nearest field for a deeper path', () => {
    expect(placeOfPath(guardiao(), 'table_class.hit_die')).toEqual({
      parts: [],
      field: 'Dado de vida',
    });
    expect(placeOfPath(guardiao(), 'table_class.casting.list_from').field).toBe('Lista de magias');
    expect(placeOfPath(guardiao(), 'table_class.skill_from[2]').field).toBe('Perícias da lista');
  });

  it('says what it can for the entry as a whole, a feature of a background, a path it cannot read and an index past the end', () => {
    expect(placeOfPath(guardiao(), '')).toEqual({ parts: [], field: '' });
    expect(placeOfPath(guardiao(), 'table_class.levels[7].features[0].name_pt').parts).toEqual([
      'Nível 8',
      'característica 1',
    ]);
    expect(placeOfPath(guardiao(), 'table_class.@@.hit_die').field).toBe('Dado de vida');
    const bg = entry(TableContentKind.BACKGROUND, 'Cartógrafo', {});
    expect(placeOfPath(bg, 'table_background.feature.effects[0].target').parts).toEqual([
      'característica',
      'efeito 1',
    ]);
  });
});

describe('the entries left out', () => {
  const sub = entry(TableContentKind.SUBCLASS, 'Juramento da Névoa', {
    key: 'subclass:juramento@mesa',
    body: {
      case: 'tableSubclass',
      value: create(TableSubclassSchema, {
        namePt: 'Juramento da Névoa',
        classKey: 'class:guardiao@mesa',
      }),
    },
  });
  const talento = entry(TableContentKind.FEAT, 'Chama Insistente', {
    key: 'feat:chama@mesa',
    charactersUsing: 3,
  });
  const ok = entry(TableContentKind.RACE, 'Corujeiro');

  it('has nothing for a campaign with nothing left out', () => {
    expect(buildLeftOut([guardiao(), ok], [])).toBe(NO_LEFT_OUT);
    expect(NO_LEFT_OUT.count).toBe(0);
  });

  it('counts only what the page shows: a key with no entry is not counted, and an archived entry is no longer reported', () => {
    const retired = { ...talento, archived: true };
    const model = buildLeftOut(
      [guardiao(), retired, ok],
      [
        v('class:guardiao@mesa', TARGET, 'bad_value'),
        v('feat:chama@mesa', '', 'bad_name'),
        v('race:ausente@mesa', '', 'bad_name'),
      ],
    );
    expect(model.count).toBe(1);
    expect(model.rows.map((r) => r.entry.key)).toEqual(['class:guardiao@mesa']);
    expect([...model.keys]).toEqual(['class:guardiao@mesa']);
    expect(buildLeftOut([retired], [v('feat:chama@mesa', '', 'bad_name')])).toBe(NO_LEFT_OUT);
  });

  it('makes a row of each entry to fix, with its subclass under the class, and counts all three', () => {
    const model = buildLeftOut(
      [guardiao(), sub, talento, ok],
      [
        v('class:guardiao@mesa', TARGET, 'bad_value'),
        v('subclass:juramento@mesa', '', 'dangling_reference'),
        v('feat:chama@mesa', 'table_feat.effects[0].damage_type', 'bad_value'),
      ],
    );
    expect(model.count).toBe(3);
    expect([...model.keys].sort()).toEqual([
      'class:guardiao@mesa',
      'feat:chama@mesa',
      'subclass:juramento@mesa',
    ]);
    expect(model.rows.map((r) => r.entry.namePt)).toEqual(['Guardião do Vale', 'Chama Insistente']);
    const [classe, feat] = model.rows;
    expect(classe.placeLine).toBe('Nível 1 · Vigília · efeito 1');
    expect(classe.shortPlace).toBe('Vigília, efeito 1');
    expect(classe.place.field).toBe('O que muda');
    expect(classe.text).toBe('este alvo não está mais no menu de efeitos. Escolha outro.');
    expect(classe.kindWord).toBe('Classe');
    expect(classe.usingLine).toBe('1 ficha usa esta classe.');
    expect(classe.usingShort).toBe('1 ficha usa');
    expect(classe.dependents.map((d) => d.entry.namePt)).toEqual(['Juramento da Névoa']);
    expect(classe.dependents[0].text).toBe(
      'Saiu porque a classe dela, Guardião do Vale, está fora. Volta sozinha quando a classe voltar.',
    );
    expect(feat.dependents).toEqual([]);
    expect(feat.usingLine).toBe('3 fichas usam este talento.');
  });

  it('keeps an entry whose parent is not out as a row of its own, with the reason it has', () => {
    const model = buildLeftOut(
      [sub],
      [v('subclass:juramento@mesa', 'table_subclass.class_key', 'dangling_reference')],
    );
    expect(model.rows).toHaveLength(1);
    expect(model.rows[0].dependents).toEqual([]);
    expect(model.rows[0].text).toBe('a classe desta subclasse não existe mais.');
    expect(model.rows[0].sentence).toBe('A classe desta subclasse não existe mais.');
  });

  it('keeps a subclass that is out for a reason of its own as a row, even under a class that is out', () => {
    const model = buildLeftOut(
      [guardiao(), sub],
      [
        v('class:guardiao@mesa', TARGET, 'bad_value'),
        v('subclass:juramento@mesa', 'table_subclass.level', 'bad_value'),
      ],
    );
    expect(model.rows.map((r) => r.entry.namePt)).toEqual([
      'Guardião do Vale',
      'Juramento da Névoa',
    ]);
    expect(model.rows[0].dependents).toEqual([]);
  });

  it('says "raça" for a subrace and the class feature follows its class', () => {
    const race = entry(TableContentKind.RACE, 'Corujeiro', { key: 'race:corujeiro@mesa' });
    const subrace = entry(TableContentKind.SUBRACE, 'Das Torres', {
      key: 'subrace:torres@mesa',
      body: {
        case: 'tableSubrace',
        value: create(TableSubraceSchema, { namePt: 'Das Torres', raceKey: 'race:corujeiro@mesa' }),
      },
    });
    expect(parentKeyOf(subrace)).toBe('race:corujeiro@mesa');
    const model = buildLeftOut(
      [race, subrace],
      [
        v('race:corujeiro@mesa', 'table_race.speed_ft', 'bad_value'),
        v('subrace:torres@mesa', '', 'dangling_reference'),
      ],
    );
    expect(model.rows[0].dependents[0].text).toContain('a raça dela, Corujeiro');
    const feature_ = entry(TableContentKind.CLASS_FEATURE, 'Golpe');
    expect(parentKeyOf(feature_)).toBe('class:wizard');
    expect(parentKeyOf(ok)).toBe('');
  });

  it('says the entries as a whole, "Fora da campanha", with the right number', () => {
    expect(leftOutTitle(1)).toBe('1 entrada fora da campanha');
    expect(leftOutTitle(3)).toBe('3 entradas fora da campanha');
  });

  it('writes "Junto com ela" for one dependent and for many', () => {
    const model = buildLeftOut(
      [guardiao(), sub],
      [
        v('class:guardiao@mesa', TARGET, 'bad_value'),
        v('subclass:juramento@mesa', '', 'dangling_reference'),
      ],
    );
    expect(togetherWith(model.rows[0])).toEqual({
      names: [{ name: 'Juramento da Névoa', word: 'subclasse' }],
      tail: 'que depende dela',
    });
    const two = {
      ...model.rows[0],
      dependents: [...model.rows[0].dependents, ...model.rows[0].dependents],
    };
    expect(togetherWith(two).tail).toBe('que dependem dela');
  });

  it('writes no line for an entry no sheet uses, and finds the violation of an entry by its key', () => {
    expect(usingLineOf(guardiao(0))).toBe('');
    const list = [v('a', 'x', 'bad_value'), v('b', 'y', 'bad_value')];
    expect(violationOfEntry(list, 'b')?.field).toBe('y');
    expect(violationOfEntry(list, 'c')).toBeUndefined();
  });
});

describe('the note of a left-out entry and the line after a fix (K2, K4)', () => {
  const classe = () => guardiao(2);
  const sub = entry(TableContentKind.SUBCLASS, 'Juramento da Névoa', {
    key: 'subclass:juramento@mesa',
    body: {
      case: 'tableSubclass',
      value: create(TableSubclassSchema, {
        namePt: 'Juramento da Névoa',
        classKey: 'class:guardiao@mesa',
      }),
    },
  });
  const model = (...extra: TableContentViolation[]) =>
    buildLeftOut(
      [classe(), sub, entry(TableContentKind.FEAT, 'Chama Insistente', { key: 'feat:chama@mesa' })],
      [
        v('class:guardiao@mesa', TARGET, 'bad_value'),
        v('subclass:juramento@mesa', '', 'dangling_reference'),
        ...extra,
      ],
    );

  it('has no note for an entry that is not out', () => {
    expect(leftOutNote(entry(TableContentKind.RACE, 'Corujeiro'), model(), true)).toBeNull();
  });

  it('says what is wrong, what to do and what comes back with it, and how many sheets are warned', () => {
    const note = leftOutNote(classe(), model(), true)!;
    expect(note.lead).toBe('Esta classe está fora da campanha.');
    expect(note.text).toBe(
      'As regras não aceitam mais um campo dela, marcado abaixo. Corrija e salve: ela volta, e Juramento da Névoa volta junto.',
    );
    expect(note.using).toBe('2 fichas usam esta classe e estão com aviso lá.');
    expect(note.parent).toBeNull();
  });

  it('agrees with a masculine kind, says "voltam junto" for two dependents, and has no line for no sheet', () => {
    const feat = entry(TableContentKind.FEAT, 'Chama Insistente', {
      key: 'feat:chama@mesa',
      charactersUsing: 1,
    });
    const m = buildLeftOut(
      [feat],
      [v('feat:chama@mesa', 'table_feat.effects[0].damage_type', 'bad_value')],
    );
    const note = leftOutNote(feat, m, true)!;
    expect(note.lead).toBe('Este talento está fora da campanha.');
    expect(note.text).toContain('um campo dele');
    expect(note.text).toContain('Corrija e salve: ele volta.');
    expect(note.using).toBe('1 ficha usa este talento e está com aviso lá.');
    const sub2 = entry(TableContentKind.SUBCLASS, 'Outro', {
      key: 'subclass:outro@mesa',
      body: sub.body,
    });
    const two = buildLeftOut(
      [classe(), sub, sub2],
      [
        v('class:guardiao@mesa', TARGET, 'bad_value'),
        v('subclass:juramento@mesa', '', 'dangling_reference'),
        v('subclass:outro@mesa', '', 'dangling_reference'),
      ],
    );
    expect(leftOutNote(classe(), two, true)!.text).toContain(
      'Juramento da Névoa e Outro voltam junto',
    );
    expect(leftOutNote(guardiao(0), two, true)!.using).toBe('');
  });

  it('says the dependent is out because of its class, with a way to the class, or only that it comes back on a phone', () => {
    const note = leftOutNote(sub, model(), true)!;
    expect(note.text).toBe(
      'Saiu porque a classe dela, Guardião do Vale, está fora. Corrija a classe: ela volta sozinha.',
    );
    expect(note.parent).toEqual({ key: 'class:guardiao@mesa', name: 'Guardião do Vale' });
    expect(leftOutNote(sub, model(), false)!.text).toBe(
      'Saiu porque a classe dela, Guardião do Vale, está fora. Volta sozinha quando a classe voltar.',
    );
  });

  it('says it for a dependent whose class is not named, without a link', () => {
    const only = buildLeftOut([sub], [v('subclass:juramento@mesa', '', 'dangling_reference')]);
    // Its class is not out: it is a row of its own, with its own reason.
    expect(leftOutNote(sub, only, true)!.text).toContain(
      'As regras não aceitam mais esta subclasse',
    );
    const orphan = { rows: [], count: 1, keys: new Set(['subclass:juramento@mesa']) };
    expect(leftOutNote(sub, orphan, true)).toMatchObject({
      text: 'Saiu porque a classe dela está fora.',
      parent: null,
    });
  });

  it('says on a phone to fix it on the laptop, naming the field, or the entry when it has no field', () => {
    expect(leftOutNote(classe(), model(), false)!.text).toBe(
      'As regras não aceitam mais um campo dela, o campo “O que muda”. Para corrigir, abra a entrada no computador.',
    );
    const wholly = buildLeftOut([guardiao(1)], [v('class:guardiao@mesa', '', 'overlay')]);
    expect(leftOutNote(guardiao(1), wholly, false)!.text).toBe(
      'As regras não aceitam mais esta classe: o conteúdo da mesa não fecha com esta entrada. Confira o que ela usa. Para corrigir, abra a entrada no computador.',
    );
    const noLabel = buildLeftOut(
      [guardiao(1)],
      [v('class:guardiao@mesa', 'table_class.nope', 'bad_value')],
    );
    expect(leftOutNote(guardiao(1), noLabel, false)!.text).toContain('marcado na entrada');
  });

  it('says the entry came back, with what came back with it and the sheets that lose the warning', () => {
    const before = model();
    const entries = [classe(), sub];
    expect(backInSentence(before, NO_LEFT_OUT, classe(), entries)).toBe(
      'Guardião do Vale voltou para a campanha. Juramento da Névoa voltou junto, e as fichas que usam esta classe não têm mais o aviso.',
    );
    expect(backInSentence(before, NO_LEFT_OUT, guardiao(1), entries)).toContain(
      'e a ficha que usa esta classe não tem mais o aviso.',
    );
    expect(backInSentence(before, NO_LEFT_OUT, guardiao(0), [guardiao(0)])).toBe(
      'Guardião do Vale voltou para a campanha.',
    );
    expect(backInSentence(before, NO_LEFT_OUT, guardiao(1), [guardiao(1)])).toBe(
      'Guardião do Vale voltou para a campanha. A ficha que usa esta classe não tem mais o aviso.',
    );
  });

  it('says nothing for an entry that was not out or is still out', () => {
    const feat = entry(TableContentKind.FEAT, 'Chama Insistente', { key: 'feat:chama@mesa' });
    expect(backInSentence(NO_LEFT_OUT, NO_LEFT_OUT, feat, [feat])).toBe('');
    expect(backInSentence(model(), model(), classe(), [classe()])).toBe('');
  });

  it('says two returned entries came back "voltaram junto"', () => {
    const sub2 = entry(TableContentKind.SUBCLASS, 'Outro', {
      key: 'subclass:outro@mesa',
      body: sub.body,
    });
    const before = buildLeftOut(
      [classe(), sub, sub2],
      [
        v('class:guardiao@mesa', TARGET, 'bad_value'),
        v('subclass:juramento@mesa', '', 'dangling_reference'),
        v('subclass:outro@mesa', '', 'dangling_reference'),
      ],
    );
    expect(backInSentence(before, NO_LEFT_OUT, guardiao(0), [guardiao(0), sub, sub2])).toBe(
      'Guardião do Vale voltou para a campanha. Juramento da Névoa e Outro voltaram junto.',
    );
  });
});
