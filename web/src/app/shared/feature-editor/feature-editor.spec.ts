import { ComponentFixture, TestBed } from '@angular/core/testing';

import { emptyEffect } from '../../core/content/effect-draft';
import { emptyFeature, type FeatureDraft } from '../../core/content/feature-draft';
import { menu } from '../../core/content/content-testing-menu';
import { costsMenu } from '../../core/content/content-testing-menu-costs';
import { TableSummonSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { summonToDraft } from '../../core/content/summon-draft';
import { create } from '@bufbuild/protobuf';
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
    fixture.componentRef.setInput('menu', over.costs ? costsMenu() : menu());
    fixture.componentRef.setInput('basePath', 'table_race.traits[1]');
    fixture.componentRef.setInput('issuesOf', (path: string) => over.issues?.[path] ?? []);
    fixture.componentRef.setInput('removeLabel', over.removeLabel ?? 'Remover traço');
    fixture.componentRef.setInput('index', over.index ?? 1);
    fixture.componentRef.setInput('count', over.count ?? 3);
    fixture.componentRef.setInput('isOption', over.isOption ?? false);
    fixture.componentRef.setInput('allowSummon', over.allowSummon ?? false);
    fixture.componentRef.setInput('campaignId', 'camp-1');
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
});
