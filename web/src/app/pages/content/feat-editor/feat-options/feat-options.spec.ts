import { TestBed } from '@angular/core/testing';

import { MAX_FEAT_OPTIONS } from '../../../../core/content/feat-draft';
import { emptyEffect } from '../../../../core/content/effect-draft';
import { menu } from '../../../../core/content/content-testing-menu';
import { type FeatureDraft, emptyFeature } from '../../../../core/content/feature-draft';
import { FeatOptions } from './feat-options';

const named = (name: string, text = ''): FeatureDraft => ({ ...emptyFeature(), name, text });
/** The words of an element, without the names of its icons. */
const text = (e: Element) => {
  const copy = e.cloneNode(true) as Element;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return (copy.textContent ?? '').replace(/\s+/g, ' ').trim();
};

function setup(options: FeatureDraft[] = [], extra: Record<string, unknown> = {}) {
  TestBed.resetTestingModule();
  const fixture = TestBed.createComponent(FeatOptions);
  const el = fixture.nativeElement as HTMLElement;
  const emitted: FeatureDraft[][] = [];
  const give = (value: FeatureDraft[]) => fixture.componentRef.setInput('options', value);
  fixture.componentRef.setInput('menu', menu());
  fixture.componentRef.setInput('choiceNumber', 1);
  Object.entries(extra).forEach(([k, v]) => fixture.componentRef.setInput(k, v));
  give(options);
  // The panel is controlled: what it emits comes back as its input, as the feat editor does.
  fixture.componentInstance.optionsChange.subscribe((next) => {
    emitted.push(next);
    give(next);
    fixture.detectChanges();
  });
  fixture.detectChanges();
  const button = (label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      (b.getAttribute('aria-label') ?? text(b)).includes(label),
    );
  return { fixture, el, emitted, button };
}

