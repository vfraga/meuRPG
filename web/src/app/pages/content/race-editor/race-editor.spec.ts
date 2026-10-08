import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
  TableRaceSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { TableContentClient } from '../../../core/content/content-client';
import { catalog, entry, feature, menu } from '../../../core/content/content-testing';
import { RaceEditor } from './race-editor';

describe('RaceEditor', () => {
  const save = vi.fn();
  const cat = catalog();
  const corujeiro = () =>
    entry(TableContentKind.RACE, 'Corujeiro', {
      body: {
        case: 'tableRace',
        value: create(TableRaceSchema, {
          namePt: 'Corujeiro',
          size: 'Medium',
          speedFt: 30,
          darkvisionFt: 60,
          abilityBonuses: { wisdom: 2, dexterity: 1 },
          languages: ['language:common'],
          traits: [
            feature('Olhos de caçador', [{ type: 'proficiency', proficiency: 'skill:perception' }]),
            feature('Planar', [
              { type: 'modifier', target: 'speed.walk', mode: 'add', value: '5 +' },
            ]),
          ],
        }),
      },
    });

  function setup() {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: TableContentClient, useValue: { save } }],
    });
    const fixture = TestBed.createComponent(RaceEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('catalog', cat);
    fixture.componentRef.setInput('menu', menu());
    fixture.componentRef.setInput('entry', corujeiro());
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

  const text = (el: Element) => (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const field = (el: HTMLElement, path: string) =>
    el.querySelector<HTMLElement>(`[data-field="${path}"]`)!;
  const click = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => text(b).includes(label))!
      .click();

  it('draws the race of the artboard: the form from the entry and the traits with the effect picked', () => {
    const { el } = setup();
    expect((field(el, 'table_race.name_pt') as HTMLInputElement).value).toBe('Corujeiro');
    expect((field(el, 'table_race.speed_ft') as HTMLInputElement).value).toBe('9');
    expect((field(el, 'table_race.darkvision_ft') as HTMLInputElement).value).toBe('18');
    expect(
      (field(el, 'table_race.traits[0].effects[0].type') as HTMLSelectElement).selectedOptions[0]
        .text,
    ).toBe('Proficiência');
    expect(
      (field(el, 'table_race.traits[0].effects[0].proficiency') as HTMLSelectElement)
        .selectedOptions[0].text,
    ).toBe('Percepção');
    const rows = Array.from(el.querySelectorAll('.preview .rows__row')).map(
      (r) => `${r.querySelector('dt')!.textContent}: ${r.querySelector('dd')!.textContent}`,
    );
    expect(rows[0]).toBe('Habilidades: Destreza +1, Sabedoria +2');
    expect(rows).toContain('Idiomas: Comum');
    expect(el.querySelectorAll('app-feature-editor')).toHaveLength(2);
  });

  it("sends the race with only the fields of each trait's effect type, and the keys the editor read", async () => {
    const { fixture, el } = setup();
    save.mockResolvedValue({ entry: corujeiro(), affected: [] });
    click(el, 'Salvar raça');
    await settle(fixture);
    const [, sent, body] = save.mock.calls[0];
    expect(sent.key).toBe('race:corujeiro@mesa');
    expect(body.case).toBe('tableRace');
    expect(body.value.speedFt).toBe(30);
    expect(body.value.darkvisionFt).toBe(60);
    expect(body.value.abilityBonuses).toMatchObject({ wisdom: 2, dexterity: 1, strength: 0 });
    expect(body.value.traits[0]).toEqual({
      key: 'feature:Olhos de caçador',
      namePt: 'Olhos de caçador',
      descPt: ['Texto.'],
      effects: [{ type: 'proficiency', proficiency: 'skill:perception' }],
    });
    expect(body.value.traits[1].effects).toEqual([
      { type: 'modifier', target: 'speed.walk', mode: 'add', value: '5 +' },
    ]);
  });

  it('says a darkvision it cannot read and does not save the race with none', async () => {
    const { fixture, el } = setup();
    const input = field(el, 'table_race.darkvision_ft') as HTMLInputElement;
    input.value = '18 m';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(input.getAttribute('aria-invalid')).toBe('true');
    expect(text(input.closest('app-text-field')!)).toContain('Escreva só o número de metros');
    click(el, 'Salvar raça');
    await settle(fixture);
    expect(save).not.toHaveBeenCalled();
    input.value = '18';
    input.dispatchEvent(new Event('input'));
    save.mockResolvedValue({ entry: corujeiro(), affected: [] });
    click(el, 'Salvar raça');
    await settle(fixture);
    expect(save.mock.calls[0][2].value.darkvisionFt).toBe(60);
  });

  it('puts "table_race.traits[1].effects[0].value" back on the formula field of the second trait', async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(
      new ConnectError('refused', Code.InvalidArgument, undefined, [
        {
          desc: TableContentRefusalSchema,
          value: create(TableContentRefusalSchema, {
            violations: [
              create(TableContentViolationSchema, {
                field: 'table_race.traits[1].effects[0].value',
                reason: 'bad_formula',
              }),
            ],
          }),
        },
      ]),
    );
    click(el, 'Salvar raça');
    await settle(fixture);
    const input = field(el, 'table_race.traits[1].effects[0].value');
    expect(input.getAttribute('aria-invalid')).toBe('true');
    expect(text(input.closest('app-text-field')!)).toContain('Esta fórmula não funciona.');
    expect(document.activeElement).toBe(input);
    expect(
      field(el, 'table_race.traits[0].effects[0].proficiency').getAttribute('aria-invalid'),
    ).not.toBe('true');
  });

  it('adds a trait, takes one off and moves one; "Só texto" drops the effects', async () => {
    const { fixture, el } = setup();
    click(el, 'Adicionar traço');
    await settle(fixture);
    expect(el.querySelectorAll('app-feature-editor')).toHaveLength(3);
    const third = el.querySelectorAll('app-feature-editor')[2];
    (third.querySelector('input') as HTMLInputElement).value = 'Voo curto';
    third.querySelector('input')!.dispatchEvent(new Event('input'));
    // "Só texto" is the first option of the effect select of a feature with no effect.
    expect((third.querySelector('select') as HTMLSelectElement).selectedOptions[0].text).toBe(
      'Só texto',
    );
    click(third as HTMLElement, 'Remover traço');
    await settle(fixture);
    expect(el.querySelectorAll('app-feature-editor')).toHaveLength(2);
    // Move the second one up.
    (
      Array.from(el.querySelectorAll('app-feature-editor')[1].querySelectorAll('button')).find(
        (b) => b.getAttribute('aria-label')?.startsWith('Subir'),
      ) as HTMLButtonElement
    ).click();
    await settle(fixture);
    expect(
      (el.querySelector('[data-field="table_race.traits[0].name_pt"]') as HTMLInputElement).value,
    ).toBe('Planar');
    // Changing a trait's effect to "Só texto" drops what it had.
    const select = field(el, 'table_race.traits[1].effects[0].type') as HTMLSelectElement;
    select.selectedIndex = 0;
    select.dispatchEvent(new Event('change'));
    await settle(fixture);
    save.mockResolvedValue({ entry: corujeiro(), affected: [] });
    click(el, 'Salvar raça');
    await settle(fixture);
    expect(save.mock.calls[0][2].value.traits[1].effects).toEqual([]);
  });

  it("offers the six abilities with the catalog's names, from −4 to +4, and puts a bonus refusal under them with the focus on it", async () => {
    const { fixture, el } = setup();
    const names = Array.from(el.querySelectorAll('app-number-stepper .step__label')).map((n) =>
      n.textContent?.trim(),
    );
    expect(names).toEqual([
      'Força',
      'Destreza',
      'Constituição',
      'Inteligência',
      'Sabedoria',
      'Carisma',
    ]);
    const minus = el.querySelector<HTMLButtonElement>(
      'app-number-stepper button[aria-label="Menos Força"]',
    )!;
    expect(minus.disabled).toBe(false);
    save.mockRejectedValue(
      new ConnectError('refused', Code.InvalidArgument, undefined, [
        {
          desc: TableContentRefusalSchema,
          value: create(TableContentRefusalSchema, {
            violations: [
              create(TableContentViolationSchema, {
                field: 'table_race.ability_bonuses.wisdom',
                reason: 'limit',
              }),
            ],
          }),
        },
      ]),
    );
    click(el, 'Salvar raça');
    await settle(fixture);
    expect(text(el.querySelector('[role="alert"]')!)).toContain('1 campo precisa de ajuste');
    expect(text(el.querySelector('.bonuses')!.parentElement!)).toContain('O bônus vai de −4 a +4.');
    expect(
      document.activeElement?.closest('[data-field="table_race.ability_bonuses.wisdom"]') ??
        document.activeElement,
    ).toBeTruthy();
  });

  it("a new sub-race picks its race from the catalog (the SRD's and the table's); with a race given, it only says which", () => {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: TableContentClient, useValue: { save } }],
    });
    const f = TestBed.createComponent(RaceEditor);
    f.componentRef.setInput('campaignId', 'camp-1');
    f.componentRef.setInput('mode', 'subrace');
    f.componentRef.setInput('catalog', cat);
    f.componentRef.setInput('menu', menu());
    f.detectChanges();
    const e = f.nativeElement as HTMLElement;
    const select = e.querySelector<HTMLSelectElement>('[data-field="table_subrace.race_key"]')!;
    expect(Array.from(select.options).map((o) => o.text.trim())).toContain('Humano');
    expect(text(e)).toContain('Salvar sub-raça');
    f.componentRef.setInput('parentKey', 'race:human');
    f.detectChanges();
    expect(e.querySelector('[data-field="table_subrace.race_key"]')).toBeNull();
    expect(text(e)).toContain('Sub-raça de Humano');
  });

  it('keeps the race select of a new sub-race after a race is chosen in it', () => {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: TableContentClient, useValue: { save } }],
    });
    const f = TestBed.createComponent(RaceEditor);
    f.componentRef.setInput('campaignId', 'camp-1');
    f.componentRef.setInput('mode', 'subrace');
    f.componentRef.setInput('catalog', cat);
    f.componentRef.setInput('menu', menu());
    f.detectChanges();
    const e = f.nativeElement as HTMLElement;
    const select = e.querySelector<HTMLSelectElement>('[data-field="table_subrace.race_key"]')!;
    select.value = 'race:human';
    select.dispatchEvent(new Event('change'));
    f.detectChanges();
    expect(e.querySelector('[data-field="table_subrace.race_key"]')).not.toBeNull();
  });

  it('keeps a trait\'s "Mais opções" with the trait when it moves', async () => {
    const { fixture, el } = setup();
    const open = el
      .querySelectorAll('app-feature-editor')[0]
      .querySelector<HTMLButtonElement>('.more')!;
    open.click();
    fixture.detectChanges();
    expect(el.querySelectorAll('app-feature-editor')[0].textContent).toContain('Menos opções');
    (
      Array.from(el.querySelectorAll('app-feature-editor')[0].querySelectorAll('button')).find(
        (b) => b.getAttribute('aria-label')?.startsWith('Descer'),
      ) as HTMLButtonElement
    ).click();
    await settle(fixture);
    expect(el.querySelectorAll('app-feature-editor')[1].textContent).toContain('Menos opções');
    expect(el.querySelectorAll('app-feature-editor')[0].textContent).not.toContain('Menos opções');
  });
});
