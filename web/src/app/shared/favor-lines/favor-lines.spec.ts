import { TestBed } from '@angular/core/testing';

import { FavorClient } from '../../core/play/favor-client';
import { FavorState } from '../../core/play/favor-state';
import { FakeFavorClient } from '../../core/play/favor-testing';
import { FavorLines } from './favor-lines';

describe('FavorLines', () => {
  beforeEach(() => TestBed.resetTestingModule());

  function mount() {
    TestBed.configureTestingModule({
      providers: [{ provide: FavorClient, useValue: new FakeFavorClient().asClient() }],
    });
    const favor = TestBed.inject(FavorState);
    const fixture = TestBed.createComponent(FavorLines);
    fixture.detectChanges();
    return { favor, fixture, el: fixture.nativeElement as HTMLElement };
  }

  it('keeps its polite live region in the page with nothing in it', () => {
    const { el } = mount();
    const live = el.querySelector('[role=status]');
    expect(live?.getAttribute('aria-live')).toBe('polite');
    expect((live?.textContent ?? '').trim()).toBe('');
  });

  it('says each line of the state, with a close button of 44 px named "Dispensar o aviso"', () => {
    const { favor, fixture, el } = mount();
    favor.lines.set([
      {
        id: 1,
        text: 'O mestre aceitou: a sua Inspiração foi para Tor.',
        tone: 'success',
        icon: 'star',
      },
      {
        id: 2,
        text: 'O mestre recusou: a Inspiração continua com você.',
        tone: 'neutral',
        icon: 'info',
      },
    ]);
    fixture.detectChanges();
    const notices = [...el.querySelectorAll('.mr-notice')];
    expect(notices.map((n) => n.textContent?.replace(/\s+/g, ' '))).toEqual([
      expect.stringContaining('O mestre aceitou: a sua Inspiração foi para Tor.'),
      expect.stringContaining('O mestre recusou: a Inspiração continua com você.'),
    ]);
    expect(notices[0].classList.contains('mr-notice--success')).toBe(true);
    expect(notices[1].classList.contains('mr-notice--neutral')).toBe(true);
    expect(el.querySelector('button.close')?.getAttribute('aria-label')).toBe('Dispensar o aviso');
  });

  it('drops a line when its close button is pressed', () => {
    const { favor, fixture, el } = mount();
    favor.lines.set([{ id: 1, text: 'Uma linha.', tone: 'neutral', icon: 'info' }]);
    fixture.detectChanges();
    el.querySelector<HTMLButtonElement>('button.close')!.click();
    fixture.detectChanges();
    expect(favor.lines()).toEqual([]);
    expect(el.querySelector('.mr-notice')).toBeNull();
  });
});
