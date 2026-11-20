import { ComponentFixture, TestBed } from '@angular/core/testing';

import { emptyEffect } from '../../core/content/effect-draft';
import { emptyFeature, type FeatureDraft, withAsks } from '../../core/content/feature-draft';
import { catalog } from '../../core/content/content-testing';
import { menu } from '../../core/content/content-testing-menu';
import { costsMenu } from '../../core/content/content-testing-menu-costs';
import { TableSummonSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { summonToDraft } from '../../core/content/summon-draft';
import { create } from '@bufbuild/protobuf';
import type { CatalogVm } from '../../core/content/catalog';
import { rollsMenu } from '../../core/content/content-testing-menu-rolls';
import { FeatureEditor } from './feature-editor';

describe('FeatureEditor (a trait, a feature)', () => {
  const emitted: FeatureDraft[] = [];
  const removed = vi.fn();
  let fixture: ComponentFixture<FeatureEditor>;
  let el: HTMLElement;

  function two(): FeatureDraft {
    return {
      ...emptyFeature(),
      name: 'Olhos de caçador',
      text: 'Enxergam longe.',
      effects: [
        { ...emptyEffect('proficiency'), proficiency: 'skill:perception' },
        { ...emptyEffect('sense'), sense: 'darkvision', rangeM: '18' },
      ],
    };
  }

  function setup(
    feature: FeatureDraft,
    over: {
      costs?: boolean;
      issues?: Record<string, string[]>;
      removeLabel?: string;
      index?: number;
      count?: number;
      isOption?: boolean;
      allowSummon?: boolean;
      menu?: ReturnType<typeof menu>;
      oneOf?: boolean;
      allowAsks?: boolean;
      allowOptions?: boolean;
      catalog?: CatalogVm | null;
    } = {},
  ) {
    emitted.length = 0;
    removed.mockReset();
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        {
          provide: CreaturesClient,
          useValue: {
            search: vi.fn(),
            statBlock: vi.fn().mockResolvedValue({ summary: { namePt: 'Lobo' } }),
          },
        },
      ],
    });
    fixture = TestBed.createComponent(FeatureEditor);
    fixture.componentRef.setInput('feature', feature);
    fixture.componentRef.setInput('menu', over.menu ?? (over.costs ? costsMenu() : menu()));
    fixture.componentRef.setInput('basePath', 'table_race.traits[1]');
    fixture.componentRef.setInput('issuesOf', (path: string) => over.issues?.[path] ?? []);
    fixture.componentRef.setInput('removeLabel', over.removeLabel ?? 'Remover traço');
    fixture.componentRef.setInput('index', over.index ?? 1);
    fixture.componentRef.setInput('count', over.count ?? 3);
    fixture.componentRef.setInput('isOption', over.isOption ?? false);
    fixture.componentRef.setInput('allowSummon', over.allowSummon ?? false);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('allowOptions', over.allowOptions ?? over.oneOf ?? false);
    fixture.componentRef.setInput('oneOf', over.oneOf ?? false);
    fixture.componentRef.setInput('allowAsks', over.allowAsks ?? false);
    fixture.componentRef.setInput('catalog', over.catalog ?? null);
    fixture.componentInstance.featureChange.subscribe((f) => emitted.push(f));
    fixture.componentInstance.removed.subscribe(removed);
    fixture.detectChanges();
    el = fixture.nativeElement as HTMLElement;
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const button = (label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      text(b).includes(label),
    )!;
  const field = (path: string) =>
    el.querySelector<HTMLElement>(`[data-field="table_race.traits[1].${path}"]`);

  it('puts the name and the effect on one row and the text in one box, with the group named apart from the name field', () => {
    setup(two());
    const grid = el.querySelector('.grid')!;
    expect(grid.querySelector('[data-field="table_race.traits[1].name_pt"]')).not.toBeNull();
    expect(
      grid.querySelector('[data-field="table_race.traits[1].effects[0].type"]'),
    ).not.toBeNull();
    expect(el.querySelectorAll('textarea')).toHaveLength(1);
    expect(el.querySelector('[role="group"]')!.getAttribute('aria-label')).toBe('Traço 2');
  });

  it('chooses "Só texto" on one effect without wiping the others', () => {
    setup(two());
    const type = field('effects[0].type') as HTMLSelectElement;
    type.selectedIndex = 0;
    type.dispatchEvent(new Event('change'));
    expect(emitted).toHaveLength(1);
    expect(emitted[0].effects.map((e) => e.type)).toEqual(['sense']);
    expect(emitted[0].name).toBe('Olhos de caçador');
  });

  it('adds an effect up to the menu\'s limit and takes the others off by a plain "Remover o efeito N"', () => {
    setup(two());
    button('Adicionar um efeito').click();
    expect(emitted.at(-1)?.effects.map((e) => e.type)).toEqual(['proficiency', 'sense', 'note']);
    setup(two());
    button('Remover o efeito 2').click();
    expect(emitted.at(-1)?.effects.map((e) => e.type)).toEqual(['proficiency']);
    const full = two();
    full.effects.push(emptyEffect('note'), emptyEffect('note'));
    setup(full);
    expect(el.textContent).not.toContain('Adicionar um efeito');
  });

  it('draws "Remover traço" in ink and "Adicionar um efeito" as a link, and removes the trait', () => {
    setup(two());
    expect(button('Remover traço').classList).toContain('remove');
    expect(button('Adicionar um efeito').classList).toContain('link');
    button('Remover traço').click();
    expect(removed).toHaveBeenCalled();
  });

  it('shows the refusals of the trait, of an effect and of a field where they belong', () => {
    setup(two(), {
      issues: {
        'table_race.traits[1]': ['Esta característica tem coisa demais.'],
        'table_race.traits[1].effects[1]': ['Este efeito não serve.'],
        'table_race.traits[1].name_pt': ['O nome tem de 1 a 60 letras.'],
      },
    });
    const t = text(el);
    expect(t).toContain('Esta característica tem coisa demais.');
    expect(t).toContain('Este efeito não serve.');
    expect(t).toContain('O nome tem de 1 a 60 letras.');
  });

  it("tracks the effects by a stable id: moving the list keeps each row's own state", () => {
    const f = two();
    setup(f);
    const sense = f.effects[1];
    const rows = Array.from(el.querySelectorAll('.effect'));
    expect(rows).toHaveLength(1);
    const swapped = { ...f, effects: [f.effects[1], f.effects[0]] };
    fixture.componentRef.setInput('feature', swapped);
    fixture.detectChanges();
    expect(sense.id).not.toBe(f.effects[0].id);
    expect((field('effects[0].type') as HTMLSelectElement).selectedOptions[0].text).toBe('Sentido');
    expect((field('effects[1].type') as HTMLSelectElement).selectedOptions[0].text).toBe(
      'Proficiência',
    );
  });

  it('says an option with content and no name is not saved, and nothing for a named one, an empty one or a trait', () => {
    const say = 'Sem nome, esta opção não é salva.';
    const withText = { ...two(), name: '', text: 'Escolha uma.' };
    setup(withText, { isOption: true });
    expect(el.textContent).toContain(say);
    setup({ ...withText, name: 'Primeira' }, { isOption: true });
    expect(el.textContent).not.toContain(say);
    setup({ ...withText, text: '', effects: [] }, { isOption: true });
    expect(el.textContent).not.toContain(say);
    setup(withText);
    expect(el.textContent).not.toContain(say);
  });

  it('moves the trait up or down and offers no move for the only one', () => {
    setup(two());
    const moved = vi.fn();
    fixture.componentInstance.moved.subscribe(moved);
    (
      Array.from(el.querySelectorAll('button')).find((b) =>
        b.getAttribute('aria-label')?.startsWith('Subir'),
      ) as HTMLButtonElement
    ).click();
    expect(moved).toHaveBeenCalledWith(-1);
    setup(two(), { count: 1, index: 0 });
    expect(
      Array.from(el.querySelectorAll('button')).some((b) =>
        b.getAttribute('aria-label')?.startsWith('Subir'),
      ),
    ).toBe(false);
  });

  describe('"Invoca criaturas" (allowSummon)', () => {
    const resourceFeature = (): FeatureDraft => ({
      ...emptyFeature(),
      name: 'Companheiro do Vale',
      effects: [{ ...emptyEffect('resource'), resource: 'companheiro', max: '1' }],
    });

    it('is drawn for a class, subclass, race or background feature and not for an option or a feat', () => {
      setup(resourceFeature(), { allowSummon: true });
      expect(el.querySelector('app-summon-block')).not.toBeNull();
      setup(resourceFeature());
      expect(el.querySelector('app-summon-block')).toBeNull();
    });

    it('turning it on sends the feature with a summon that spends its resource; off sends none', () => {
      setup(resourceFeature(), { allowSummon: true });
      el.querySelector<HTMLButtonElement>('app-summon-block button[role="switch"]')!.click();
      expect(emitted[0].summon).toMatchObject({ resourceKey: 'companheiro' });
      setup({ ...resourceFeature(), summon: emitted[0].summon }, { allowSummon: true });
      el.querySelector<HTMLButtonElement>('app-summon-block button[role="switch"]')!.click();
      expect(emitted[0].summon).toBeNull();
    });

    it('loads the toggle from the feature and follows the name of its one resource', () => {
      const stored = create(TableSummonSchema, {
        creatureKeys: ['monster:wolf'],
        count: '1',
        resourceKey: 'companheiro',
      });
      setup(
        { ...resourceFeature(), summon: summonToDraft(stored) },
        { allowSummon: true, costs: true },
      );
      expect(
        el.querySelector('app-summon-block button[role="switch"]')!.getAttribute('aria-checked'),
      ).toBe('true');
      const name = el.querySelector<HTMLInputElement>(
        '[data-field="table_race.traits[1].effects[0].resource"]',
      );
      expect(name).not.toBeNull();
      name!.value = 'vale';
      name!.dispatchEvent(new Event('input'));
      expect(emitted[0].summon?.resourceKey).toBe('vale');
    });

    it("does not rewrite a stored resource the feature never defined (the class's)", () => {
      const stored = create(TableSummonSchema, {
        creatureKeys: ['monster:wolf'],
        count: '1',
        resourceKey: 'da_classe',
      });
      setup(
        { ...resourceFeature(), summon: summonToDraft(stored) },
        { allowSummon: true, costs: true },
      );
      const name = el.querySelector<HTMLInputElement>(
        '[data-field="table_race.traits[1].effects[0].resource"]',
      )!;
      name.value = 'vale';
      name.dispatchEvent(new Event('input'));
      expect(emitted[0].summon?.resourceKey).toBe('da_classe');
    });
  });

  it('offers a reroll the resources of its own entry, wherever the resource effect sits in the list', () => {
    setup(
      {
        ...emptyFeature(),
        name: 'Sorte Teimosa',
        effects: [
          { ...emptyEffect('reroll'), rolls: ['attack'], atMost: 2 },
          {
            ...emptyEffect('resource'),
            resource: 'sorte_teimosa',
            max: '3',
            recharge: 'long_rest',
          },
        ],
      },
      { menu: rollsMenu() },
    );
    const select = field('effects[0].resource') as HTMLSelectElement;
    expect(Array.from(select.options).map((o) => text(o))).toEqual([
      'Nenhum: vale sempre',
      'Sorte Teimosa (3 por descanso longo)',
    ]);
    expect(select.disabled).toBe(false);
  });

  describe('a trait that is a choice of one (T1, T6)', () => {
    const night = (): FeatureDraft => ({
      ...emptyFeature(),
      name: 'Sentidos da noite',
      options: [
        { ...emptyFeature(), name: 'Olhos de coruja' },
        { ...emptyFeature(), name: 'Passo de pena' },
      ],
    });

    it('says there is no option yet and offers to add one', () => {
      setup(emptyFeature(), { oneOf: true });
      expect(text(el.querySelector('.options')!)).toContain(
        'Nenhuma opção. Adicione duas ou mais: o jogador escolhe uma, uma vez, ao criar o personagem.',
      );
      button('Adicionar uma opção').click();
      expect(emitted.at(-1)?.options).toHaveLength(1);
      expect(el.textContent).not.toContain('Os efeitos ficam nas opções');
    });

    it('explains the one-time choice and says under the effect that the effects live in the options', () => {
      setup(night(), { oneOf: true });
      const t = text(el.querySelector('.options')!);
      expect(t).toContain('O jogador escolhe uma destas opções, uma vez, ao criar o personagem');
      expect(t).not.toContain('Ponha na característica um efeito');
      expect(text(el)).toContain('Os efeitos ficam nas opções; o traço diz que há uma escolha.');
      expect(el.querySelectorAll('.options .opt')).toHaveLength(2);
    });

    it('puts the refusal of an effect "Escolha" under the select, in place of the hint', () => {
      setup(night(), {
        oneOf: true,
        issues: {
          'table_race.traits[1].effects[0].type': [
            'Um traço com opções já é a escolha: troque este efeito por “Só texto” ou outro.',
          ],
        },
      });
      expect(text(el)).toContain('Um traço com opções já é a escolha');
      expect(text(el)).not.toContain('Os efeitos ficam nas opções');
    });

    it('marks the option whose name repeats another, and limits an option name to 60 letters', () => {
      setup(night(), {
        oneOf: true,
        issues: {
          'table_race.traits[1].options[1].name_pt': [
            'Duas opções deste traço têm o mesmo nome. Mude uma delas.',
          ],
        },
      });
      const second = el.querySelector<HTMLInputElement>(
        '[data-field="table_race.traits[1].options[1].name_pt"]',
      )!;
      expect(second.getAttribute('maxlength')).toBe('60');
      expect(text(second.closest('app-text-field')!)).toContain('Duas opções deste traço');
    });

    it('keeps the class feature wording where the trait is not a choice of one', () => {
      setup(night());
      expect(el.querySelector('.options')).toBeNull();
      fixture.componentRef.setInput('allowOptions', true);
      fixture.detectChanges();
      expect(text(el.querySelector('.options')!)).toContain(
        'Quantas ele escolhe é o efeito de escolha da característica que diz.',
      );
    });
  });

  describe('what an option row asks (board 580-03)', () => {
    it('draws "O que a opção pede" only for an option row whose feature may ask', () => {
      setup({ ...emptyFeature(), name: 'Golpe' }, { isOption: true });
      expect(text(el)).not.toContain('O que a opção pede');
      setup({ ...emptyFeature(), name: 'Golpe' }, { isOption: false, allowAsks: true });
      expect(text(el)).not.toContain('O que a opção pede');
      setup({ ...emptyFeature(), name: 'Golpe' }, { isOption: true, allowAsks: true });
      expect(text(el)).toContain('O que a opção pede');
      expect(button('Adicionar pré-requisito')).toBeDefined();
    });

    it('heads "O que a opção pede" with "O jogador precisa de todos" at its right, as the board has it (580-03)', () => {
      setup({ ...emptyFeature(), name: 'Golpe' }, { isOption: true, allowAsks: true });
      const head = el.querySelector('.asks .asks__head')!;
      expect(head.querySelector('h4')?.textContent).toContain('O que a opção pede');
      expect(text(head.querySelector('.asks__all')!)).toBe('O jogador precisa de todos');
      expect(text(el.querySelector('.asks')!)).not.toContain('Vale só para quem vai escolher');
    });

    it('offers "Adicionar pré-requisito" as a text action with a caret, not as an outlined button (board 580-03)', () => {
      setup({ ...emptyFeature(), name: 'Golpe' }, { isOption: true, allowAsks: true });
      const add = button('Adicionar pré-requisito');
      expect(add.classList).toContain('asks-add__link');
      expect(add.classList).not.toContain('mat-mdc-outlined-button');
      expect(text(add)).toContain('arrow_drop_down');
    });

    it('says in the hint of "Outras opções" that it is never the option itself (board 580-03)', () => {
      const asked = withAsks(
        { ...emptyFeature(), name: 'Golpe' },
        { requiresOptions: ['option:x'] },
      );
      setup(asked, { isOption: true, allowAsks: true });
      expect(text(el)).toContain(
        'Opções que o personagem já escolheu, do SRD ou da mesa. Nunca a própria opção.',
      );
      expect(text(el)).not.toContain('1 de 10.');
      // The count stands at the end of the chip row, apart from the sentence.
      expect(text(el.querySelector('.pick__count')!).trim()).toBe('1 de 10');
    });

    it('draws the chip a cycle refusal is about with the danger border, an icon and a hidden "com problema" (580-03)', () => {
      const peer = (key: string, asks: string): FeatureDraft =>
        withAsks({ ...emptyFeature(), key, name: key }, { requiresOptions: [asks] });
      const own = withAsks(
        { ...emptyFeature(), key: 'feature:b@mesa', name: 'Asa Partida' },
        { requiresOptions: ['feature:a@mesa', 'feature:z@mesa'] },
      );
      setup(own, {
        isOption: true,
        allowAsks: true,
        issues: {
          'table_race.traits[1].prerequisite.requires_option_keys': [
            'Duas opções não podem pedir uma à outra.',
          ],
        },
      });
      fixture.componentRef.setInput('peers', [
        peer('feature:a@mesa', 'feature:b@mesa'),
        { ...emptyFeature(), key: 'feature:z@mesa', name: 'Zê' },
      ]);
      fixture.detectChanges();
      const chips = Array.from(el.querySelectorAll('app-pick-list .chip'));
      expect(chips[0].classList).toContain('chip--bad');
      expect(chips[0].querySelector('mat-icon.chip__bad')).not.toBeNull();
      expect(text(chips[0])).toContain(', com problema');
      expect(chips[1].classList).not.toContain('chip--bad');
      // The sentence stands under the chips and above the select.
      const pick = el.querySelector('app-pick-list')!;
      const sentence = pick.querySelector('app-field-note')!;
      expect(text(sentence)).toContain('Duas opções não podem pedir uma à outra.');
      expect(
        sentence.compareDocumentPosition(pick.querySelector('.pick--add')!) &
          Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    });

    it('flags no chip while the row has no refusal', () => {
      const own = withAsks(
        { ...emptyFeature(), key: 'feature:b@mesa', name: 'Asa Partida' },
        { requiresOptions: ['feature:a@mesa'] },
      );
      setup(own, { isOption: true, allowAsks: true });
      expect(el.querySelector('app-pick-list .chip--bad')).toBeNull();
    });

    it('keeps the outlined "Remover" for a trait that is not an option row', () => {
      setup(two(), { removeLabel: 'Remover traço' });
      expect(button('Remover traço').classList).toContain('remove');
      expect(el.querySelector('button[aria-label^="Mais ações de"]')).toBeNull();
    });

    it('shows the row the option already asks, and clearing it sends the option with no prerequisite', () => {
      const asked = withAsks(
        { ...emptyFeature(), name: 'Golpe' },
        { classKey: 'class:fighter', classLevel: '7' },
      );
      setup(asked, { isOption: true, allowAsks: true });
      expect(text(el)).toContain('Nível de classe');
      el.querySelector<HTMLButtonElement>('button[aria-label="Tirar Nível de classe"]')!.click();
      expect(emitted.at(-1)?.prerequisite).toBeUndefined();
    });
  });

  describe('the siblings of an option in "Outras opções" (#580, board 580-03)', () => {
    const sibling = (key: string, name: string): FeatureDraft => ({ ...emptyFeature(), key, name });
    const parent = (): FeatureDraft => ({
      ...emptyFeature(),
      name: 'Truques de lâmina',
      options: [
        sibling('feature:a@mesa', 'Golpe do Corvo'),
        withAsks(sibling('feature:b@mesa', 'Asa Partida'), {
          classKey: 'class:fighter',
          classLevel: '7',
        }),
        sibling('', 'Bico de Ferro'),
      ],
    });
    const open = (n: number) => {
      const row = el.querySelector<HTMLElement>(`[role="group"][aria-label="Opção ${n}"]`)!;
      const toggle = row.querySelector<HTMLButtonElement>('button[aria-expanded="false"]');
      toggle?.click();
      fixture.detectChanges();
      return row;
    };
    const addAsk = (row: HTMLElement) => {
      Array.from(row.querySelectorAll<HTMLButtonElement>('.asks-add__menu button'))
        .find((b) => text(b).includes('Adicionar pré-requisito'))!
        .click();
      fixture.detectChanges();
      Array.from(row.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
        .find((b) => text(b).includes('Outras opções'))!
        .click();
      fixture.detectChanges();
    };
    // The add control is a button that opens the system's menu panel: each choice is a menu item with its origin as a tag.
    const choices = (row: HTMLElement) => {
      row.querySelector<HTMLButtonElement>('app-pick-list .pick__addbtn')!.click();
      fixture.detectChanges();
      return Array.from(document.querySelectorAll('.pick-menu .mat-mdc-menu-item')).map((o) =>
        text(o).trim(),
      );
    };

    it('offers the saved siblings with their "Opção N", never the option itself or one not saved', () => {
      setup(parent(), { allowOptions: true });
      const row = open(2);
      addAsk(row);
      const all = choices(row);
      // Every choice carries its origin: a sibling its place, an option of the table "Da mesa", an SRD one "SRD".
      expect(all).toContain('Golpe do CorvoOpção 1');
      expect(all.every((o) => /(Opção \d|Da mesa|SRD)$/.test(o))).toBe(true);
      expect(all.some((o) => o.startsWith('Asa Partida'))).toBe(false);
      expect(all.some((o) => o.startsWith('Bico de Ferro'))).toBe(false);
    });

    it('shows a picked sibling as a chip with the badge of its place', () => {
      const f = parent();
      f.options[1] = withAsks(f.options[1], { requiresOptions: ['feature:a@mesa'] });
      setup(f, { allowOptions: true });
      const chip = open(2).querySelector('app-pick-list .chip')!;
      expect(text(chip)).toContain('Golpe do Corvo');
      expect(chip.querySelector('.mr-tag')?.textContent?.trim()).toBe('Opção 1');
    });

    it('says in the hint that an option not saved yet can be asked for after the feature is saved, and keeps "Nunca a própria opção."', () => {
      setup(parent(), { allowOptions: true });
      const row = open(2);
      addAsk(row);
      expect(text(row)).toContain(
        'Uma opção nova só pode ser pedida depois de salvar a característica.',
      );
      expect(text(row)).toContain('Nunca a própria opção.');
    });

    it('leaves the sentence out when every option is saved', () => {
      const f = parent();
      f.options[2] = { ...f.options[2], key: 'feature:c@mesa' };
      setup(f, { allowOptions: true });
      const row = open(2);
      addAsk(row);
      expect(text(row)).toContain('Nunca a própria opção.');
      expect(text(row)).not.toContain('depois de salvar a característica');
    });
  });

  describe('the arrows of an option row (#580, board 580-03)', () => {
    const named = (key: string, name: string): FeatureDraft => ({ ...emptyFeature(), key, name });
    const three = (): FeatureDraft => ({
      ...emptyFeature(),
      name: 'Truques de lâmina',
      options: [named('k1', 'Golpe do Corvo'), named('k2', 'Asa Partida'), named('', '')],
    });
    const render = async () => {
      fixture.detectChanges();
      await fixture.whenStable();
    };
    // The label says which option: "Subir a opção Golpe do Corvo", or "Subir a opção 3" for one with no name.
    const arrow = (verb: 'Subir' | 'Descer', name: string) =>
      el.querySelector<HTMLButtonElement>(
        `button[aria-label="${verb} a opção ${name.replace(/^Opção /, '')}"]`,
      )!;

    it('draws up and down on each row, disabled at the ends, named by the option (or by its place when it has no name)', () => {
      setup(three(), { allowOptions: true, index: -1, count: 0 });
      expect(arrow('Subir', 'Golpe do Corvo').disabled).toBe(true);
      expect(arrow('Descer', 'Golpe do Corvo').disabled).toBe(false);
      expect(arrow('Subir', 'Asa Partida').disabled).toBe(false);
      expect(arrow('Descer', 'Asa Partida').disabled).toBe(false);
      expect(arrow('Subir', 'Opção 3').disabled).toBe(false);
      expect(arrow('Descer', 'Opção 3').disabled).toBe(true);
      expect(arrow('Subir', 'Golpe do Corvo').querySelector('mat-icon')?.textContent).toBe(
        'arrow_upward',
      );
    });

    it('draws the arrows and the "⋮" as the system\'s icon buttons, each saying which option it moves', () => {
      setup(three(), { allowOptions: true, index: -1, count: 0 });
      for (const label of [
        'Subir a opção Asa Partida',
        'Descer a opção Asa Partida',
        'Mais ações da opção Asa Partida',
        'Mais ações da opção 3',
      ]) {
        const b = el.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
        expect(b, label).not.toBeNull();
        expect(b!.classList, label).toContain('mat-mdc-icon-button');
      }
    });

    it('keeps "Precisa de ajuste" on the option that was refused when it moves (the refusals are by place, the row is the option)', async () => {
      const refused = {
        'table_race.traits[1].options[1].prerequisite.requires_option_keys': ['Sem motivo.'],
      };
      setup(three(), { allowOptions: true, index: -1, count: 0, issues: refused });
      await render();
      const sub = (n: number) =>
        text(el.querySelectorAll('.opt')[n - 1].querySelector('.opt__text')!);
      expect(sub(2)).toContain('Precisa de ajuste');
      expect(sub(1)).not.toContain('Precisa de ajuste');
      arrow('Subir', 'Asa Partida').click();
      fixture.componentRef.setInput('feature', emitted.at(-1));
      await render();
      expect(text(el.querySelectorAll('.opt')[0].querySelector('.opt__title')!)).toContain(
        'Asa Partida',
      );
      expect(sub(1)).toContain('Precisa de ajuste');
      expect(sub(2)).not.toContain('Precisa de ajuste');
      // The open row is the refused one, and the field inside it still carries the refusal.
      expect(el.querySelectorAll('.opt')[0].classList).toContain('opt--open');
      arrow('Descer', 'Asa Partida').click();
      fixture.componentRef.setInput('feature', emitted.at(-1));
      await render();
      expect(sub(2)).toContain('Precisa de ajuste');
      expect(sub(1)).not.toContain('Precisa de ajuste');
    });

    it('puts the refusals of a new save at the places it names, so the field it focuses is where the option now is', async () => {
      const path = 'table_race.traits[1].options[1].prerequisite.requires_option_keys';
      setup(three(), {
        allowOptions: true,
        index: -1,
        count: 0,
        issues: { [path]: ['Sem motivo.'] },
      });
      await render();
      arrow('Subir', 'Asa Partida').click();
      fixture.componentRef.setInput('feature', emitted.at(-1));
      await render();
      // A new save names its refusals by the places the options have now: the option at place 2 (Golpe do Corvo, after the move).
      fixture.componentRef.setInput('issuesOf', (p: string) =>
        p === path ? ['Outra recusa.'] : [],
      );
      await render();
      const sub = (n: number) =>
        text(el.querySelectorAll('.opt')[n - 1].querySelector('.opt__text')!);
      expect(sub(2)).toContain('Precisa de ajuste');
      expect(sub(1)).not.toContain('Precisa de ajuste');
      expect(el.querySelectorAll('.opt')[1].classList).toContain('opt--open');
      expect(
        el
          .querySelectorAll('.opt')[1]
          .querySelector(`[data-field^="${path.replace(/\.[^.]+$/, '')}"]`),
      ).not.toBeNull();
    });

    it('draws no arrows for a feature with one option', () => {
      setup(
        { ...three(), options: [named('k1', 'Golpe')] },
        { allowOptions: true, index: -1, count: 0 },
      );
      expect(el.querySelector('button[aria-label^="Subir"]')).toBeNull();
    });

    it('moves an option down or up and keeps the keys with their options, so a pick keeps its option', () => {
      setup(three(), { allowOptions: true, index: -1, count: 0 });
      arrow('Descer', 'Golpe do Corvo').click();
      expect(emitted.at(-1)?.options.map((o) => [o.key, o.name])).toEqual([
        ['k2', 'Asa Partida'],
        ['k1', 'Golpe do Corvo'],
        ['', ''],
      ]);
      setup(three(), { allowOptions: true, index: -1, count: 0 });
      arrow('Subir', 'Asa Partida').click();
      expect(emitted.at(-1)?.options.map((o) => o.key)).toEqual(['k2', 'k1', '']);
    });

    it('keeps the focus on the arrow that was pressed, in the row at its new place', async () => {
      setup(three(), { allowOptions: true, index: -1, count: 0 });
      arrow('Descer', 'Golpe do Corvo').focus();
      arrow('Descer', 'Golpe do Corvo').click();
      fixture.componentRef.setInput('feature', emitted.at(-1));
      fixture.detectChanges();
      await fixture.whenStable();
      expect(document.activeElement).toBe(arrow('Descer', 'Golpe do Corvo'));
      // At the last place "Descer" is disabled: the focus goes to "Subir" of the same row, not to the page.
      arrow('Descer', 'Golpe do Corvo').click();
      fixture.componentRef.setInput('feature', emitted.at(-1));
      fixture.detectChanges();
      await fixture.whenStable();
      expect(document.activeElement).toBe(arrow('Subir', 'Golpe do Corvo'));
    });
  });

  describe('the option rows are collapsed, one open at a time (#580, board 580-03)', () => {
    const opt = (key: string, name: string, text = ''): FeatureDraft => ({
      ...emptyFeature(),
      key,
      name,
      text,
    });
    const list = (): FeatureDraft => ({
      ...emptyFeature(),
      name: 'Truques de lâmina',
      options: [
        opt('k1', 'Golpe do Corvo', 'Soma o dado.'),
        withAsks(opt('k2', 'Asa Partida', 'Soma o dado ao ataque.'), {
          classKey: 'class:paladin',
          classLevel: '7',
          requiresOptions: ['k1'],
        }),
        withAsks(opt('k3', 'Bico de Ferro'), { classKey: 'class:paladin', classLevel: '10' }),
      ],
    });
    const rows = () =>
      Array.from(el.querySelectorAll<HTMLElement>('section.options > [role="group"]'));
    const toggle = (n: number) =>
      rows()[n - 1].querySelector<HTMLButtonElement>('button[aria-expanded]')!;
    const render = async () => {
      fixture.detectChanges();
      await fixture.whenStable();
    };

    it('draws each option as a closed row: a button with its place and name, and a summary line of what it asks', () => {
      setup(list(), {
        allowOptions: true,
        allowAsks: true,
        index: -1,
        count: 0,
        catalog: catalog(),
      });
      // The feature's own flag: only an option of a list that may ask says "não pede nada".
      expect(rows().length).toBe(3);
      expect(toggle(1).getAttribute('aria-expanded')).toBe('false');
      expect(text(toggle(1))).toContain('Opção 1 · Golpe do Corvo');
      expect(
        el.querySelector('input[aria-label="Nome da opção"], [data-field$="options[0].name_pt"]'),
      ).toBeNull();
      expect(text(toggle(1))).toContain('Soma o dado · não pede nada');
      expect(text(toggle(2))).toContain('Soma o dado ao ataque · pede Paladino 7 e Golpe do Corvo');
      expect(text(toggle(3))).toContain('Pede Paladino 10');
    });

    it('says nothing of what it asks for an option of a feature that cannot ask', () => {
      setup(list(), { allowOptions: true, allowAsks: false, oneOf: true, index: -1, count: 0 });
      expect(text(toggle(1))).not.toContain('pede');
    });

    it('opens one row at a time, keeps the focus on its button and links it to its body', async () => {
      setup(list(), { allowOptions: true, allowAsks: true, index: -1, count: 0 });
      toggle(2).focus();
      toggle(2).click();
      await render();
      expect(toggle(2).getAttribute('aria-expanded')).toBe('true');
      expect(rows()[1].classList).toContain('opt--open');
      const body = el.querySelector<HTMLElement>(`#${toggle(2).getAttribute('aria-controls')}`);
      expect(body).not.toBeNull();
      expect(rows()[1].contains(body)).toBe(true);
      expect(rows()[1].querySelector('input')).not.toBeNull();
      expect(document.activeElement).toBe(toggle(2));
      toggle(3).click();
      await render();
      expect(toggle(2).getAttribute('aria-expanded')).toBe('false');
      expect(toggle(3).getAttribute('aria-expanded')).toBe('true');
      expect(rows()[1].querySelector('input')).toBeNull();
      toggle(3).focus();
      toggle(3).click();
      await render();
      expect(toggle(3).getAttribute('aria-expanded')).toBe('false');
      expect(document.activeElement).toBe(toggle(3));
    });

    it('names a group "Opção N" once: the open body is not a second group', async () => {
      setup(list(), { allowOptions: true, allowAsks: true, index: -1, count: 0 });
      toggle(1).click();
      await render();
      expect(el.querySelectorAll('[role="group"][aria-label="Opção 1"]').length).toBe(1);
      expect(rows()[0].querySelector('[role="group"][aria-label="Característica"]')).toBeNull();
    });

    it('keeps the open option open when it moves, and "Adicionar uma opção" opens the new one with its name focused', async () => {
      setup(list(), { allowOptions: true, allowAsks: true, index: -1, count: 0 });
      toggle(2).click();
      await render();
      el.querySelector<HTMLButtonElement>(
        'button[aria-label="Subir a opção Asa Partida"]',
      )!.click();
      fixture.componentRef.setInput('feature', emitted.at(-1));
      await render();
      expect(toggle(1).getAttribute('aria-expanded')).toBe('true');
      expect(text(toggle(1))).toContain('Opção 1 · Asa Partida');
      button('Adicionar uma opção').click();
      fixture.componentRef.setInput('feature', emitted.at(-1));
      await render();
      expect(rows().length).toBe(4);
      expect(toggle(4).getAttribute('aria-expanded')).toBe('true');
      expect(toggle(1).getAttribute('aria-expanded')).toBe('false');
      expect(rows()[3].contains(document.activeElement)).toBe(true);
    });

    it('opens the row that has a refusal and marks it in its summary', async () => {
      setup(list(), {
        allowOptions: true,
        allowAsks: true,
        index: -1,
        count: 0,
        issues: {
          'table_race.traits[1].options[2].prerequisite.class_level.level': [
            'O nível é de 1 a 20.',
          ],
        },
      });
      await render();
      expect(toggle(3).getAttribute('aria-expanded')).toBe('true');
      expect(text(toggle(1))).not.toContain('Precisa de ajuste');
      expect(text(toggle(3))).toContain('Precisa de ajuste');
    });

    it('puts "Precisa de ajuste" after the name on the title line, outside the summary, with no dot tied to it (580-03)', async () => {
      const nameless = (): FeatureDraft => ({
        ...emptyFeature(),
        name: 'Truques de lâmina',
        options: [opt('k1', 'Golpe do Corvo', 'Soma o dado.'), opt('k2', 'Asa Partida')],
      });
      setup(nameless(), {
        allowOptions: true,
        allowAsks: false,
        oneOf: true,
        index: -1,
        count: 0,
        issues: {
          'table_race.traits[1].options[0].name_pt': ['Falta o nome.'],
          'table_race.traits[1].options[1].name_pt': ['Falta o nome.'],
        },
      });
      await render();
      for (const n of [1, 2]) {
        const row = rows()[n - 1];
        const line = row.querySelector('.opt__line')!;
        // The tag is the second item of the title line, after the name, and the summary line is another element.
        expect(line.children[0].classList).toContain('opt__title');
        expect(line.children[1].classList).toContain('opt__bad');
        expect(text(line.children[1])).toContain('Precisa de ajuste');
        expect(row.querySelector('.opt__sub')!.textContent).not.toContain('Precisa de ajuste');
        expect(row.querySelector('.opt__dot')).toBeNull();
        expect(getComputedStyle(line.children[1]).whiteSpace).toBe('nowrap');
        expect(row.classList).toContain('opt--bad');
      }
      expect(text(rows()[0].querySelector('.opt__sub')!).trim()).toBe('Soma o dado');
      expect(text(rows()[1].querySelector('.opt__sub')!).trim()).toBe('');
    });

    it('takes an option away from the "⋮" menu of its row and puts the focus on the next row', async () => {
      setup(list(), { allowOptions: true, allowAsks: true, index: -1, count: 0 });
      rows()[0]
        .querySelector<HTMLButtonElement>(
          'button[aria-label="Mais ações da opção Golpe do Corvo"]',
        )!
        .click();
      await render();
      document.querySelector<HTMLButtonElement>('.mat-mdc-menu-panel [role="menuitem"]')!.click();
      expect(emitted.at(-1)?.options.map((o) => o.key)).toEqual(['k2', 'k3']);
      fixture.componentRef.setInput('feature', emitted.at(-1));
      await render();
      expect(document.activeElement).toBe(toggle(1));
    });

    it('has no "Remover opção" button in the open body: the menu of the row holds it', async () => {
      setup(list(), { allowOptions: true, allowAsks: true, index: -1, count: 0 });
      toggle(2).click();
      await render();
      expect(
        Array.from(el.querySelectorAll('button')).some((b) => text(b).includes('Remover opção')),
      ).toBe(false);
    });
  });
});
