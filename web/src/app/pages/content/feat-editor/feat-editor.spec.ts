import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  type TableEntry,
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
  TableFeatSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { ContentSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { type CatalogVm, catalogVm } from '../../../core/content/catalog';
import { TableContentClient } from '../../../core/content/content-client';
import { catalog, entry } from '../../../core/content/content-testing';
import { menu } from '../../../core/content/content-testing-menu';
import { FeatEditor } from './feat-editor';
import { settle } from '../../../../test-settle';

function refusal(...violations: [string, string][]) {
  return new ConnectError('refused', Code.InvalidArgument, undefined, [
    {
      desc: TableContentRefusalSchema,
      value: create(TableContentRefusalSchema, {
        violations: violations.map(([field, reason]) =>
          create(TableContentViolationSchema, { field, reason }),
        ),
      }),
    },
  ]);
}

describe('FeatEditor (MR-025)', () => {
  const save = vi.fn();
  const stored = () =>
    entry(TableContentKind.FEAT, 'Mestre das Cordas', {
      body: {
        case: 'tableFeat',
        value: create(TableFeatSchema, {
          namePt: 'Mestre das Cordas',
          descPt: ['Você prende quem quiser.'],
          prerequisite: { minimums: { strength: 13 }, level: 4, raceKey: 'race:human' },
          effects: [
            { type: 'ability_increase', count: 1, from: ['strength', 'dexterity'], value: '1' },
          ],
        }),
      },
    });

  const owl = () => {
    const race = entry(TableContentKind.RACE, 'Corujeiro');
    race.off = true;
    return race;
  };
  const withRaces = () =>
    entry(TableContentKind.FEAT, 'Pequeno e Escorregadio', {
      body: {
        case: 'tableFeat',
        value: create(TableFeatSchema, {
          namePt: 'Pequeno e Escorregadio',
          prerequisite: {
            minimums: { dexterity: 13 },
            races: ['race:human', owl().key],
            size: 'Small',
            lacksFeatureKey: 'feat:grappler',
          },
        }),
      },
    });

  function setup(existing: boolean | TableEntry = true, shown: CatalogVm = catalog()) {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [{ provide: TableContentClient, useValue: { save } }],
    });
    const fixture = TestBed.createComponent(FeatEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('catalog', shown);
    fixture.componentRef.setInput('entries', [owl()]);
    fixture.componentRef.setInput('menu', menu());
    if (existing) fixture.componentRef.setInput('entry', existing === true ? stored() : existing);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ');
  const field = (el: HTMLElement, path: string) =>
    el.querySelector<HTMLElement>(`[data-field="${path}"]`)!;
  const click = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => text(b).includes(label))!
      .click();

  const remove = (el: HTMLElement, name: string) =>
    el.querySelector<HTMLButtonElement>(`button[aria-label="Tirar ${name}"]`)!.click();

  it('draws the form from the entry: name, text, the minimum scores, the level, the race and the ability increase', () => {
    const { el } = setup();
    expect((field(el, 'table_feat.name_pt') as HTMLInputElement).value).toBe('Mestre das Cordas');
    expect((field(el, 'table_feat.desc_pt') as HTMLTextAreaElement).value).toBe(
      'Você prende quem quiser.',
    );
    expect((field(el, 'table_feat.prerequisite.minimums.strength') as HTMLInputElement).value).toBe(
      '13',
    );
    expect(
      (field(el, 'table_feat.prerequisite.minimums.dexterity') as HTMLInputElement).value,
    ).toBe('');
    expect((field(el, 'table_feat.prerequisite.level') as HTMLInputElement).value).toBe('4');
    expect(text(field(el, 'table_feat.prerequisite.races').closest('app-search-pick')!)).toContain(
      'Humano',
    );
    expect(
      (field(el, 'table_feat.effects[0].type') as HTMLSelectElement).selectedOptions[0].text,
    ).toBe('Aumento de habilidade');
    expect(text(field(el, 'table_feat.effects[0].from'))).toContain('Força');
  });

  it('writes "Como os jogadores veem" with the same text as the player\'s page', () => {
    const { el } = setup();
    const rows = Array.from(el.querySelectorAll('.preview .rows__row')).map(
      (r) => `${r.querySelector('dt')!.textContent}: ${r.querySelector('dd')!.textContent}`,
    );
    expect(rows[0]).toContain('Pré-requisito: Força 13, ser Humano e nível 4 ou mais');
    expect(text(el.querySelector('.preview')!)).toContain('Você prende quem quiser.');
  });

  it('does not save while a number of the effect is no whole number, and saves it once it reads', async () => {
    const { fixture, el } = setup();
    save.mockResolvedValue({ entry: stored(), affected: [] });
    const count = field(el, 'table_feat.effects[0].count') as HTMLInputElement;
    count.value = '1.5';
    count.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    expect(save).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(count);
    expect(count.getAttribute('aria-invalid')).toBe('true');
    expect(text(el)).toContain('Escreva um número inteiro, como 2.');
    count.value = '2';
    count.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.effects[0].count).toBe(2);
  });

  it('sends the whole feat: the prerequisite as numbers, the effect and no key of its own', async () => {
    const { fixture, el } = setup();
    save.mockResolvedValue({ entry: stored(), affected: [] });
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    const [, sent, body, key] = save.mock.calls[0];
    expect(sent.key).toBe('feat:mestre-das-cordas@mesa');
    expect(key).toEqual(expect.any(String));
    expect(body.case).toBe('tableFeat');
    expect(body.value).toEqual({
      namePt: 'Mestre das Cordas',
      descPt: ['Você prende quem quiser.'],
      prerequisite: {
        minimums: { strength: 13 },
        proficiencyKey: '',
        spellcasting: false,
        raceKey: 'race:human',
        races: [],
        size: '',
        lacksFeatureKey: '',
        level: 4,
      },
      effects: [
        { type: 'ability_increase', count: 1, from: ['strength', 'dexterity'], value: '1' },
      ],
    });
  });

  it("puts the server's paths back on their inputs: the prerequisite, the ability increase and the name", async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(
      refusal(
        ['table_feat.name_pt', 'duplicate_name'],
        ['table_feat.prerequisite.level', 'bad_value'],
        ['table_feat.prerequisite.race_key', 'dangling_reference'],
        ['table_feat.effects[0].count', 'bad_value'],
      ),
    );
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    expect(text(el.querySelector('[role="alert"]')!)).toContain('4 campos precisam de ajuste');
    expect(text(field(el, 'table_feat.name_pt').closest('app-text-field')!)).toContain(
      'Já existe um talento da mesa com este nome.',
    );
    expect(text(field(el, 'table_feat.prerequisite.level').closest('app-text-field')!)).toContain(
      'O nível vai de 1 a 20',
    );
    expect(text(field(el, 'table_feat.prerequisite.races').closest('app-search-pick')!)).toContain(
      'Esta raça ou sub-raça não existe mais.',
    );
    expect(text(field(el, 'table_feat.effects[0].count').closest('app-text-field')!)).toContain(
      'Quantos o jogador escolhe',
    );
    expect(document.activeElement).toBe(field(el, 'table_feat.name_pt'));
  });

  it('a new feat starts empty, asks nothing and is called "Novo talento" in the preview', () => {
    const { el } = setup(false);
    expect((field(el, 'table_feat.name_pt') as HTMLInputElement).value).toBe('');
    expect((field(el, 'table_feat.prerequisite.level') as HTMLInputElement).value).toBe('');
    expect(text(el.querySelector('.preview')!)).toContain('Novo talento');
    expect(text(el.querySelector('.preview')!)).toContain('Pré-requisitoNenhum');
  });

  it('draws the races as chips, the size and the feature to lack, and writes them in the preview by name', () => {
    const { el } = setup(
      withRaces(),
      catalogVm(
        create(ContentSchema, {
          races: [
            { key: owl().key, namePt: 'Corujeiro' },
            { key: 'race:human', namePt: 'Humano' },
          ],
        }),
      ),
    );
    const races = field(el, 'table_feat.prerequisite.races').closest('app-search-pick')!;
    expect(text(races)).toContain('Humano');
    expect(text(races)).toContain('Corujeiro');
    expect(text(races)).toContain('Desligada');
    expect((field(el, 'table_feat.prerequisite.size') as HTMLSelectElement).value).toBe('Small');
    const lack = field(el, 'table_feat.prerequisite.lacks_feature_key').closest('app-search-pick')!;
    expect(text(lack)).toContain('Agarrador · talento do SRD');
    const preview = text(el.querySelector('.preview')!);
    expect(preview).toContain('Destreza 13, ser Humano ou Corujeiro,');
    expect(preview).toContain('ter tamanho pequeno e não ter Agarrador');
    expect(preview).not.toContain('feat:grappler');
  });

  it('saves the races, the size and the feature to lack untouched when only the name changes', async () => {
    const { fixture, el } = setup(withRaces());
    save.mockResolvedValue({ entry: withRaces(), affected: [] });
    const name = field(el, 'table_feat.name_pt') as HTMLInputElement;
    name.value = 'Pequeno e Veloz';
    name.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.prerequisite).toEqual({
      minimums: { dexterity: 13 },
      proficiencyKey: '',
      spellcasting: false,
      raceKey: '',
      races: ['race:human', owl().key],
      size: 'Small',
      lacksFeatureKey: 'feat:grappler',
      level: 0,
    });
  });

  const searchLacks = (el: HTMLElement, fixture: { detectChanges(): void }, query: string) => {
    const lack = field(el, 'table_feat.prerequisite.lacks_feature_key') as HTMLInputElement;
    lack.value = query;
    lack.dispatchEvent(new Event('input'));
    fixture.detectChanges();
  };

  it('lists the features to lack found by name, under Talentos, Características and Traços', () => {
    const { fixture, el } = setup(false);
    searchLacks(el, fixture, 'agar');
    const list = text(el.querySelector('[role="listbox"]')!);
    expect(list).toContain('Talentos');
    expect(list).toContain('Agarrador · talento do SRD');
    expect(list).toContain('Agarrar com firmeza · Monge, nível 6');
    expect(list).toContain('Nenhum traço com esse nome.');
  });

  it('sends the key of the feature picked, and none once its chip is taken off', async () => {
    const { fixture, el } = setup(false);
    save.mockResolvedValue({ entry: stored(), affected: [] });
    searchLacks(el, fixture, 'visao');
    el.querySelector<HTMLElement>('[role="option"]')!.click();
    fixture.detectChanges();
    expect(text(el.querySelector('.chip')!)).toContain('Visão no Escuro · traço do SRD');
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.prerequisite.lacksFeatureKey).toBe('trait:darkvision');
    remove(el, 'Visão no Escuro');
    fixture.detectChanges();
    expect(el.querySelector('.chip')).toBeNull();
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    expect(save.mock.calls[1][2].value.prerequisite.lacksFeatureKey).toBe('');
  });

  it('does not offer the feat being edited in the search', () => {
    const { fixture, el } = setup();
    searchLacks(el, fixture, 'mestre');
    expect(text(el.querySelector('[role="listbox"]')!)).not.toContain('Mestre das Cordas');
  });

  it('moves the single race of an older feat into the list when another race is added, never sending both', async () => {
    const two = catalogVm(
      create(ContentSchema, {
        races: [
          { key: 'race:human', namePt: 'Humano' },
          { key: 'race:elf', namePt: 'Elfo' },
        ],
      }),
    );
    const { fixture, el } = setup(true, two);
    save.mockResolvedValue({ entry: stored(), affected: [] });
    const search = field(el, 'table_feat.prerequisite.races') as HTMLInputElement;
    search.value = '';
    search.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    // The race asked is not offered again.
    expect(text(el.querySelector('[role="listbox"]')!)).not.toContain('Humano');
    el.querySelector<HTMLElement>('[role="option"]')!.click();
    fixture.detectChanges();
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    const p = save.mock.calls[0][2].value.prerequisite;
    expect(p.raceKey).toBe('');
    expect(p.races).toEqual(['race:human', 'race:elf']);
  });

  it('takes the single race off its chip', async () => {
    const { fixture, el } = setup();
    save.mockResolvedValue({ entry: stored(), affected: [] });
    remove(el, 'Humano');
    fixture.detectChanges();
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    const p = save.mock.calls[0][2].value.prerequisite;
    expect(p.raceKey).toBe('');
    expect(p.races).toEqual([]);
  });

  it('sends the size the master picks, in English', async () => {
    const { fixture, el } = setup(false);
    save.mockResolvedValue({ entry: stored(), affected: [] });
    const size = field(el, 'table_feat.prerequisite.size') as HTMLSelectElement;
    expect(Array.from(size.options).map((o) => o.text)).toEqual([
      'Qualquer',
      'Miúdo',
      'Pequeno',
      'Médio',
      'Grande',
    ]);
    size.value = 'Medium';
    size.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.prerequisite.size).toBe('Medium');
  });

  it('puts the refusals of the races, the size and the feature to lack on their fields', async () => {
    const { fixture, el } = setup(withRaces());
    save.mockRejectedValue(
      refusal(
        ['table_feat.prerequisite.races[1]', 'dangling_reference'],
        ['table_feat.prerequisite.size', 'bad_value'],
        ['table_feat.prerequisite.lacks_feature_key', 'bad_value'],
      ),
    );
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    const at = (path: string) =>
      text(field(el, path).closest('app-search-pick, app-select-field')!);
    expect(at('table_feat.prerequisite.races')).toContain(
      'Esta raça ou sub-raça não existe mais. Escolha outra.',
    );
    expect(at('table_feat.prerequisite.size')).toContain(
      'Escolha um tamanho: Miúdo, Pequeno, Médio ou Grande.',
    );
    expect(at('table_feat.prerequisite.lacks_feature_key')).toContain(
      'Um talento não pode pedir que ele mesmo não esteja na ficha.',
    );
  });

  it('outlines the refused race chip and the field, and the refused feature to lack', async () => {
    const { fixture, el } = setup(withRaces());
    save.mockRejectedValue(
      refusal(
        ['table_feat.prerequisite.races[1]', 'dangling_reference'],
        ['table_feat.prerequisite.lacks_feature_key', 'bad_value'],
      ),
    );
    click(el, 'Salvar talento');
    await settle(fixture, 3);
    const races = field(el, 'table_feat.prerequisite.races').closest('app-search-pick')!;
    const chips = Array.from(races.querySelectorAll('.chip'));
    expect(chips.map((c) => c.classList.contains('chip--invalid'))).toEqual([false, true]);
    expect(field(el, 'table_feat.prerequisite.races').getAttribute('aria-invalid')).toBe('true');
    const lack = field(el, 'table_feat.prerequisite.lacks_feature_key');
    expect(lack.getAttribute('aria-invalid')).toBe('true');
    expect(lack.closest('app-search-pick')!.querySelector('.chip--invalid')).not.toBeNull();
  });

  describe('the option panel and the cards "De onde vêm as opções"', () => {
    const veteran = (patch: Record<string, unknown> = {}) =>
      entry(TableContentKind.FEAT, 'Estilo do Veterano', {
        body: {
          case: 'tableFeat',
          value: create(TableFeatSchema, {
            namePt: 'Estilo do Veterano',
            effects: [{ type: 'choice', choice: 'feature', count: 1 }],
            options: [
              {
                namePt: 'Guarda Alta',
                effects: [{ type: 'modifier', target: 'ac', mode: 'add', value: '1' }],
              },
              { namePt: 'Mão Firme', descPt: ['Seu golpe cai sempre no mesmo ponto.'] },
            ],
            ...patch,
          }),
        },
      });
    const panel = (el: HTMLElement) => el.querySelector('app-feat-options')!;
    const cards = (el: HTMLElement) =>
      Array.from(el.querySelectorAll<HTMLInputElement>('app-option-source input[type="radio"]'));

    it('lists the options of the feat in their panel and counts them on the first card', () => {
      const { el } = setup(veteran());
      expect(text(panel(el).querySelector('.count')!)).toBe('2 de 60 opções');
      expect(Array.from(panel(el).querySelectorAll('.row__title')).map((t) => text(t))).toEqual([
        'Opção 1 · Guarda Alta',
        'Opção 2 · Mão Firme',
      ]);
      expect(text(panel(el))).toContain('no efeito 1;');
      expect(text(el.querySelector('app-option-source .desc')!)).toBe('As 2 do painel abaixo.');
      expect(cards(el).findIndex((c) => c.checked)).toBe(0);
    });

    it('reads the choice and the options in "Como os jogadores veem"', () => {
      const { el } = setup(veteran());
      const preview = el.querySelector('.preview')!;
      const rows = Array.from(preview.querySelectorAll('.rows__row')).map((r) => text(r).trim());
      expect(rows).toContain('Escolha1 de 2 opções');
      const options = Array.from(preview.querySelectorAll('.items--nested li')).map((o) =>
        text(o).trim(),
      );
      expect(options).toEqual(['Guarda Alta.', 'Mão Firme. Seu golpe cai sempre no mesmo ponto.']);
    });

    it('draws no cards for a feat with no choice of options, and tells the panel to ask for one', () => {
      const { el } = setup(false);
      expect(el.querySelector('app-option-source')).toBeNull();
      expect(text(panel(el))).toContain('Nenhuma opção ainda.');
    });

    it('sends the options in order, the named ones only, with the choice untouched', async () => {
      const { fixture, el } = setup(veteran());
      save.mockResolvedValue({ entry: veteran(), affected: [] });
      click(panel(el) as HTMLElement, 'Adicionar uma opção');
      fixture.detectChanges();
      el.querySelector<HTMLButtonElement>('button[aria-label="Descer Opção 1"]')!.click();
      fixture.detectChanges();
      click(el, 'Salvar talento');
      await settle(fixture, 3);
      const sent = save.mock.calls[0][2].value;
      expect(sent.options.map((o: { namePt: string }) => o.namePt)).toEqual([
        'Mão Firme',
        'Guarda Alta',
      ]);
      expect(sent.effects).toEqual([{ type: 'choice', choice: 'feature', count: 1 }]);
    });

    it('offers a whole set: the card, the select and the empty-panel note, and sends the set with no list', async () => {
      const { fixture, el } = setup(veteran({ options: [] }));
      save.mockResolvedValue({ entry: veteran(), affected: [] });
      cards(el)[1].click();
      fixture.detectChanges();
      expect((el.querySelector('app-option-source select') as HTMLSelectElement).value).toBe(
        'invocation',
      );
      expect(text(panel(el))).toContain('Este talento oferece o conjunto Invocação mística.');
      expect(
        panel(el)
          .querySelector<HTMLElement>('[data-field="table_feat.options"]')!
          .getAttribute('aria-disabled'),
      ).toBe('true');
      click(el, 'Salvar talento');
      await settle(fixture, 3);
      expect(save.mock.calls[0][2].value.effects[0]).toMatchObject({
        type: 'choice',
        choice: 'feature',
        set: 'invocation',
      });
      expect(save.mock.calls[0][2].value.options).toBeUndefined();
    });

    it('puts each refusal on the field it names: the list, the cards and an option', async () => {
      const { fixture, el } = setup(veteran());
      save.mockRejectedValue(
        refusal(
          ['table_feat.options', 'limit'],
          ['table_feat.effects[0].set', 'bad_value'],
          ['table_feat.options[1].effects[0].target', 'bad_value'],
        ),
      );
      click(el, 'Salvar talento');
      await settle(fixture, 3);
      expect(text(panel(el).querySelector('.add')!)).toContain(
        'O talento passou de 60 opções. Remova uma.',
      );
      expect(text(el.querySelector('app-option-source')!)).toContain(
        'Este talento tem opções próprias: a escolha oferece elas ou um conjunto, não os dois.',
      );
      const rows = panel(el).querySelectorAll('.row');
      expect(rows[0].classList.contains('row--open')).toBe(false);
      expect(rows[1].classList.contains('row--open')).toBe(true);
      expect(text(el.querySelector('[role="alert"]')!)).toContain('3 campos precisam de ajuste');
    });
  });
  describe('the named choices of the feat', () => {
    const pick = (name = 'habilidade') => ({ type: 'choice', choice: 'ability', count: 1, name });
    const prof = (proficiency: string) => ({ type: 'proficiency', proficiency });
    const named = (effects: object[], options: object[] = []) =>
      entry(TableContentKind.FEAT, 'Resiliente', {
        body: {
          case: 'tableFeat',
          value: create(TableFeatSchema, { namePt: 'Resiliente', effects, options }),
        },
      });
    const proficiency = (el: HTMLElement, at: number) =>
      field(el, `table_feat.effects[${at}].proficiency`) as HTMLSelectElement;
    const bar = (el: HTMLElement) => el.querySelector('app-editor-bar')!;

    it('offers the choice to the fields of the effects after it, and says how many effects use it', () => {
      const { el } = setup(named([pick(), prof('save.@habilidade')]));
      const select = proficiency(el, 1);
      expect(Array.from(select.querySelectorAll('optgroup')).map((g) => g.label)).toEqual([
        'A escolha do talento',
        'Valores fixos',
      ]);
      expect(select.selectedOptions[0].label).toContain('Teste de resistência · Habilidade');
      const side = el.querySelector('app-feat-choices')!;
      expect(text(side)).toContain('Escolhas deste talento');
      expect(text(side)).toContain('Habilidade · efeito 1');
      expect(text(side)).toContain('usada em 1 efeito');
      expect(text(side)).toContain(
        'Você não digita nada: "Habilidade" vem antes dos valores fixos nos campos que aceitam uma habilidade.',
      );
    });

    it('draws the side card only when the feat has a named choice', () => {
      expect(setup(false).el.querySelector('app-feat-choices section')).toBeNull();
      expect(
        setup(named([prof('skill:perception')])).el.querySelector('app-feat-choices section'),
      ).toBeNull();
    });

    it('gives the fields of an option fixed values only, and says so', () => {
      const { el } = setup(
        named(
          [pick(), { type: 'choice', choice: 'feature', count: 1 }],
          [{ namePt: 'Mão Firme', effects: [prof('skill:perception')] }],
        ),
      );
      const select = field(el, 'table_feat.options[0].effects[0].proficiency') as HTMLSelectElement;
      expect(select.querySelectorAll('optgroup')).toHaveLength(0);
      expect(text(select)).toContain(
        'As opções do talento não usam as escolhas dele: só valores fixos.',
      );
    });

    it('draws the three cards once, between the fields of the choice and "Mais opções"', () => {
      const { el } = setup(
        entry(TableContentKind.FEAT, 'Veterano', {
          body: {
            case: 'tableFeat',
            value: create(TableFeatSchema, {
              namePt: 'Veterano',
              effects: [{ type: 'choice', choice: 'feature', count: 1, set: 'invocation' }],
            }),
          },
        }),
      );
      expect(el.querySelectorAll('[data-field="table_feat.effects[0].set"]')).toHaveLength(1);
      expect(el.querySelector('app-effect-picker app-option-source')).not.toBeNull();
      const cards = el.querySelector('app-option-source')!;
      const more = el.querySelector('app-effect-picker button.more')!;
      expect(cards.compareDocumentPosition(more) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect(text(cards.querySelector('app-select-field')!)).toContain('Invocação mística');
    });

    it('keeps the marked options list for a choice with no set, and the refusal on the cards', async () => {
      const { fixture, el } = setup(
        entry(TableContentKind.FEAT, 'Veterano', {
          body: {
            case: 'tableFeat',
            value: create(TableFeatSchema, {
              namePt: 'Veterano',
              effects: [{ type: 'choice', choice: 'feature', count: 1 }],
            }),
          },
        }),
      );
      el.querySelector<HTMLButtonElement>('app-effect-picker button.more')!.click();
      fixture.detectChanges();
      expect(el.querySelector('[data-field="table_feat.effects[0].from"]')).not.toBeNull();
      expect(el.querySelector('[data-field="table_feat.effects[0].set"]')).toBe(
        el.querySelector('app-option-source [data-field]'),
      );
      save.mockRejectedValue(refusal(['table_feat.effects[0].set', 'bad_value']));
      click(el, 'Salvar talento');
      await settle(fixture, 3);
      expect(el.querySelectorAll('[data-field="table_feat.effects[0].set"]')).toHaveLength(1);
      expect(text(el.querySelector('app-option-source')!)).toContain('Este talento tem opções');
    });

    describe('a field whose choice was removed', () => {
      const removed = () => named([prof('save.@habilidade')]);

      it('shows "Escolha…" and the refusal, and keeps the text until another value is picked', () => {
        const { el } = setup(removed());
        const select = proficiency(el, 0);
        expect(select.selectedOptions[0].text).toBe('Escolha…');
        expect(select.getAttribute('aria-invalid')).toBe('true');
        expect(text(select.closest('app-select-field')!)).toContain(
          'A escolha “habilidade” que este campo usava foi removida. Escolha outro valor.',
        );
      });

      it('blocks "Salvar talento" with the reason, and never sends', async () => {
        const { fixture, el } = setup(removed());
        const button = bar(el).querySelector<HTMLButtonElement>('button.mr-button--off')!;
        expect(button.textContent).toContain('Salvar talento');
        expect(text(bar(el))).toContain(
          'Um campo usava uma escolha que foi removida ou ficou abaixo dele. Escolha outro valor.',
        );
        button.click();
        await settle(fixture, 3);
        expect(save).not.toHaveBeenCalled();
      });

      it('saves again once the field holds a value that reads', async () => {
        const { fixture, el } = setup(removed());
        save.mockResolvedValue({ entry: removed(), affected: [] });
        const select = proficiency(el, 0);
        select.value = 'skill:perception';
        select.dispatchEvent(new Event('change'));
        fixture.detectChanges();
        expect(bar(el).querySelector('button.mr-button--off')).toBeNull();
        click(el, 'Salvar talento');
        await settle(fixture, 3);
        expect(save.mock.calls[0][2].value.effects[0].proficiency).toBe('skill:perception');
      });
    });

    it('says the choice is below a field that points at it, and locks the group', () => {
      const { el } = setup(named([prof('save.@habilidade'), pick()]));
      const select = proficiency(el, 0);
      const group = select.querySelector('optgroup')!;
      expect(group.label).toBe('A escolha do talento');
      const row = group.querySelector('option')!;
      expect(row.disabled).toBe(true);
      expect(row.textContent).toContain(
        '“Habilidade” está no efeito 2, abaixo deste. Suba a escolha para usá-la aqui.',
      );
      expect(text(select.closest('app-select-field')!)).toContain(
        'A escolha vem depois deste efeito. Suba a escolha ou desça este efeito.',
      );
      expect(bar(el).querySelector('button.mr-button--off')).not.toBeNull();
    });
  });
});
