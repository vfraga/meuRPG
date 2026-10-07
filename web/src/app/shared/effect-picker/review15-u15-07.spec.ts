import { ComponentFixture, TestBed } from '@angular/core/testing';

import { type EffectDraft, emptyEffect } from '../../core/content/effect-draft';
import { menu } from '../../core/content/content-testing';
import { EffectPicker } from './effect-picker';

describe('Review15 U15-7: the range field (metres on screen, feet in the draft) must not rewrite what is typed', () => {
  let fixture: ComponentFixture<EffectPicker>;
  let input: HTMLInputElement;

  function setup() {
    TestBed.resetTestingModule();
    fixture = TestBed.createComponent(EffectPicker);
    fixture.componentRef.setInput('effect', { ...emptyEffect('sense'), sense: 'darkvision', rangeFt: 60 });
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

  it('keeps "4" as typed', async () => {
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
