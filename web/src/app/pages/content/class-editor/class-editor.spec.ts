import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { Ability } from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableClassSchema,
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { TableContentClient } from '../../../core/content/content-client';
import { catalog, classDefaults, entry, feature } from '../../../core/content/content-testing';
import { menu } from '../../../core/content/content-testing-menu';
import { ClassEditor } from './class-editor';
import { settle } from '../../../../test-settle';

// The whole editor renders per test (a 20-row grid, every section): on a busy machine that passes the default 5 s.
vi.setConfig({ testTimeout: 30_000 });

describe('ClassEditor', () => {
  const save = vi.fn();
  const cat = catalog();
  const defaults = classDefaults();

  const guardiao = () =>
    entry(TableContentKind.CLASS, 'Guardião do Vale', {
      body: {
        case: 'tableClass',
        value: create(TableClassSchema, {
          namePt: 'Guardião do Vale',
          hitDie: 10,
          savingThrows: [Ability.STRENGTH, Ability.WISDOM],
          skillChoose: 2,
          skillFrom: ['skill:arcana', 'skill:perception', 'skill:investigation'],
          proficiencies: ['proficiency:light-armor', 'proficiency:shields'],
          subclassLevel: 3,
          asiLevels: [4, 8, 12, 16, 19],
          casting: {
            kind: 'half',
            ability: Ability.WISDOM,
            preparation: 'prepared',
            listFrom: 'class:wizard',
            startLevel: 2,
          },
          levels: defaults.tables[3].rows.map((r, i) => ({
            ...r,
            features:
              i === 0
                ? [feature('Vigília', [{ type: 'sense', sense: 'darkvision', rangeFt: 60 }])]
                : [],
          })),
        }),
      },
    });

  function setup(e: ReturnType<typeof guardiao> | null = guardiao()) {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: TableContentClient, useValue: { save } }],
    });
    const fixture = TestBed.createComponent(ClassEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('catalog', cat);
    fixture.componentRef.setInput('menu', menu());
    fixture.componentRef.setInput('defaults', defaults);
    fixture.componentRef.setInput('entry', e);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const text = (el: Element) => (el.textContent ?? '').replace(/\s+/g, ' ');
  const field = (el: HTMLElement, path: string) =>
    el.querySelector<HTMLElement>(`[data-field="${path}"]`)!;
  const cell = (el: HTMLElement, label: string) =>
    el.querySelector<HTMLInputElement>(`input[aria-label="${label}"]`)!;
  const click = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => text(b).includes(label))!
      .click();
  const type = (input: HTMLInputElement, value: string) => {
    input.value = value;
    input.dispatchEvent(new Event('input'));
  };
  const refusal = (...violations: [string, string][]) =>
    new ConnectError('refused', Code.InvalidArgument, undefined, [
      {
        desc: TableContentRefusalSchema,
        value: create(TableContentRefusalSchema, {
          violations: violations.map(([f, r]) =>
            create(TableContentViolationSchema, { field: f, reason: r }),
          ),
        }),
      },
    ]);

  it('draws the class of the artboard from the entry: the basics, the skills pressed, the table from the server', () => {
    const { el } = setup();
    expect((field(el, 'table_class.name_pt') as HTMLInputElement).value).toBe('Guardião do Vale');
    expect((field(el, 'table_class.hit_die') as HTMLSelectElement).selectedOptions[0].text).toBe(
      'd10',
    );
    expect(
      (field(el, 'table_class.saving_throws[0]') as HTMLSelectElement).selectedOptions[0].text,
    ).toBe('Força');
    expect(
      (field(el, 'table_class.saving_throws[1]') as HTMLSelectElement).selectedOptions[0].text,
    ).toBe('Sabedoria');
    expect(text(el)).toContain('Perícias que ele pode escolher (3 de 3)');
    const pressed = Array.from(el.querySelectorAll('.skills [aria-pressed="true"]')).map((b) =>
      text(b).replace('check', '').trim(),
    );
    expect(pressed).toEqual(['Arcanismo', 'Investigação', 'Percepção']);
    expect(cell(el, 'Nível 2, espaços de magia de 1º nível').value).toBe('2');
    expect(cell(el, 'Nível 1, espaços de magia de 1º nível').value).toBe('');
    expect(cell(el, 'Nível 5, bônus de proficiência').value).toBe('+3');
    // The half caster that prepares: no "Magias" column, 5 circles; ASI and subclass levels as locked chips.
    expect(Array.from(el.querySelectorAll('th')).some((th) => text(th) === 'Magias')).toBe(false);
    expect(el.querySelectorAll('thead th[aria-label^="Espaços de"]')).toHaveLength(3);
    expect(text(el.querySelectorAll('tbody tr')[3])).toContain('Incremento no Valor de Habilidade');
    expect(text(el.querySelectorAll('tbody tr')[2])).toContain('Escolha de subclasse');
    expect(text(el.querySelectorAll('tbody tr')[0])).toContain('Vigília');
    expect(text(el.querySelector('.count, app-class-features')!)).toContain(
      '1 de 60 características',
    );
  });

  it('a new class starts from the table of a class that does not cast, and "Metade" pastes the server\'s half-caster table', async () => {
    const { fixture, el } = setup(null);
    expect(el.querySelector('table')!.querySelectorAll('thead th')).toHaveLength(3);
    expect(cell(el, 'Nível 2, bônus de proficiência').value).toBe('+2');
    (
      Array.from(el.querySelectorAll('app-segmented input')).find(
        (i) => (i as HTMLInputElement).value === 'half',
      ) as HTMLInputElement
    ).dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    expect(cell(el, 'Nível 2, espaços de magia de 1º nível').value).toBe('2');
    expect(cell(el, 'Nível 9, espaços de magia de 3º nível').value).toBe('2');
    expect(el.querySelector('.ask')).toBeNull();
  });

  it('asks before replacing a table the master edited, and "Manter a minha tabela" keeps it', async () => {
    const { fixture, el } = setup();
    type(cell(el, 'Nível 5, espaços de magia de 1º nível'), '9');
    await settle(fixture, 3);
    // Full caster: the kind changes the default table, and the edited one is at stake.
    (
      Array.from(el.querySelectorAll('app-segmented input')).find(
        (i) => (i as HTMLInputElement).value === 'full',
      ) as HTMLInputElement
    ).dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    const ask = el.querySelector('.ask')!;
    expect(text(ask)).toContain('Refazer a tabela dos 20 níveis?');
    expect(text(ask)).toContain('Você editou a tabela dos níveis.');
    click(ask as HTMLElement, 'Manter a minha tabela');
    await settle(fixture, 3);
    expect(el.querySelector('.ask')).toBeNull();
    expect(cell(el, 'Nível 5, espaços de magia de 1º nível').value).toBe('9');
    save.mockResolvedValue({ entry: guardiao(), affected: [] });
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.casting.kind).toBe('full');
    expect(save.mock.calls[0][2].value.levels[4].slots[0]).toBe(9);
  });

  it('"Refazer a tabela" puts the new kind\'s rows in', async () => {
    const { fixture, el } = setup();
    type(cell(el, 'Nível 5, espaços de magia de 1º nível'), '9');
    await settle(fixture, 3);
    (
      Array.from(el.querySelectorAll('app-segmented input')).find(
        (i) => (i as HTMLInputElement).value === 'full',
      ) as HTMLInputElement
    ).dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    click(el.querySelector('.ask') as HTMLElement, 'Refazer a tabela');
    await settle(fixture, 3);
    // The full caster of the fixture: 4 first-circle slots at level 5.
    expect(cell(el, 'Nível 5, espaços de magia de 1º nível').value).toBe('4');
  });

  it('an unedited table is replaced without asking', async () => {
    const { fixture, el } = setup();
    (
      Array.from(el.querySelectorAll('app-segmented input')).find(
        (i) => (i as HTMLInputElement).value === 'none',
      ) as HTMLInputElement
    ).dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    expect(el.querySelector('.ask')).toBeNull();
    expect(el.querySelector('table')!.querySelectorAll('thead th')).toHaveLength(3);
  });

  it('sends the cells as typed, the proficiency bonus included, and nothing it did not touch', async () => {
    const { fixture, el } = setup();
    type(cell(el, 'Nível 5, espaços de magia de 3º nível'), '1');
    type(cell(el, 'Nível 1, bônus de proficiência'), '3');
    await settle(fixture, 3);
    expect(cell(el, 'Nível 1, bônus de proficiência').value).toBe('+3');
    save.mockResolvedValue({ entry: guardiao(), affected: [] });
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    const [campaign, sent, body] = save.mock.calls[0];
    expect(campaign).toBe('camp-1');
    expect(sent.key).toBe('class:guardi-o-do-vale@mesa');
    expect(body.case).toBe('tableClass');
    expect(body.value.levels[4].slots).toEqual([4, 2, 1, 0, 0, 0, 0, 0, 0]);
    expect(body.value.levels[0].profBonus).toBe(3);
    expect(body.value.levels[1].profBonus).toBe(2);
    expect(body.value.levels[0].features[0].key).toBe('feature:Vigília');
    expect(body.value.levels[0].features[0].effects).toEqual([
      { type: 'sense', sense: 'darkvision', rangeFt: 60 },
    ]);
  });

  it("adds a feature with an effect from the menu: only the chosen type's fields go", async () => {
    const { fixture, el } = setup();
    click(el, 'Adicionar característica');
    await settle(fixture, 3);
    const base = 'table_class.levels[0].features[1]';
    type(field(el, `${base}.name_pt`) as HTMLInputElement, 'Olhar de coruja');
    await settle(fixture, 3);
    const type_ = field(el, `${base}.effects[0].type`) as HTMLSelectElement;
    type_.value = 'sense';
    type_.dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    const sense = field(el, `${base}.effects[0].sense`) as HTMLSelectElement;
    sense.value = 'darkvision';
    sense.dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    type(field(el, `${base}.effects[0].range_ft`) as HTMLInputElement, '18');
    await settle(fixture, 3);
    save.mockResolvedValue({ entry: guardiao(), affected: [] });
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.levels[0].features[1]).toEqual({
      key: '',
      namePt: 'Olhar de coruja',
      descPt: [],
      effects: [{ type: 'sense', sense: 'darkvision', rangeFt: 60 }],
    });
  });

  it("moves a feature to another level: it goes under that level's row", async () => {
    const { fixture, el } = setup();
    click(el, 'Vigília');
    await settle(fixture, 3);
    const level = el.querySelector<HTMLSelectElement>('app-select-field[lead] select')!;
    level.selectedIndex = 4;
    level.dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    save.mockResolvedValue({ entry: guardiao(), affected: [] });
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    const levels = save.mock.calls[0][2].value.levels;
    expect(levels[0].features).toEqual([]);
    expect(levels[4].features[0].namePt).toBe('Vigília');
  });

  it('puts a refused grid cell on its input, with the focus, the summary and the section marked', async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(refusal(['table_class.levels[4].slots[2]', 'bad_table']));
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    const input = cell(el, 'Nível 5, espaços de magia de 3º nível');
    expect(input.getAttribute('aria-invalid')).toBe('true');
    expect(input.dataset['field']).toBe('table_class.levels[4].slots[2]');
    expect(document.activeElement).toBe(input);
    expect(text(el.querySelector('app-level-grid')!)).toContain(
      'Nível 5, espaços de magia de 3º nível. Espaços de magia: de 0 a 9 por nível',
    );
    expect(text(el.querySelector('[role="alert"]')!)).toContain('1 campo precisa de ajuste');
    expect(text(el.querySelector('app-section-nav')!)).toContain('Com erro: Tabela dos 20 níveis');
    // What was typed stays.
    expect((field(el, 'table_class.name_pt') as HTMLInputElement).value).toBe('Guardião do Vale');
  });

  it('opens the feature a refusal is about, and lands on its formula', async () => {
    const { fixture, el } = setup();
    expect(el.querySelector('app-feature-editor')).toBeNull();
    save.mockRejectedValue(
      refusal(['table_class.levels[0].features[0].effects[0].range_ft', 'bad_value']),
    );
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    const input = field(el, 'table_class.levels[0].features[0].effects[0].range_ft');
    expect(document.activeElement).toBe(input);
    expect(text(input.closest('app-text-field')!)).toContain(
      'Este valor não serve para este efeito.',
    );
  });

  it('does not save a number that is no whole number in a closed feature: it opens that feature and lands on the field', async () => {
    const { fixture, el } = setup();
    click(el, 'Adicionar característica');
    await settle(fixture, 3);
    const base = 'table_class.levels[0].features[1]';
    type(field(el, `${base}.name_pt`) as HTMLInputElement, 'Escolha do vale');
    await settle(fixture, 3);
    const type_ = field(el, `${base}.effects[0].type`) as HTMLSelectElement;
    type_.value = 'choice';
    type_.dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    type(field(el, `${base}.effects[0].count`) as HTMLInputElement, '1.5');
    await settle(fixture, 3);
    click(el, 'Vigília');
    await settle(fixture, 3);
    expect(el.querySelector(`[data-field="${base}.effects[0].count"]`)).toBeNull();
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    expect(save).not.toHaveBeenCalled();
    const count = field(el, `${base}.effects[0].count`);
    expect(document.activeElement).toBe(count);
    expect(text(count.closest('app-text-field')!)).toContain('Escreva um número inteiro');
  });

  it('does not save a start level of the casting that is no whole number: it opens "Mais opções" and lands on the field', async () => {
    const { fixture, el } = setup();
    (
      Array.from(el.querySelectorAll('app-segmented input')).find(
        (i) => (i as HTMLInputElement).value === 'full',
      ) as HTMLInputElement
    ).dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    const start = field(el, 'table_class.casting.start_level') as HTMLInputElement;
    type(start, 'cedo');
    await settle(fixture, 3);
    expect(text(start.closest('app-text-field')!)).toContain('Escreva um número inteiro');
    save.mockClear();
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    expect(save).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(start);
    type(start, '+3');
    await settle(fixture, 3);
    expect(start.value).toBe('+3');
    save.mockResolvedValue({ entry: guardiao(), affected: [] });
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.casting.startLevel).toBe(3);
  });

  it('keeps "Mais opções" open when the start level is typed wrong and then fixed, also when the person opened it', async () => {
    const { fixture, el } = setup();
    (
      Array.from(el.querySelectorAll('app-segmented input')).find(
        (i) => (i as HTMLInputElement).value === 'full',
      ) as HTMLInputElement
    ).dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    const start = field(el, 'table_class.casting.start_level') as HTMLInputElement;
    const more = start.closest('details') as HTMLDetailsElement;
    more.open = true;
    type(start, '1a');
    await settle(fixture, 3);
    expect(more.open).toBe(true);
    type(start, '1');
    await settle(fixture, 3);
    expect(text(start.closest('app-text-field')!)).not.toContain('Escreva um número inteiro');
    expect(more.open).toBe(true);
  });

  it('says "Esta classe tem 61" when the server refuses the 61st feature', async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(refusal(['table_class.levels[0].features[0]', 'limit']));
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    // At the panel head, with the counter, and not on a feature row.
    const head = el.querySelector('[data-field="table_class.features"]')!;
    expect(text(head.parentElement!)).toContain(
      'Esta classe tem 1. Uma classe da mesa não pode ter mais de 60 características.',
    );
    expect(el.querySelector('app-feature-editor')).toBeNull();
  });

  it('lands the refusal of the casting ability on its select', async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(
      refusal(['table_class.casting.ability', 'bad_casting'], ['table_class.hit_die', 'bad_value']),
    );
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    expect(field(el, 'table_class.casting.ability').getAttribute('aria-invalid')).toBe('true');
    expect(field(el, 'table_class.hit_die').getAttribute('aria-invalid')).toBe('true');
    expect(text(el.querySelector('[role="alert"]')!)).toContain('2 campos precisam de ajuste');
  });

  it('proficiencies: the six groups are check rows, the rest a pick list; multiclass minimums go as typed', async () => {
    const { fixture, el } = setup();
    const checks = Array.from(el.querySelectorAll('app-check-row')).map((c) => ({
      label: text(c).replace('check', '').trim(),
      on: (c.querySelector('input') as HTMLInputElement).checked,
    }));
    expect(checks.find((c) => c.label === 'Armadura leve')!.on).toBe(true);
    expect(checks.find((c) => c.label === 'Armadura média')!.on).toBe(false);
    expect(checks.find((c) => c.label === 'Escudos')!.on).toBe(true);
    (
      Array.from(el.querySelectorAll('app-check-row input')).find(
        (i) => text(i.closest('app-check-row')!).replace('check', '').trim() === 'Armadura média',
      ) as HTMLInputElement
    ).dispatchEvent(new Event('change'));
    const mc = el.querySelector<HTMLSelectElement>(
      '[data-field="table_class.minimums"] .req__add select',
    )!;
    mc.value = 'wisdom';
    mc.dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    type(field(el, 'table_class.minimums.wisdom') as HTMLInputElement, '14');
    await settle(fixture, 3);
    save.mockResolvedValue({ entry: guardiao(), affected: [] });
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    const body = save.mock.calls[0][2].value;
    expect(body.proficiencies).toEqual([
      'proficiency:light-armor',
      'proficiency:shields',
      'proficiency:medium-armor',
    ]);
    expect(body.minimums).toMatchObject({ wisdom: 14 });
  });

  it('keeps the subclass level, the ASI levels and the casting list the way the entry had them', async () => {
    const { fixture, el } = setup();
    save.mockResolvedValue({ entry: guardiao(), affected: [] });
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value).toMatchObject({
      subclassLevel: 3,
      asiLevels: [4, 8, 12, 16, 19],
      casting: {
        kind: 'half',
        ability: Ability.WISDOM,
        listFrom: 'class:wizard',
        preparation: 'prepared',
        startLevel: 2,
      },
    });
  });

  it('shows three mistakes at once, each on its own field, from the real paths', async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(
      refusal(
        ['table_class.hit_die', 'bad_value'],
        ['table_class.saving_throws[1]', 'bad_value'],
        ['table_class.levels[4].slots[2]', 'bad_table'],
      ),
    );
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    expect(field(el, 'table_class.hit_die').getAttribute('aria-invalid')).toBe('true');
    expect(field(el, 'table_class.saving_throws[1]').getAttribute('aria-invalid')).toBe('true');
    expect(cell(el, 'Nível 5, espaços de magia de 3º nível').getAttribute('aria-invalid')).toBe(
      'true',
    );
    expect(text(el.querySelector('[role="alert"]')!)).toContain('3 campos precisam de ajuste');
    expect(text(el.querySelector('app-section-nav')!)).toContain('Com erro: Básico');
    expect(text(el.querySelector('app-section-nav')!)).toContain('Com erro: Tabela dos 20 níveis');
  });

  it('keeps the level with a refusal of a whole row: "Nível 7, espaços de magia. Quem conjura precisa de espaços de magia neste nível."', async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(refusal(['table_class.levels[6].slots', 'bad_table']));
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    expect(text(el.querySelector('app-level-grid')!)).toContain(
      'Nível 7, espaços de magia. Quem conjura precisa de espaços de magia neste nível.',
    );
    expect(
      cell(el, 'Nível 7, espaços de magia de 1º nível').getAttribute('aria-describedby'),
    ).toBeNull();
  });

  it('asks the casting question right under the kind, with the focus on its title', async () => {
    const { fixture, el } = setup();
    type(cell(el, 'Nível 5, espaços de magia de 1º nível'), '9');
    await settle(fixture, 3);
    (
      Array.from(el.querySelectorAll('app-segmented input')).find(
        (i) => (i as HTMLInputElement).value === 'full',
      ) as HTMLInputElement
    ).dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    const casting = el.querySelector('app-casting-fields')!;
    const kids = Array.from(casting.children).map((c) => c.tagName.toLowerCase());
    // The kind's control, its note, then the question.
    expect(kids.slice(0, 3)).toEqual(['div', 'app-field-note', 'app-table-question']);
    expect(document.activeElement?.textContent).toContain('Refazer a tabela dos 20 níveis?');
  });

  it('asks before "Restaurar o padrão" replaces an edited table, and says nothing is needed when it is already the default', async () => {
    const { fixture, el } = setup();
    click(el, 'Restaurar o padrão');
    await settle(fixture, 3);
    expect(el.querySelector('app-table-question')).toBeNull();
    type(cell(el, 'Nível 5, espaços de magia de 1º nível'), '9');
    await settle(fixture, 3);
    click(el, 'Restaurar o padrão');
    await settle(fixture, 3);
    expect(text(el.querySelector('app-table-question')!)).toContain(
      'Voltar a tabela ao padrão do Paladino?',
    );
    click(el.querySelector('app-table-question') as HTMLElement, 'Restaurar o padrão');
    await settle(fixture, 3);
    expect(cell(el, 'Nível 5, espaços de magia de 1º nível').value).toBe('4');
  });

  it('the list of sections claims nothing it does not know: no check marks, one section "location"', () => {
    const { el } = setup();
    const nav = el.querySelector('app-section-nav')!;
    expect(nav.querySelectorAll('mat-icon')).toHaveLength(0);
    expect(nav.querySelectorAll('[aria-current="location"]')).toHaveLength(1);
    expect(Array.from(nav.querySelectorAll('button')).map((b) => text(b).trim())).toEqual([
      'Básico',
      'Proficiências',
      'Conjuração',
      'Tabela dos 20 níveis',
      'Características',
      'Subclasse',
    ]);
  });

  it('writes the multiclass prerequisite as one row per requirement: ability, minimum, remove', async () => {
    const { fixture, el } = setup();
    const add = el.querySelector<HTMLSelectElement>(
      '[data-field="table_class.minimums"] .req__add select',
    )!;
    add.value = 'wisdom';
    add.dispatchEvent(new Event('change'));
    await settle(fixture, 3);
    expect(el.querySelectorAll('[data-field="table_class.minimums"] .req__row')).toHaveLength(1);
    type(field(el, 'table_class.minimums.wisdom') as HTMLInputElement, '14');
    await settle(fixture, 3);
    click(
      el.querySelector('[data-field="table_class.minimums"] .req__row') as HTMLElement,
      'Tirar Sabedoria',
    );
    await settle(fixture, 3);
    expect(el.querySelectorAll('[data-field="table_class.minimums"] .req__row')).toHaveLength(0);
    save.mockResolvedValue({ entry: guardiao(), affected: [] });
    click(el, 'Salvar classe');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.minimums).toBeUndefined();
  });

  describe('the edits behind the controls, pinned on the draft', () => {
    /** The component's own methods and draft: the controls call exactly these. */
    interface Api {
      draft(): {
        saves: number[];
        skillChoose: number;
        skillFrom: string[];
        proficiencies: string[];
        multiclassProficiencies: string[];
        minimums: Record<string, number>;
        anyOf: Record<string, number>;
        asiLevels: number[];
        casting: { kind: string; preparation: string };
        rows: { profBonus: number; cantrips: number; spells: number; slots: number[] }[];
        features: { id: string; level: number }[];
      };
      status(): string;
      question(): { kind: string } | null;
      questionText(): string;
      tableNote(): string;
      setSave(i: 0 | 1, ability: number): void;
      toggleSkill(key: string): void;
      setSkillChoose(n: number): void;
      toggleProficiency(key: string, on: boolean): void;
      addOther(key: string): void;
      removeOther(key: string): void;
      othersOf(list: string[]): string[];
      addMulticlassProficiency(key: string): void;
      removeMulticlassProficiency(key: string): void;
      addRequirement(f: 'minimums' | 'anyOf', a: string): void;
      moveRequirement(f: 'minimums' | 'anyOf', from: string, to: string): void;
      setRequirement(f: 'minimums' | 'anyOf', a: string, text: string): void;
      requirementValues(f: 'minimums' | 'anyOf'): string[];
      abilityName(field: string): string;
      setKind(kind: string): void;
      setPreparation(p: string): void;
      patchCasting(p: Record<string, unknown>): void;
      answerQuestion(replace: boolean): void;
      editCell(e: { level: number; field: string; slot?: number; value: number }): void;
      setAsi(levels: string[]): void;
      addFromGrid(level: number): void;
      openFeature(): string;
      subclassLink(e: unknown): string[];
      newSubclassLink(): string[];
    }
    const api = (e: ReturnType<typeof guardiao> | null = guardiao()) => {
      const page = setup(e);
      return {
        ...page,
        c: page.fixture.componentInstance as unknown as Api,
      };
    };

    it('swaps a saving throw, toggles skills and never lets the number to choose go under 0', () => {
      const { c } = api();
      c.setSave(1, Ability.CHARISMA);
      expect(c.draft().saves).toEqual([Ability.STRENGTH, Ability.CHARISMA]);
      c.toggleSkill('skill:arcana');
      c.toggleSkill('skill:stealth');
      expect(c.draft().skillFrom).toEqual([
        'skill:perception',
        'skill:investigation',
        'skill:stealth',
      ]);
      c.setSkillChoose(-3);
      expect(c.draft().skillChoose).toBe(0);
      c.setSkillChoose(4);
      expect(c.draft().skillChoose).toBe(4);
    });

    it('turns a proficiency group on once and off, and lists the others apart from the six groups', () => {
      const { c } = api();
      c.toggleProficiency('proficiency:heavy-armor', true);
      c.toggleProficiency('proficiency:heavy-armor', true);
      expect(c.draft().proficiencies.filter((k) => k === 'proficiency:heavy-armor')).toHaveLength(
        1,
      );
      c.toggleProficiency('proficiency:shields', false);
      expect(c.draft().proficiencies).toEqual([
        'proficiency:light-armor',
        'proficiency:heavy-armor',
      ]);
      c.addOther('proficiency:tool-x');
      expect(c.othersOf(c.draft().proficiencies)).toEqual(['proficiency:tool-x']);
      c.removeOther('proficiency:tool-x');
      expect(c.othersOf(c.draft().proficiencies)).toEqual([]);
      c.addMulticlassProficiency('proficiency:light-armor');
      expect(c.draft().multiclassProficiencies).toEqual(['proficiency:light-armor']);
      c.removeMulticlassProficiency('proficiency:light-armor');
      expect(c.draft().multiclassProficiencies).toEqual([]);
    });

    it('moves a requirement to a free ability only, and types a minimum that falls back to 1', () => {
      const { c } = api();
      c.addRequirement('anyOf', 'strength');
      c.addRequirement('anyOf', 'wisdom');
      expect(c.requirementValues('anyOf')).toEqual(['strength', 'wisdom']);
      expect(c.draft().anyOf['strength']).toBe(13);
      c.moveRequirement('anyOf', 'strength', 'wisdom');
      c.moveRequirement('anyOf', 'strength', 'strength');
      expect(c.requirementValues('anyOf')).toEqual(['strength', 'wisdom']);
      c.setRequirement('anyOf', 'strength', '15 pontos');
      expect(c.draft().anyOf['strength']).toBe(15);
      c.setRequirement('anyOf', 'strength', '');
      expect(c.draft().anyOf['strength']).toBe(1);
      c.moveRequirement('anyOf', 'strength', 'dexterity');
      expect(c.requirementValues('anyOf')).toEqual(['dexterity', 'wisdom']);
      expect(c.abilityName('wisdom')).toBe('Sabedoria');
      expect(c.abilityName('nonsense')).toBe('nonsense');
    });

    it('edits each column of a row, keeps the ASI levels in order, and adds a feature at the level of its row', () => {
      const { c } = api();
      c.editCell({ level: 3, field: 'cantrips', value: 2 });
      c.editCell({ level: 3, field: 'spells', value: 5 });
      c.editCell({ level: 3, field: 'profBonus', value: 9 });
      c.editCell({ level: 3, field: 'slot', slot: 1, value: 6 });
      expect(c.draft().rows[2]).toMatchObject({ cantrips: 2, spells: 5, profBonus: 9 });
      expect(c.draft().rows[2].slots[1]).toBe(6);
      expect(c.draft().rows[3].cantrips).not.toBe(2);
      c.setAsi(['12', '4', '8']);
      expect(c.draft().asiLevels).toEqual([4, 8, 12]);
      c.addFromGrid(5);
      expect(c.draft().features.some((f) => f.level === 5 && f.id === c.openFeature())).toBe(true);
    });

    it('asks about the table on a change of the casting preparation, answers both ways and says what happened', () => {
      const { c } = api();
      c.setPreparation('prepared');
      expect(c.question()).toBeNull();
      c.editCell({ level: 5, field: 'slot', slot: 0, value: 9 });
      c.setPreparation('known');
      expect(c.question()?.kind).toBe('casting');
      expect(c.questionText()).toContain(
        'Você editou a tabela dos níveis. Refazê-la com o padrão do class:ranger',
      );
      c.answerQuestion(false);
      expect(c.question()).toBeNull();
      expect(c.status()).toBe('A conjuração mudou; a tabela ficou como estava.');
      expect(c.draft().rows[4].slots[0]).toBe(9);
      expect(c.tableNote()).toContain('As magias são conhecidas');
      c.setPreparation('prepared');
      c.answerQuestion(true);
      expect(c.status()).toBe('A tabela foi refeita com o padrão.');
      expect(c.draft().rows[4].slots[0]).not.toBe(9);
      expect(c.tableNote()).toContain('As magias são preparadas');
      c.answerQuestion(true);
      expect(c.question()).toBeNull();
    });

    it('says in the note where the numbers come from for each way of casting', () => {
      const { c } = api();
      c.setKind('');
      expect(c.tableNote()).toContain('Uma classe sem conjuração só tem o bônus de proficiência');
      c.setKind('third');
      expect(c.tableNote()).toContain('Preenchida com o padrão de um terço.');
      c.patchCasting({ startLevel: 4 });
      expect(c.draft().casting.kind).toBe('third');
    });

    it('links to a subclass and to a new one in the same table', () => {
      const { c } = api();
      expect(c.newSubclassLink()).toEqual(['/campaigns', 'camp-1', 'content', 'new', 'subclass']);
      expect(c.subclassLink({ key: 'subclass:x' })).toEqual([
        '/campaigns',
        'camp-1',
        'content',
        'entries',
        'subclass:x',
      ]);
    });
  });
});
