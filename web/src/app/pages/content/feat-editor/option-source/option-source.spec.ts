import { TestBed } from '@angular/core/testing';

import { menu } from '../../../../core/content/content-testing-menu';
import { type EffectDraft, emptyEffect } from '../../../../core/content/effect-draft';
import { OptionSource } from './option-source';

const text = (e: Element) => (e.textContent ?? '').replace(/\s+/g, ' ').trim();
const choice = (patch: Partial<EffectDraft> = {}): EffectDraft => ({
  ...emptyEffect('choice'),
  choice: 'feature',
  ...patch,
});

function setup(effect: EffectDraft, extra: Record<string, unknown> = {}) {
  TestBed.resetTestingModule();
  const fixture = TestBed.createComponent(OptionSource);
  const el = fixture.nativeElement as HTMLElement;
  const emitted: EffectDraft[] = [];
  fixture.componentRef.setInput('effect', effect);
  fixture.componentRef.setInput('menu', menu());
  fixture.componentRef.setInput('basePath', 'table_feat.effects[0]');
  Object.entries(extra).forEach(([k, v]) => fixture.componentRef.setInput(k, v));
  fixture.componentInstance.effectChange.subscribe((next) => {
    emitted.push(next);
    fixture.componentRef.setInput('effect', next);
    fixture.detectChanges();
  });
  fixture.detectChanges();
  const radios = () => Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
  const on = () => radios().findIndex((r) => r.checked);
  return { fixture, el, emitted, radios, on };
}

describe('OptionSource: "De onde vêm as opções"', () => {
  it('draws the three cards with the one that fits the choice on', () => {
    const { el, radios, on } = setup(choice(), { ownCount: 4 });
    expect(radios()).toHaveLength(3);
    expect(Array.from(el.querySelectorAll('.title')).map(text)).toEqual([
      'As opções do talento',
      'Um conjunto inteiro',
      'Opções do SRD que eu marco',
    ]);
    expect(Array.from(el.querySelectorAll('.desc')).map(text)).toEqual([
      'As 4 do painel abaixo.',
      'Todas de um conjunto do SRD.',
      'Uma lista escolhida a dedo.',
    ]);
    expect(on()).toBe(0);
    expect(el.querySelector('app-select-field')).toBeNull();
  });

  it('says the panel is empty before it has options', () => {
    const { el } = setup(choice());
    expect(text(el.querySelector('.desc')!)).toBe('Escritas no painel abaixo.');
  });

  it('shows the set with its select, the note of the set and the footer note, and names the set on its card', () => {
    const { el, on } = setup(choice({ set: 'fighting_style' }));
    expect(on()).toBe(1);
    const select = el.querySelector<HTMLSelectElement>('app-select-field select')!;
    expect(select.value).toBe('fighting_style');
    expect(Array.from(select.options).map((o) => o.textContent!.trim())).toEqual([
      'Invocação mística',
      'Estilo de luta',
    ]);
    expect(text(el.querySelectorAll('.desc')[1])).toBe('Estilo de luta');
    expect(text(el.querySelector('.set__note')!)).toBe(
      'Os estilos de luta do guerreiro, do paladino e do patrulheiro.',
    );
    expect(text(el.querySelector('.foot')!)).toBe(
      'As opções da mesa no conjunto entram junto; uma oculta não aparece para o jogador.',
    );
  });

  it('turns on the marked list when the choice lists SRD options', () => {
    const { on } = setup(choice({ from: ['feature:fighting-style'] }));
    expect(on()).toBe(2);
  });

  it('picks a set: the first one, and the marked list goes', () => {
    const { emitted, radios, on } = setup(choice({ from: ['feature:fighting-style'] }));
    radios()[1].click();
    expect(emitted.at(-1)).toMatchObject({ set: 'invocation', from: [] });
    expect(on()).toBe(1);
  });

  it('changes the set from the select', () => {
    const { el, emitted } = setup(choice({ set: 'invocation' }));
    const select = el.querySelector<HTMLSelectElement>('app-select-field select')!;
    select.value = 'fighting_style';
    select.dispatchEvent(new Event('change'));
    expect(emitted.at(-1)!.set).toBe('fighting_style');
  });

  it('goes back to the options of the feat: the set and the list are cleared', () => {
    const { emitted, radios, on } = setup(choice({ set: 'fighting_style' }));
    radios()[0].click();
    expect(emitted.at(-1)).toMatchObject({ set: '', from: [] });
    expect(on()).toBe(0);
  });

  it('keeps the marked-list card on while nothing is marked yet', () => {
    const { emitted, radios, on } = setup(choice({ set: 'fighting_style' }));
    radios()[2].click();
    expect(emitted.at(-1)).toMatchObject({ set: '' });
    expect(on()).toBe(2);
    radios()[0].click();
    expect(on()).toBe(0);
  });

  it('puts the refusal of the pair under the cards, on the first card', () => {
    const message =
      'Este talento tem opções próprias: a escolha oferece elas ou um conjunto, não os dois.';
    const { el } = setup(choice({ set: 'fighting_style' }), {
      issuesOf: (path: string) => (path === 'table_feat.effects[0].set' ? [message] : []),
    });
    const first = el.querySelector('input[type="radio"]')!;
    expect(first.getAttribute('data-field')).toBe('table_feat.effects[0].set');
    expect(text(el.querySelector('app-radio-cards')!)).toContain(message);
  });
});
