import { ComponentFixture, TestBed } from '@angular/core/testing';

import { emptyEffect } from '../../core/content/effect-draft';
import { emptyFeature, type FeatureDraft } from '../../core/content/feature-draft';
import { menu } from '../../core/content/content-testing';
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
      issues?: Record<string, string[]>;
      removeLabel?: string;
      index?: number;
      count?: number;
    } = {},
  ) {
    emitted.length = 0;
    removed.mockReset();
    TestBed.resetTestingModule();
    fixture = TestBed.createComponent(FeatureEditor);
    fixture.componentRef.setInput('feature', feature);
    fixture.componentRef.setInput('menu', menu());
    fixture.componentRef.setInput('basePath', 'table_race.traits[1]');
    fixture.componentRef.setInput('issuesOf', (path: string) => over.issues?.[path] ?? []);
    fixture.componentRef.setInput('removeLabel', over.removeLabel ?? 'Remover traço');
    fixture.componentRef.setInput('index', over.index ?? 1);
    fixture.componentRef.setInput('count', over.count ?? 3);
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
});
