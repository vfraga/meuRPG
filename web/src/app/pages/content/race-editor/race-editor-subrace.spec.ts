import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
  TableRaceSchema,
  TableSubraceSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { TableContentClient } from '../../../core/content/content-client';
import { catalog, entry, feature } from '../../../core/content/content-testing';
import { menu } from '../../../core/content/content-testing-menu';
import { RaceEditor } from './race-editor';
import { settle } from '../../../../test-settle';

describe('RaceEditor, a subrace that replaces traits of its race (T2, T6)', () => {
  const save = vi.fn();
  const RACE = 'race:corujeiro@mesa';
  const olhos = feature('Olhos de caçador', [
    { type: 'proficiency', proficiency: 'skill:perception' },
  ]);
  const parent = () =>
    entry(TableContentKind.RACE, 'Corujeiro', {
      key: RACE,
      body: {
        case: 'tableRace',
        value: create(TableRaceSchema, {
          namePt: 'Corujeiro',
          traits: [
            olhos,
            feature('Planar'),
            {
              ...feature('Sentidos da noite'),
              options: [feature('Olhos de coruja'), feature('Passo de pena')],
            },
          ],
        }),
      },
    });
  const sub = (init: object = {}) =>
    entry(TableContentKind.SUBRACE, 'Corujeiro das Neves', {
      body: {
        case: 'tableSubrace',
        value: create(TableSubraceSchema, {
          namePt: 'Corujeiro das Neves',
          raceKey: RACE,
          abilityBonuses: { constitution: 2, wisdom: 1 },
          traits: [feature('Pele de geada', [{ type: 'note', textPt: 'Resiste ao frio.' }])],
          replacesTraitKeys: [olhos.key],
          replacesAbilityBonuses: true,
          ...init,
        }),
      },
    });

  function setup(opts: { entry?: ReturnType<typeof sub> | null; parentKey?: string } = {}) {
    save.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: TableContentClient, useValue: { save } }],
    });
    const entries = [parent(), ...(opts.entry ? [opts.entry] : [])];
    const fixture = TestBed.createComponent(RaceEditor);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('mode', 'subrace');
    fixture.componentRef.setInput('catalog', catalog(entries));
    fixture.componentRef.setInput('menu', menu());
    fixture.componentRef.setInput('entries', entries);
    if (opts.entry !== null) {
      fixture.componentRef.setInput('entry', opts.entry ?? sub());
    }
    if (opts.parentKey) {
      fixture.componentRef.setInput('parentKey', opts.parentKey);
    }
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const text = (el: Element) => (el.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ');
  const rows = (el: HTMLElement) => Array.from(el.querySelectorAll('app-replaced-traits li'));
  const box = (row: Element) => row.querySelector<HTMLInputElement>('input[type="checkbox"]')!;
  const click = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => text(b).includes(label))!
      .click();

  it('lists the traits of the race with one line each, the replaced one marked "Substituído"', () => {
    const { el } = setup();
    const panel = el.querySelector('app-replaced-traits')!;
    expect(text(panel.querySelector('h2')!)).toBe('Traços de Corujeiro');
    expect(rows(el).map((r) => text(r.querySelector('.label')!))).toEqual([
      'Olhos de caçador',
      'Planar',
      'Sentidos da noite',
    ]);
    expect(rows(el).map((r) => text(r.querySelector('.sub')!))).toEqual([
      'Proficiência em Percepção',
      'Só texto',
      'Escolha entre 2 opções',
    ]);
    expect(rows(el).map((r) => text(r).includes('Substituído'))).toEqual([true, false, false]);
    expect(rows(el).map((r) => box(r).checked)).toEqual([true, false, false]);
  });

  it('marks a trait and unmarks another, and saves the keys it was left with', async () => {
    const { fixture, el } = setup();
    box(rows(el)[1]).click();
    box(rows(el)[0]).click();
    fixture.detectChanges();
    expect(rows(el).map((r) => text(r).includes('Substituído'))).toEqual([false, true, false]);
    save.mockResolvedValue({ entry: sub(), affected: [] });
    click(el, 'Salvar sub-raça');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.replacesTraitKeys).toEqual(['feature:Planar']);
  });

  it('sends the replaced traits and the switch of the bonuses back as they were read', async () => {
    const { fixture, el } = setup();
    save.mockResolvedValue({ entry: sub(), affected: [] });
    click(el, 'Salvar sub-raça');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value).toMatchObject({
      replacesTraitKeys: [olhos.key],
      replacesAbilityBonuses: true,
    });
  });

  it('turns "Substituiu os bônus de Corujeiro" on and off, under the bonuses', async () => {
    const { fixture, el } = setup({ entry: sub({ replacesAbilityBonuses: false }) });
    const sw = el.querySelector<HTMLButtonElement>(
      '[data-field="table_subrace.replaces_ability_bonuses"]',
    )!;
    expect(sw.getAttribute('aria-checked')).toBe('false');
    expect(text(sw.closest('app-switch-field')!)).toContain('Substituiu os bônus de Corujeiro');
    sw.click();
    fixture.detectChanges();
    expect(sw.getAttribute('aria-checked')).toBe('true');
    save.mockResolvedValue({ entry: sub(), affected: [] });
    click(el, 'Salvar sub-raça');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.replacesAbilityBonuses).toBe(true);
  });

  it('is locked, with the switch off, while the race of a new subrace is not chosen', () => {
    const { fixture, el } = setup({ entry: null });
    const panel = el.querySelector('app-replaced-traits')!;
    expect(text(panel)).toContain('Traços da raça');
    expect(text(panel)).toContain('Escolha a raça da sub-raça primeiro');
    expect(rows(el)).toHaveLength(0);
    const sw = el.querySelector<HTMLButtonElement>(
      '[data-field="table_subrace.replaces_ability_bonuses"]',
    )!;
    expect(sw.disabled).toBe(true);
    expect(text(sw.closest('app-switch-field')!)).toContain('Substitui os bônus da raça');
    expect(text(el.querySelector(`#${sw.getAttribute('aria-describedby')}`)!)).toContain(
      'Escolha a raça da sub-raça para poder ligar.',
    );
    const select = el.querySelector<HTMLSelectElement>('[data-field="table_subrace.race_key"]')!;
    select.value = 'race:human';
    select.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    expect(sw.disabled).toBe(false);
    expect(text(sw.closest('app-switch-field')!)).toContain('Substituiu os bônus de Humano');
    expect(rows(el)).toHaveLength(0);
  });

  it("opens a new subrace from its race with the race's traits to mark", () => {
    const { el } = setup({ entry: null, parentKey: RACE });
    expect(rows(el)).toHaveLength(3);
    expect(rows(el).some((r) => box(r).checked)).toBe(false);
  });

  it("lists an SRD race's traits from the menu, name and summary, with the marked one checked", async () => {
    const srd = sub({ raceKey: 'race:elf', replacesTraitKeys: ['trait:keen-senses'] });
    const { fixture, el } = setup({ entry: srd });
    expect(rows(el)).toHaveLength(2);
    expect(text(rows(el)[0])).toContain('Sentidos Aguçados');
    expect(text(rows(el)[0])).toContain('Você tem proficiência na perícia Percepção.');
    expect(box(rows(el)[0]).checked).toBe(true);
    expect(box(rows(el)[1]).checked).toBe(false);
    expect(text(rows(el)[1])).not.toContain('Sortudo');
    box(rows(el)[1]).click();
    fixture.detectChanges();
    save.mockResolvedValue({ entry: srd, affected: [] });
    click(el, 'Salvar sub-raça');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.replacesTraitKeys).toEqual([
      'trait:keen-senses',
      'trait:trance',
    ]);
  });

  it('marks a key that is no trait of an SRD parent as stale, as for a table race', () => {
    const srd = sub({ raceKey: 'race:elf', replacesTraitKeys: ['trait:lucky'] });
    const { el } = setup({ entry: srd });
    expect(rows(el)).toHaveLength(3);
    expect(text(rows(el).at(-1)!)).toContain('Um traço que não é de');
    expect(box(rows(el).at(-1)!).checked).toBe(true);
  });

  it('shows a marked key that is no trait of the race as a red row, and saving drops it once unmarked', async () => {
    const { fixture, el } = setup({
      entry: sub({ replacesTraitKeys: [olhos.key, 'trait:sumiu@mesa'] }),
    });
    const stale = rows(el).at(-1)!;
    expect(text(stale)).toContain('Um traço que não é de Corujeiro');
    expect(text(stale)).toContain('Este traço não é de Corujeiro. Desmarque para salvar.');
    expect(box(stale).checked).toBe(true);
    expect(box(stale).getAttribute('aria-invalid')).toBe('true');
    const issue = stale.querySelector('.inline-issue')!;
    expect(box(stale).getAttribute('aria-describedby')).toBe(issue.id);
    expect(box(rows(el)[0]).hasAttribute('aria-invalid')).toBe(false);
    expect(stale.querySelector('app-check-row .row--invalid')).not.toBeNull();
    expect(text(stale)).not.toContain('Substituído');
    box(stale).click();
    fixture.detectChanges();
    expect(rows(el)).toHaveLength(3);
    save.mockResolvedValue({ entry: sub(), affected: [] });
    click(el, 'Salvar sub-raça');
    await settle(fixture, 3);
    expect(save.mock.calls[0][2].value.replacesTraitKeys).toEqual([olhos.key]);
  });

  it('writes the preview as the players read it, and the master-only note beside it', () => {
    const { el } = setup();
    const side = el.querySelector('.editor__side')!;
    const heads = Array.from(side.querySelectorAll('h2')).map((h) => text(h));
    expect(heads).toContain('Traços de Corujeiro');
    expect(heads).toContain('Traços da sub-raça');
    const preview = text(side.querySelector('.preview')!);
    expect(preview).toContain('Planar.');
    expect(preview).not.toContain('Olhos de caçador');
    expect(preview).toContain('Olhos de coruja.');
    expect(preview).toContain('Pele de geada.');
    const note = side.querySelector('.master-note')!;
    expect(text(note)).toContain('Só você vê');
    expect(text(note.querySelector('dt')!)).toBe('Substitui');
    expect(text(note.querySelector('dd')!)).toBe('Olhos de caçador e os bônus de Corujeiro');
    expect(note.querySelector('mat-icon')?.textContent).toBe('visibility_off');
  });

  it('draws no master note when nothing is replaced', () => {
    const { el } = setup({
      entry: sub({ replacesTraitKeys: [], replacesAbilityBonuses: false }),
    });
    expect(el.querySelector('.master-note')).toBeNull();
  });

  it('puts the refusal of a "Escolha" effect on a trait with options under its effect select', async () => {
    const { fixture, el } = setup();
    save.mockRejectedValue(
      new ConnectError('refused', Code.InvalidArgument, undefined, [
        {
          desc: TableContentRefusalSchema,
          value: create(TableContentRefusalSchema, {
            violations: [
              create(TableContentViolationSchema, {
                field: 'table_subrace.traits[0].effects[0].choice',
                reason: 'bad_value',
              }),
              create(TableContentViolationSchema, {
                field: 'table_subrace.replaces_trait_keys[0]',
                reason: 'dangling_reference',
              }),
            ],
          }),
        },
      ]),
    );
    click(el, 'Salvar sub-raça');
    await settle(fixture, 3);
    const select = el.querySelector<HTMLElement>(
      '[data-field="table_subrace.traits[0].effects[0].type"]',
    )!;
    expect(text(select.closest('app-select-field')!)).toContain(
      'Um traço com opções já é a escolha: troque este efeito por “Só texto” ou outro.',
    );
    expect(text(el.querySelector('[role="alert"]')!)).toContain('2 campos precisam de ajuste');
  });

  it('outlines and focuses the stale row a refusal of its own index names', async () => {
    const { fixture, el } = setup({
      entry: sub({ replacesTraitKeys: [olhos.key, 'trait:sumiu@mesa'] }),
    });
    save.mockRejectedValue(
      new ConnectError('refused', Code.InvalidArgument, undefined, [
        {
          desc: TableContentRefusalSchema,
          value: create(TableContentRefusalSchema, {
            violations: [
              create(TableContentViolationSchema, {
                field: 'table_subrace.replaces_trait_keys[1]',
                reason: 'dangling_reference',
              }),
            ],
          }),
        },
      ]),
    );
    click(el, 'Salvar sub-raça');
    await settle(fixture, 3);
    const stale = rows(el).at(-1)!;
    expect(box(stale).getAttribute('data-field')).toBe('table_subrace.replaces_trait_keys[1]');
    expect(document.activeElement).toBe(box(stale));
    expect(stale.querySelector('.row--invalid')).not.toBeNull();
    expect(text(el.querySelector('[role="alert"]')!)).toContain('1 campo precisa de ajuste');
  });
});
