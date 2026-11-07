import { TestBed } from '@angular/core/testing';

import { MasterWait } from './master-wait';

describe('MasterWait', () => {
  function render(title: string, detail?: string) {
    const fixture = TestBed.createComponent(MasterWait);
    fixture.componentRef.setInput('title', title);
    if (detail !== undefined) {
      fixture.componentRef.setInput('detail', detail);
    }
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('is a status card with the hourglass, the title and what waits', () => {
    const el = render('Esperando o mestre', 'O resultado do seu ataque sai quando ele responder.');
    expect(el.getAttribute('role')).toBe('status');
    expect(el.querySelector('mat-icon')?.textContent?.trim()).toBe('hourglass_top');
    expect(el.querySelector('.wait__title')?.textContent).toContain('Esperando o mestre');
    expect(el.querySelector('.wait__detail')?.textContent).toBe(
      'O resultado do seu ataque sai quando ele responder.',
    );
  });

  it('reads as one sentence: the title ends with a stop only a screen reader hears', () => {
    const el = render('Esperando o mestre', 'O turno continua quando ele responder.');
    expect(
      el.textContent
        ?.replace(/hourglass_top/, '')
        .replace(/\s+/g, ' ')
        .trim(),
    ).toBe('Esperando o mestre. O turno continua quando ele responder.');
    expect(el.querySelector('.mr-visually-hidden')?.textContent).toBe('. ');
  });

  it('draws no detail line when there is none', () => {
    expect(render('Esperando o mestre').querySelector('.wait__detail')).toBeNull();
  });
});
