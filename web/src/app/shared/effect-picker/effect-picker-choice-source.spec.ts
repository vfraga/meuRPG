import { ComponentFixture, TestBed } from '@angular/core/testing';

import { choiceSourceMenu } from '../../core/content/content-testing-menu-choices';
import { type EffectDraft, draftToEffect, emptyEffect } from '../../core/content/effect-draft';
import { EffectPicker } from './effect-picker';

const BASE = 'table_class.levels[0].features[0].effects[0]';
const menu = choiceSourceMenu();

const tool = (over: Partial<EffectDraft> = {}): EffectDraft => ({
  ...emptyEffect('choice'),
  choice: 'tool',
  count: 1,
  ...over,
});
const feature = (over: Partial<EffectDraft> = {}): EffectDraft => ({
  ...emptyEffect('choice'),
  choice: 'feature',
  count: 1,
  ...over,
});

describe('EffectPicker: where the options of a choice come from', () => {
  const emitted: EffectDraft[] = [];
  let fixture: ComponentFixture<EffectPicker>;
  let el: HTMLElement;

  function setup(
    effect: EffectDraft,
    over: { offer?: boolean; issues?: Record<string, string[]> } = {},
  ) {
    emitted.length = 0;
    TestBed.resetTestingModule();
    fixture = TestBed.createComponent(EffectPicker);
    fixture.componentRef.setInput('effect', effect);
    fixture.componentRef.setInput('menu', menu);
    fixture.componentRef.setInput('basePath', BASE);
    fixture.componentRef.setInput('offerFeatures', over.offer ?? true);
    fixture.componentRef.setInput('issuesOf', (p: string) => over.issues?.[p] ?? []);
    fixture.componentInstance.effectChange.subscribe((e) => emitted.push(e));
    fixture.detectChanges();
    el = fixture.nativeElement as HTMLElement;
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const field = (name: string) => el.querySelector<HTMLElement>(`[data-field="${BASE}.${name}"]`);
  const more = () => el.querySelector<HTMLButtonElement>('button.more');
  const open = () => {
    more()!.click();
    fixture.detectChanges();
  };
  const cards = () =>
    Array.from(el.querySelectorAll<HTMLInputElement>('app-radio-cards input[type=radio]'));
  const boxes = () =>
    Array.from(field('filter')!.querySelectorAll<HTMLInputElement>('input[type=checkbox]'));
  const labelOf = (b: HTMLInputElement) =>
    text(b.closest('label')!.querySelector('.label')!).trim();
  const options = (select: HTMLSelectElement) =>
    Array.from(select.options).map((o) => o.text.trim());

  describe('a choice of tools', () => {
    it('keeps the source under "Mais opções" until the list or the filter has something', () => {
      setup(tool());
      expect(cards()).toHaveLength(0);
      expect(field('filter')).toBeNull();
      open();
      expect(cards()).toHaveLength(2);
      expect(text(el)).toContain('De onde vêm as opções');
      expect(field('filter')).toBeNull();
    });

    it('offers the two sources, the list being the one on by default', () => {
      setup(tool());
      open();
      expect(
        cards().map((c) => c.closest('label')!.textContent!.includes('Uma lista que eu escrevo')),
      ).toEqual([true, false]);
      expect(cards()[0].checked).toBe(true);
    });

    it('switching to the filter shows the kinds, hides the list and clears it', () => {
      setup(tool({ from: ['proficiency:dice-set'] }));
      open();
      cards()[1].click();
      fixture.detectChanges();
      expect(emitted.at(-1)).toMatchObject({ filterOn: true, filter: [], from: [] });
      setup(emitted.at(-1)!);
      expect(boxes().map(labelOf)).toEqual(['Armas', 'Ferramentas', 'Veículos e kits']);
      expect(el.querySelector('[data-field$=".from"]')).toBeNull();
      expect(text(el)).toContain('Marque ao menos um tipo.');
    });

    it('ticks the kinds in the menu order and says how many proficiencies the filter matches', () => {
      setup(tool({ filterOn: true }));
      boxes()[2].click();
      expect(emitted.at(-1)!.filter).toEqual(['other']);
      setup(tool({ filterOn: true, filter: ['other'] }));
      boxes()[0].click();
      expect(emitted.at(-1)!.filter).toEqual(['weapon', 'other']);
      setup(tool({ filterOn: true, filter: ['weapon', 'tool'] }));
      expect(text(el)).toContain('Oferece 57 proficiências hoje.');
      expect(boxes().map((b) => b.checked)).toEqual([true, true, false]);
      expect(field('filter')!.closest('.frow')!.classList).toContain('frow--wide');
    });

    it('shows a refusal of the filter on the group and writes the filter without the list', () => {
      setup(tool({ filterOn: true, filter: ['tool'] }), {
        issues: { [`${BASE}.filter`]: ['Marque ao menos um tipo, sem repetir.'] },
      });
      expect(text(el)).toContain('Marque ao menos um tipo, sem repetir.');
      const sent = draftToEffect(tool({ filterOn: true, filter: ['tool'] }), menu);
      expect(sent.filter?.proficiencyKinds).toEqual(['tool']);
      expect(sent.from).toBeUndefined();
    });

    it('going back to the list clears the kinds', () => {
      setup(tool({ filterOn: true, filter: ['tool'] }));
      cards()[0].click();
      expect(emitted.at(-1)).toMatchObject({ filterOn: false, filter: [] });
    });

    it('has no source cards for another kind of choice', () => {
      setup({ ...tool(), choice: 'skill' });
      open();
      expect(cards()).toHaveLength(0);
      expect(field('filter')).toBeNull();
    });
  });

  describe('a choice of features', () => {
    it('lists the features that have options, the SRD invocations left out, with "Nenhuma" first', () => {
      setup(feature({ fromFeature: 'feature:fighting-style' }));
      expect(options(field('from_feature') as HTMLSelectElement)).toEqual([
        'Nenhuma',
        'Dádiva do pacto',
        'Estilo de luta',
        'Truques de lâmina (Guarda do Corvo)',
      ]);
    });

    it('says how many options the feature offers today, beside the field', () => {
      setup(feature({ fromFeature: 'feature:class-guardiao-3f9a1c--truques@mesa' }));
      expect(text(field('from_feature')!.closest('.frow')!)).toContain('Oferece 3 opções hoje.');
    });

    it('picking a feature clears the list and the set, and hides them', () => {
      setup(feature({ from: ['option:archery'] }));
      open();
      const select = field('from_feature') as HTMLSelectElement;
      select.selectedIndex = options(select).indexOf('Estilo de luta');
      select.dispatchEvent(new Event('change'));
      fixture.detectChanges();
      expect(emitted.at(-1)).toMatchObject({
        fromFeature: 'feature:fighting-style',
        from: [],
        set: '',
      });
      setup(emitted.at(-1)!);
      expect(field('from')).toBeNull();
      expect(field('set')).toBeNull();
    });

    it('stays in view, unfolded, with no feature picked', () => {
      setup(feature());
      const select = field('from_feature') as HTMLSelectElement;
      expect(select).not.toBeNull();
      expect(text(select.closest('.frow')!)).toContain(
        'Vazio: a escolha usa a lista ou as opções desta característica.',
      );
    });

    it('is not offered where the features have no such source, unless a key is stored', () => {
      setup(feature(), { offer: false });
      open();
      expect(field('from_feature')).toBeNull();
      setup(feature({ fromFeature: 'feature:fighting-style' }), { offer: false });
      expect(field('from_feature')).not.toBeNull();
    });

    it('keeps a stored key the menu no longer lists, said in words', () => {
      setup(feature({ fromFeature: 'feature:gone' }));
      const select = field('from_feature') as HTMLSelectElement;
      expect(select.options[select.selectedIndex].text.trim()).toBe(
        'Uma característica que saiu da lista',
      );
      expect(text(field('from_feature')!.closest('.frow')!)).toContain(
        'Esta característica saiu da lista ou não tem mais opções.',
      );
    });

    it('changing the kind of choice drops the filter and the feature', () => {
      setup(tool({ filterOn: true, filter: ['tool'], fromFeature: 'feature:fighting-style' }));
      const select = field('choice') as HTMLSelectElement;
      select.selectedIndex = options(select).indexOf('Perícias');
      select.dispatchEvent(new Event('change'));
      expect(emitted.at(-1)).toMatchObject({
        choice: 'skill',
        filterOn: false,
        filter: [],
        fromFeature: '',
      });
    });
  });
});
