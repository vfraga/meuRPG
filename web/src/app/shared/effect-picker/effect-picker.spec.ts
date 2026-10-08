import { ComponentFixture, TestBed } from '@angular/core/testing';

import { type EffectDraft, emptyEffect } from '../../core/content/effect-draft';
import { menu } from '../../core/content/content-testing';
import { EffectPicker } from './effect-picker';

describe('EffectPicker (the closed menu of ADR-0018, from the server)', () => {
  const emitted: EffectDraft[] = [];
  let fixture: ComponentFixture<EffectPicker>;
  let el: HTMLElement;

  function setup(
    effect: EffectDraft,
    over: { issues?: Record<string, string[]>; allowTextOnly?: boolean } = {},
  ) {
    emitted.length = 0;
    TestBed.resetTestingModule();
    fixture = TestBed.createComponent(EffectPicker);
    fixture.componentRef.setInput('effect', effect);
    fixture.componentRef.setInput('menu', menu());
    fixture.componentRef.setInput('basePath', 'table_race.traits[0].effects[0]');
    fixture.componentRef.setInput('allowTextOnly', over.allowTextOnly ?? false);
    fixture.componentRef.setInput('issuesOf', (path: string) => over.issues?.[path] ?? []);
    fixture.componentInstance.effectChange.subscribe((e) => emitted.push(e));
    fixture.detectChanges();
    el = fixture.nativeElement as HTMLElement;
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const field = (path: string) =>
    el.querySelector<HTMLElement>(`[data-field="table_race.traits[0].effects[0].${path}"]`);
  const change = (select: HTMLSelectElement, label: string) => {
    select.selectedIndex = Array.from(select.options).findIndex((o) => o.text.trim() === label);
    select.dispatchEvent(new Event('change'));
  };

  it('lists the menu\'s types with "Só texto" first only where a feature may have none, and says what the type is for', () => {
    setup(emptyEffect('proficiency'), { allowTextOnly: true });
    const type = field('type') as HTMLSelectElement;
    expect(Array.from(type.options).map((o) => o.text.trim())).toEqual([
      'Só texto',
      'Modificador',
      'Proficiência',
      'Sentido',
      'Escolha',
      'Nota e magia concedida',
    ]);
    expect(text(el)).toContain('Dá proficiência.');
    setup(emptyEffect('proficiency'));
    expect(
      Array.from((field('type') as HTMLSelectElement).options).map((o) => o.text.trim()),
    ).not.toContain('Só texto');
  });

  it('draws the fields the menu says the type reads: the required one open, the optional ones folded under "Mais opções"', async () => {
    setup(emptyEffect('proficiency'));
    expect(field('proficiency')).not.toBeNull();
    expect(field('level')).toBeNull();
    expect(field('when')).toBeNull();
    expect(text(el)).toContain('Mais opções');
    (
      Array.from(el.querySelectorAll('button')).find((b) =>
        text(b).includes('Mais opções'),
      ) as HTMLButtonElement
    ).click();
    fixture.detectChanges();
    expect(field('level')).not.toBeNull();
    expect(field('when')).not.toBeNull();
    expect(text(el)).toContain('Menos opções');
  });

  it('opens an optional field that holds a value, and one a refusal points at, without being asked', () => {
    setup({ ...emptyEffect('proficiency'), proficiency: 'skill:perception', when: 'level() >= 5' });
    expect(field('when')).not.toBeNull();
    expect(field('level')).toBeNull();
    setup(emptyEffect('proficiency'), {
      issues: {
        'table_race.traits[0].effects[0].level': ['Este valor não serve para este efeito.'],
      },
    });
    expect(field('level')).not.toBeNull();
    expect(text(el)).toContain('Este valor não serve para este efeito.');
    expect(field('level')!.getAttribute('aria-invalid')).toBe('true');
  });

  it('takes the formula functions and the tag prefixes from the menu, not from the browser', () => {
    setup({ ...emptyEffect('modifier'), target: 'speed.walk', mode: 'add', tags: '' });
    expect(text(el)).toContain('Funções: mod("<habilidade>"), prof().');
    (
      Array.from(el.querySelectorAll('button')).find((b) =>
        text(b).includes('Mais opções'),
      ) as HTMLButtonElement
    ).click();
    fixture.detectChanges();
    expect(text(el)).toContain('começando por against:… ou about:….');
  });

  it('starts a new effect when the type changes: nothing of the old type is kept', () => {
    setup({ ...emptyEffect('proficiency'), proficiency: 'skill:perception' });
    change(field('type') as HTMLSelectElement, 'Sentido');
    expect(emitted).toHaveLength(1);
    expect(emitted[0].type).toBe('sense');
    expect(emitted[0].proficiency).toBe('');
  });

  it('offers the choices of a closed list from the menu, and a "choice" effect\'s `from` by its kind, which a new kind empties', () => {
    setup({ ...emptyEffect('choice'), choice: 'skill', count: 2, from: ['skill:arcana'] });
    const pick = el.querySelector('[data-field="table_race.traits[0].effects[0].from"]')!;
    expect(text(pick)).toContain('Arcanismo');
    const add = pick.querySelector('select') as HTMLSelectElement;
    expect(Array.from(add.options).map((o) => o.text.trim())).toEqual(['', 'Percepção']);
    add.selectedIndex = 1;
    add.dispatchEvent(new Event('change'));
    expect(emitted.at(-1)?.from).toEqual(['skill:arcana', 'skill:perception']);
    (pick.querySelector('.chip__x') as HTMLButtonElement).click();
    expect(emitted.at(-1)?.from).toEqual([]);
    change(field('choice') as HTMLSelectElement, 'Uma opção de uma lista do SRD');
    expect(emitted.at(-1)).toMatchObject({ choice: 'feature', from: [] });
  });

  it("asks a sense's range in metres and keeps feet underneath", () => {
    setup({ ...emptyEffect('sense'), sense: 'darkvision', rangeM: '18' });
    const range = field('range_ft') as HTMLInputElement;
    expect(range.value).toBe('18');
    range.value = '9';
    range.dispatchEvent(new Event('input'));
    expect(emitted.at(-1)?.rangeM).toBe('9');
  });

  it('puts the hint beside its field, and the refusal under it', () => {
    setup({ ...emptyEffect('modifier'), target: 'speed.walk', mode: 'add' });
    expect(el.querySelector('.frow__hint')).not.toBeNull();
    setup(
      { ...emptyEffect('modifier'), target: 'speed.walk', mode: 'add' },
      { issues: { 'table_race.traits[0].effects[0].value': ['Esta fórmula não funciona.'] } },
    );
    const row = field('value')!.closest('.frow')!;
    expect(text(row)).toContain('Esta fórmula não funciona.');
    expect(row.querySelector('.frow__hint')).toBeNull();
  });
});

describe('EffectPicker: the range field (metres on screen, feet on the wire)', () => {
  let fixture: ComponentFixture<EffectPicker>;
  let input: HTMLInputElement;

  function setup() {
    TestBed.resetTestingModule();
    fixture = TestBed.createComponent(EffectPicker);
    fixture.componentRef.setInput('effect', {
      ...emptyEffect('sense'),
      sense: 'darkvision',
      rangeM: '18',
    });
    fixture.componentRef.setInput('menu', menu());
    fixture.componentRef.setInput('basePath', 'table_race.traits[0].effects[0]');
    fixture.componentRef.setInput('allowTextOnly', false);
    fixture.componentRef.setInput('issuesOf', () => []);
    // The parent owns the draft: it takes what is emitted and passes it back, as the editor does.
    fixture.componentInstance.effectChange.subscribe((e: EffectDraft) => {
      fixture.componentRef.setInput('effect', e);
    });
    fixture.detectChanges();
    input = (fixture.nativeElement as HTMLElement).querySelector<HTMLInputElement>(
      '[data-field="table_race.traits[0].effects[0].range_ft"]',
    )!;
  }

  async function type(text: string) {
    input.value = text;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  it('keeps "4" as typed, though it is not a whole number of feet', async () => {
    setup();
    await type('4');
    expect(input.value).toBe('4');
  });

  it('keeps "4,5" as typed, character by character', async () => {
    setup();
    await type('4');
    expect(input.value).toBe('4');
    await type('4,');
    expect(input.value).toBe('4,');
    await type('4,5');
    expect(input.value).toBe('4,5');
  });
});
