import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { Ability } from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
  TableSubclassSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { TableContentClient } from '../../../core/content/content-client';
import { catalog, classDefaults, entry, menu } from '../../../core/content/content-testing';
import { SubclassEditor } from './subclass-editor';

vi.setConfig({ testTimeout: 30_000 });

describe('SubclassEditor', () => {
  const save = vi.fn();
  const cat = catalog();
  const defaults = classDefaults();

  const tinta = () =>
    entry(TableContentKind.SUBCLASS, 'Tradição da Tinta', {
      body: {
        case: 'tableSubclass',
        value: create(TableSubclassSchema, {
          namePt: 'Tradição da Tinta',
          classKey: 'class:wizard',
          alwaysPrepared: [{ classLevel: 3, spellKey: 'spell:light' }],
          levels: [
            {
              level: 2,
              features: [
                { key: 'feature:x', namePt: 'Traço arcano', descPt: ['Texto.'], effects: [] },
              ],
            },
          ],
        }),
      },
    });

  function setup(e: ReturnType<typeof tinta> | null = tinta(), parentKey = '') {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: TableContentClient, useValue: { save } }],
    });
    const fixture = TestBed.createComponent(SubclassEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('catalog', cat);
    fixture.componentRef.setInput('menu', menu());
    fixture.componentRef.setInput('defaults', defaults);
    fixture.componentRef.setInput('entry', e);
    fixture.componentRef.setInput('parentKey', parentKey);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  async function settle(fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) {
    for (let i = 0; i < 3; i++) {
      fixture.detectChanges();
      await new Promise((r) => setTimeout(r));
      await fixture.whenStable();
    }
    fixture.detectChanges();
  }

  const text = (el: Element) => (el.textContent ?? '').replace(/\s+/g, ' ');
  const field = (el: HTMLElement, path: string) =>
    el.querySelector<HTMLElement>(`[data-field="${path}"]`)!;
  const click = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => text(b).includes(label))!
      .click();
  const cell = (el: HTMLElement, label: string) =>
    el.querySelector<HTMLInputElement>(`input[aria-label="${label}"]`)!;
  const pick = (select: HTMLSelectElement, label: string) => {
    select.selectedIndex = Array.from(select.options).findIndex((o) => o.text.trim() === label);
    select.dispatchEvent(new Event('change'));
  };

  it("says which class it is of and that the choice happens at the class's level (an SRD class here)", () => {
    const { el } = setup();
    expect(text(el)).toContain(
      'Subclasse de Mago, escolhida no nível 2 (o da classe). A classe não muda depois de criada.',
    );
    expect(el.querySelector('[data-field="table_subclass.class_key"]')).toBeNull();
    expect((field(el, 'table_subclass.name_pt') as HTMLInputElement).value).toBe(
      'Tradição da Tinta',
    );
    expect(text(el)).toContain('Nível 2');
    expect(text(el)).toContain('Traço arcano');
  });

  it('a new subclass picks its class from the catalog, and the table class is marked', () => {
    const { el } = setup(null, '');
    const select = field(el, 'table_subclass.class_key') as HTMLSelectElement;
    expect(Array.from(select.options).map((o) => o.text.trim())).toContain('Mago');
  });

  it('"Esta subclasse conjura" brings the third-caster rows from the level casting starts, from the server\'s table', async () => {
    const { fixture, el } = setup();
    expect(el.querySelector('app-level-grid')).toBeNull();
    (el.querySelector('[role="switch"]') as HTMLButtonElement).click();
    await settle(fixture);
    expect(cell(el, 'Nível 3, espaços de magia de 1º nível').value).toBe('2');
    expect(cell(el, 'Nível 3, truques').value).toBe('2');
    expect(cell(el, 'Nível 3, magias conhecidas').value).toBe('3');
    expect(cell(el, 'Nível 2, espaços de magia de 1º nível')).toBeNull();
    expect(el.querySelectorAll('tbody tr')).toHaveLength(18);
    // The subclass's own grid has no bonus column and no features column.
    expect(Array.from(el.querySelectorAll('th')).map((th) => text(th))).not.toContain('Bônus');
    pick(field(el, 'table_subclass.casting.ability') as HTMLSelectElement, 'Inteligência');
    pick(field(el, 'table_subclass.casting.list_from') as HTMLSelectElement, 'A lista do Mago');
    await settle(fixture);
    save.mockResolvedValue({ entry: tinta(), affected: [] });
    click(el, 'Salvar subclasse');
    await settle(fixture);
    const body = save.mock.calls[0][2];
    expect(body.case).toBe('tableSubclass');
    expect(body.value.casting).toMatchObject({
      kind: 'third',
      ability: Ability.INTELLIGENCE,
      listFrom: 'class:wizard',
      preparation: 'known',
      startLevel: 3,
    });
    expect(body.value.levels.map((l: { level: number }) => l.level)).toEqual(
      [2, ...Array.from({ length: 18 }, (_, i) => i + 3)].filter((v, i, a) => a.indexOf(v) === i),
    );
    expect(body.value.levels.find((l: { level: number }) => l.level === 3)).toMatchObject({
      cantripsKnown: 2,
      spellsKnown: 3,
      slots: [2, 0, 0, 0, 0, 0, 0, 0, 0],
    });
    expect(body.value.levels.find((l: { level: number }) => l.level === 2).features[0].key).toBe(
      'feature:x',
    );
  });

  it("asks before the third caster's table is replaced by another way of preparing, once it was edited", async () => {
    const { fixture, el } = setup();
    (el.querySelector('[role="switch"]') as HTMLButtonElement).click();
    await settle(fixture);
    const c = cell(el, 'Nível 4, espaços de magia de 1º nível');
    c.value = '1';
    c.dispatchEvent(new Event('input'));
    await settle(fixture);
    (el.querySelector('input[type="radio"][value="prepared"]') as HTMLInputElement).dispatchEvent(
      new Event('change'),
    );
    await settle(fixture);
    expect(text(el.querySelector('.ask')!)).toContain('Refazer a tabela?');
    click(el.querySelector('.ask') as HTMLElement, 'Refazer a tabela');
    await settle(fixture);
    expect(cell(el, 'Nível 4, espaços de magia de 1º nível').value).toBe('2');
    // Prepared: no "Magias" column.
    expect(Array.from(el.querySelectorAll('th')).map((th) => text(th))).not.toContain('Magias');
  });

  it('always-prepared spells: by class level, with the SRD and table spells, added and taken off', async () => {
    const { fixture, el } = setup();
    expect(text(el.querySelector('.groups')!)).toContain('Luz');
    pick(el.querySelector('.addlevel select') as HTMLSelectElement, 'Nível 1');
    await settle(fixture);
    expect(el.querySelectorAll('.group')).toHaveLength(2);
    const select = el.querySelector('.group[aria-label="Nível 1"] select') as HTMLSelectElement;
    pick(select, 'Luz');
    await settle(fixture);
    (
      el.querySelector(
        '.group[aria-label="Nível 3"] button[aria-label="Tirar Luz"]',
      ) as HTMLButtonElement
    ).click();
    await settle(fixture);
    save.mockResolvedValue({ entry: tinta(), affected: [] });
    click(el, 'Salvar subclasse');
    await settle(fixture);
    expect(save.mock.calls[0][2].value.alwaysPrepared).toEqual([
      { classLevel: 1, spellKey: 'spell:light' },
    ]);
  });

  it('lands the refusal of an always-prepared spell on the group, and a refused third-caster cell on its input', async () => {
    const { fixture, el } = setup();
    (el.querySelector('[role="switch"]') as HTMLButtonElement).click();
    await settle(fixture);
    save.mockRejectedValue(
      new ConnectError('refused', Code.InvalidArgument, undefined, [
        {
          desc: TableContentRefusalSchema,
          value: create(TableContentRefusalSchema, {
            violations: [
              create(TableContentViolationSchema, {
                field: 'table_subclass.always_prepared[0].spell_key',
                reason: 'bad_value',
              }),
              create(TableContentViolationSchema, {
                field: 'table_subclass.levels[2].slots[0]',
                reason: 'bad_table',
              }),
            ],
          }),
        },
      ]),
    );
    click(el, 'Salvar subclasse');
    await settle(fixture);
    expect(
      text(el.querySelector('[data-field="table_subclass.always_prepared"]')!.parentElement!),
    ).toContain('Uma magia sempre preparada é de 1º nível ou mais, nunca um truque.');
    // Levels 2 (a feature) and 3, 4...: level 4 is the third row, `levels[2]`.
    expect(cell(el, 'Nível 4, espaços de magia de 1º nível').getAttribute('aria-invalid')).toBe(
      'true',
    );
  });

  it("a stored third caster switches Preparadas and Conhecidas with no question: its rows are the server's, which carry no bonus", async () => {
    const stored = entry(TableContentKind.SUBCLASS, 'Tradição da Tinta', {
      body: {
        case: 'tableSubclass',
        value: create(TableSubclassSchema, {
          namePt: 'Tradição da Tinta',
          classKey: 'class:fighter',
          casting: {
            kind: 'third',
            ability: Ability.INTELLIGENCE,
            preparation: 'known',
            listFrom: 'class:wizard',
            startLevel: 3,
          },
          levels: defaults.tables[7].rows
            .filter((_r, i) => i >= 2)
            .map((r, i) => ({
              level: i + 3,
              cantripsKnown: r.cantripsKnown,
              spellsKnown: r.spellsKnown,
              slots: r.slots,
            })),
        }),
      },
    });
    const { fixture, el } = setup(stored);
    (el.querySelector('input[type="radio"][value="prepared"]') as HTMLInputElement).dispatchEvent(
      new Event('change'),
    );
    await settle(fixture);
    expect(el.querySelector('app-table-question')).toBeNull();
    // Prepared: no "Magias" column; the table is the prepared default (no spells known).
    expect(Array.from(el.querySelectorAll('th')).map((th) => text(th))).not.toContain('Magias');
    (el.querySelector('input[type="radio"][value="known"]') as HTMLInputElement).dispatchEvent(
      new Event('change'),
    );
    await settle(fixture);
    expect(el.querySelector('app-table-question')).toBeNull();
    expect(cell(el, 'Nível 3, magias conhecidas').value).toBe('3');
  });

  it('puts a refused always-prepared spell of the flat list on its own group, with its message under it', async () => {
    const stored = tinta();
    const { fixture, el } = setup(stored);
    pick(el.querySelector('.addlevel select') as HTMLSelectElement, 'Nível 1');
    await settle(fixture);
    pick(el.querySelector('.group[aria-label="Nível 1"] select') as HTMLSelectElement, 'Luz');
    await settle(fixture);
    // The request lists level 1's spell first (index 0), then level 3's (index 1).
    save.mockRejectedValue(
      new ConnectError('refused', Code.InvalidArgument, undefined, [
        {
          desc: TableContentRefusalSchema,
          value: create(TableContentRefusalSchema, {
            violations: [
              create(TableContentViolationSchema, {
                field: 'table_subclass.always_prepared[1].spell_key',
                reason: 'dangling_reference',
              }),
            ],
          }),
        },
      ]),
    );
    click(el, 'Salvar subclasse');
    await settle(fixture);
    const group3 = el.querySelector('[data-field="table_subclass.always_prepared#3"]')!;
    expect(text(group3)).toContain('Esta magia não existe mais. Escolha outra.');
    expect(
      text(el.querySelector('[data-field="table_subclass.always_prepared#1"]')!),
    ).not.toContain('Esta magia não existe');
    expect(group3.contains(document.activeElement)).toBe(true);
  });

  it('the 61st feature refusal goes to the panel head', async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(
      new ConnectError('refused', Code.InvalidArgument, undefined, [
        {
          desc: TableContentRefusalSchema,
          value: create(TableContentRefusalSchema, {
            violations: [
              create(TableContentViolationSchema, {
                field: 'table_subclass.levels[0].features[0]',
                reason: 'limit',
              }),
            ],
          }),
        },
      ]),
    );
    click(el, 'Salvar subclasse');
    await settle(fixture);
    expect(
      text(el.querySelector('[data-field="table_subclass.features"]')!.parentElement!),
    ).toContain(
      'Esta subclasse tem 1. Uma subclasse da mesa não pode ter mais de 60 características.',
    );
  });
});