describe('FeatOptions: the "Opções do talento" panel', () => {
  it('says there is no option yet, with the count and an add button', () => {
    const { el, button } = setup();
    expect(text(el.querySelector('.count')!)).toBe('0 de 60 opções');
    expect(text(el)).toContain('Nenhuma opção ainda. Escreva aqui as opções só deste talento.');
    expect(el.querySelector('.rows')).toBeNull();
    expect(button('Adicionar uma opção')!.getAttribute('aria-disabled')).not.toBe('true');
  });

  it('draws a folded row per option with its place, name and what it does, and the intro with the number of the effect', () => {
    const guard = {
      ...named('Guarda Alta'),
      effects: [{ ...emptyEffect('modifier'), target: 'ac', mode: 'add', value: '1' }],
    };
    const { el } = setup([guard, named('Mão Firme')], { choiceNumber: 2 });
    expect(text(el.querySelector('.count')!)).toBe('2 de 60 opções');
    expect(text(el)).toContain(
      'O jogador fica com a que escolher no efeito 2; os efeitos dela valem enquanto ele tiver o talento.',
    );
    const rows = el.querySelectorAll('.row');
    expect(text(rows[0].querySelector('.row__title')!)).toBe('Opção 1 · Guarda Alta');
    expect(text(rows[0].querySelector('.row__sub')!)).toBe('Modificador: Classe de Armadura +1');
    expect(text(rows[1].querySelector('.row__toggle')!)).toBe('Opção 2 · Mão Firme');
    expect(rows[0].querySelector('.row__toggle')!.getAttribute('aria-expanded')).toBe('false');
    expect(rows[0].querySelector('.row__body')!.hasAttribute('hidden')).toBe(true);
  });

  it('opens a row with its fields, never offering a named choice inside it', () => {
    const { el, button } = setup([named('Guarda Alta', 'Classe de Armadura +1')]);
    button('Opção 1')!.click();
    TestBed.tick();
    const body = el.querySelector('.row__body')!;
    expect(body.hasAttribute('hidden')).toBe(false);
    expect(
      (el.querySelector('[data-field="table_feat.options[0].name_pt"]') as HTMLInputElement).value,
    ).toBe('Guarda Alta');
    const types = Array.from(
      el.querySelectorAll<HTMLOptionElement>(
        '[data-field="table_feat.options[0].effects[0].type"] option',
      ),
    ).map((o) => o.textContent!.trim());
    expect(types).not.toContain('Aumento de habilidade');
  });

  it('adds an empty option, open, at the end', () => {
    const { el, emitted, button } = setup([named('Guarda Alta')]);
    button('Adicionar uma opção')!.click();
    expect(emitted).toHaveLength(1);
    expect(emitted[0].map((o) => o.name)).toEqual(['Guarda Alta', '']);
    expect(el.querySelectorAll('.row')).toHaveLength(2);
    expect(el.querySelectorAll('.row--open')).toHaveLength(1);
    expect(text(el.querySelector('.count')!)).toBe('2 de 60 opções');
  });

  it('moves an option up and down with the arrows, and the end ones cannot go further', () => {
    const { el, emitted, button } = setup([named('A'), named('B'), named('C')]);
    expect(button('Subir Opção 1')!.disabled).toBe(true);
    expect(button('Descer Opção 3')!.disabled).toBe(true);
    button('Descer Opção 1')!.click();
    expect(emitted.at(-1)!.map((o) => o.name)).toEqual(['B', 'A', 'C']);
    button('Subir Opção 3')!.click();
    expect(emitted.at(-1)!.map((o) => o.name)).toEqual(['B', 'C', 'A']);
    expect(text(el.querySelectorAll('.row__title')[0])).toBe('Opção 1 · B');
  });

  it('has no arrows for a single option', () => {
    const { button } = setup([named('A')]);
    expect(button('Subir')).toBeUndefined();
    expect(button('Mais ações de Opção 1')).toBeDefined();
  });

  it('moves and removes an option from its menu', () => {
    const { emitted, button } = setup([named('A'), named('B'), named('C')]);
    const item = (label: string) =>
      Array.from(document.querySelectorAll<HTMLButtonElement>('.mat-mdc-menu-item')).find((b) =>
        text(b).includes(label),
      )!;
    button('Mais ações de Opção 2')!.click();
    TestBed.tick();
    expect(item('Subir Opção 2')).toBeDefined();
    expect(item('Descer Opção 2')).toBeDefined();
    item('Remover Opção 2').click();
    expect(emitted.at(-1)!.map((o) => o.name)).toEqual(['A', 'C']);
  });

  it('holds at 60: the add button waits, still focusable, and says why', () => {
    const sixty = Array.from({ length: MAX_FEAT_OPTIONS }, (_, i) => named(`Opção ${i}`));
    const { el, emitted, button } = setup(sixty);
    expect(text(el.querySelector('.count')!)).toBe('60 de 60 opções');
    const add = button('Adicionar uma opção')!;
    expect(add.getAttribute('aria-disabled')).toBe('true');
    expect(add.classList.contains('mr-button--off')).toBe(true);
    expect(text(el.querySelector('#feat-options-lock')!)).toBe(
      'No máximo 60 opções. Remova uma para pôr outra.',
    );
    expect(add.getAttribute('aria-describedby')).toBe('feat-options-lock');
    add.click();
    expect(emitted).toHaveLength(0);
  });

  it('takes a new option again once one is removed from a full list', () => {
    const sixty = Array.from({ length: MAX_FEAT_OPTIONS }, (_, i) => named(`Opção ${i}`));
    const { el, button } = setup(sixty);
    button('Mais ações de Opção 60')!.click();
    TestBed.tick();
    Array.from(document.querySelectorAll<HTMLButtonElement>('.mat-mdc-menu-item'))
      .find((b) => text(b).includes('Remover'))!
      .click();
    expect(text(el.querySelector('.count')!)).toBe('59 de 60 opções');
    expect(el.querySelector('#feat-options-lock')).toBeNull();
  });

  it('shows a count over the limit as a plain number', () => {
    const over = Array.from({ length: MAX_FEAT_OPTIONS + 1 }, (_, i) => named(`Opção ${i}`));
    const { el } = setup(over);
    expect(text(el.querySelector('.count')!)).toBe('61 opções');
    expect(el.querySelector('.count--over')).not.toBeNull();
  });

  it('takes no option of its own while the choice offers a set, and says so', () => {
    const { el, emitted, button } = setup([], { setName: 'Estilo de luta' });
    expect(text(el)).toContain(
      'Este talento oferece o conjunto Estilo de luta. Para escrever opções só dele, escolha “As opções do talento” no efeito 1.',
    );
    expect(text(el.querySelector('#feat-options-lock')!)).toBe(
      'A escolha oferece um conjunto: um talento oferece as opções dele ou um conjunto, não os dois.',
    );
    button('Adicionar uma opção')!.click();
    expect(emitted).toHaveLength(0);
  });

  it('asks for the choice when the feat has none to offer the options', () => {
    const { el } = setup([named('A')], { choiceNumber: 0 });
    expect(text(el)).toContain('o talento precisa de um efeito “Escolha”');
  });

  it("puts the server's refusals on the add button and inside the row, which opens", () => {
    const issues: Record<string, string[]> = {
      'table_feat.options': ['O talento passou de 60 opções. Remova uma.'],
      'table_feat.options[1].name_pt': [
        'Duas opções desta característica têm o mesmo nome. Mude uma delas.',
      ],
    };
    const { el } = setup([named('A'), named('A')], {
      issuesOf: (path: string) => issues[path] ?? [],
      refused: ['table_feat.options[1].name_pt'],
    });
    expect(text(el.querySelector('.add')!)).toContain('O talento passou de 60 opções. Remova uma.');
    expect(el.querySelector('button[data-field="table_feat.options"]')).not.toBeNull();
    const rows = el.querySelectorAll('.row');
    expect(rows[0].classList.contains('row--open')).toBe(false);
    expect(rows[1].classList.contains('row--open')).toBe(true);
    expect(text(rows[1])).toContain('Duas opções desta característica têm o mesmo nome.');
  });

  it('says in the row that an option with no name is not saved', () => {
    const { el, button } = setup([named('', 'Algo escrito')]);
    button('Opção 1')!.click();
    TestBed.tick();
    expect(text(el.querySelector('.row')!)).toContain('Sem nome, esta opção não é salva.');
  });

  it('writes a typed name back into the list', () => {
    const { el, emitted, button } = setup([named('A')]);
    button('Opção 1')!.click();
    TestBed.tick();
    const input = el.querySelector<HTMLInputElement>(
      '[data-field="table_feat.options[0].name_pt"]',
    )!;
    input.value = 'Guarda Alta';
    input.dispatchEvent(new Event('input'));
    expect(emitted.at(-1)![0].name).toBe('Guarda Alta');
  });
});
