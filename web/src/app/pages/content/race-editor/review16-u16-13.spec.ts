// Finding U16-13 in review/unit-16-web-content-campaigns.md
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { TableContentClient } from '../../../core/content/content-client';
import { catalog, menu } from '../../../core/content/content-testing';
import { RaceEditor } from './race-editor';

describe('Review16 U16-13: new sub-race keeps its race select until saved', () => {
  it('still shows the race select after a race is chosen on a new sub-race', () => {
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: TableContentClient, useValue: { save: vi.fn() } }],
    });
    const f = TestBed.createComponent(RaceEditor);
    f.componentRef.setInput('campaignId', 'camp-1');
    f.componentRef.setInput('mode', 'subrace');
    f.componentRef.setInput('catalog', catalog());
    f.componentRef.setInput('menu', menu());
    f.detectChanges();
    const e = f.nativeElement as HTMLElement;
    const select = e.querySelector<HTMLSelectElement>('[data-field="table_subrace.race_key"]')!;
    select.value = 'race:human';
    select.dispatchEvent(new Event('change'));
    f.detectChanges();
    expect(e.querySelector('[data-field="table_subrace.race_key"]')).not.toBeNull();
  });
});
