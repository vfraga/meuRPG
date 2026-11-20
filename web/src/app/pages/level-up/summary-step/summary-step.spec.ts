import { TestBed } from '@angular/core/testing';

import { SummaryStep } from './summary-step';
import type { LevelUpSession } from '../level-up-session';

describe('SummaryStep: the feat taken (MR-025)', () => {
  function render(featSummary: { name: string; increase: string } | null) {
    const session = {
      draft: { featSummary: () => featSummary, swap: { summaries: () => [] } },
      rows: () => [],
      masterAdds: () => '',
    } as unknown as LevelUpSession;
    const fixture = TestBed.createComponent(SummaryStep);
    fixture.componentRef.setInput('s', session);
    fixture.detectChanges();
    return (fixture.nativeElement as HTMLElement).textContent?.replace(/\s+/g, ' ') ?? '';
  }

  it('names the feat and what it raises above the changes', () => {
    const text = render({ name: 'Atleta', increase: '+1 Força, +1 Destreza' });
    expect(text).toContain('Talento: Atleta');
    expect(text).toContain('+1 Força, +1 Destreza');
    // Only once (a duplicated block once listed it twice).
    expect(text.split('Talento: Atleta')).toHaveLength(2);
  });

  it('says nothing of a feat when the increase was taken', () => {
    expect(render(null)).not.toContain('Talento');
  });
});
