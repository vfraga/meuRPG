// Review 12, finding U12-03: the in-place "Passar o turno mesmo assim?" question
// is never reset when its inputs change, so a stale confirm still emits
// next(true) (discard) for whatever turn is current by then.
import { TestBed } from '@angular/core/testing';

import { NextTurn } from './next-turn';

describe('Review12 U12-03: NextTurn stale discard question', () => {
  function open() {
    const fixture = TestBed.createComponent(NextTurn);
    fixture.componentRef.setInput('pendingNote', 'Falta aplicar 5 de dano');
    const sent: boolean[] = [];
    fixture.componentInstance.next.subscribe((d) => sent.push(d));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    el.querySelector<HTMLButtonElement>('.next')!.click();
    fixture.detectChanges();
    expect(el.querySelector('[role="alertdialog"]')).not.toBeNull();
    return { fixture, el, sent };
  }

  it('closes the question when the owed damage is gone (applied elsewhere)', () => {
    const { fixture, el } = open();
    fixture.componentRef.setInput('pendingNote', null);
    fixture.detectChanges();
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('does not emit a discard once the owed damage is gone', () => {
    const { fixture, el, sent } = open();
    fixture.componentRef.setInput('pendingNote', null);
    fixture.detectChanges();
    el.querySelector<HTMLButtonElement>('.ask__go')?.click();
    expect(sent).not.toContain(true);
  });
});
