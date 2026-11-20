import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { SheetFrame } from './sheet-frame';
import { nextTurn } from '../../../../test-settle';

@Component({
  imports: [SheetFrame],
  template: `<app-sheet-frame title="Teste"><p id="first">Um</p>@if (more()) {<p id="later">Depois</p>}</app-sheet-frame>`,
})
class Host {
  more = signal(false);
}

describe('SheetFrame', () => {
  const realResize = globalThis.ResizeObserver;
  const resizeCallbacks: (() => void)[] = [];

  beforeEach(() => {
    resizeCallbacks.length = 0;
    globalThis.ResizeObserver = class {
      constructor(cb: () => void) {
        resizeCallbacks.push(cb);
      }
      observe() {}
      disconnect() {}
      unobserve() {}
    } as never;
  });
  afterEach(() => {
    globalThis.ResizeObserver = realResize;
  });

  it('turns the shadows on when content added after the first render makes the body overflow', async () => {
    const fixture = TestBed.createComponent(Host);
    fixture.detectChanges();
    await fixture.whenStable();
    const body = fixture.nativeElement.querySelector('.frame__body') as HTMLElement;
    let tall = false;
    Object.defineProperty(body, 'scrollHeight', {
      get: () => (tall ? 500 : 100),
      configurable: true,
    });
    Object.defineProperty(body, 'clientHeight', { get: () => 200, configurable: true });
    expect(body.classList.contains('frame__body--scrolls')).toBe(false);

    tall = true;
    fixture.componentInstance.more.set(true);
    fixture.detectChanges();
    // The mutation observer reports the new child and the frame checks again.
    await nextTurn();
    fixture.detectChanges();
    expect(body.classList.contains('frame__body--scrolls')).toBe(true);
  });

  it('puts the icon in a tile with the title and the subtitle beside it, and the subtitle under the head otherwise', () => {
    const make = (tile: boolean) => {
      const f = TestBed.createComponent(SheetFrame);
      f.componentRef.setInput('title', 'A montaria caiu');
      f.componentRef.setInput('subtitle', 'Reação · desmontar');
      f.componentRef.setInput('icon', 'download');
      f.componentRef.setInput('tile', tile);
      f.detectChanges();
      return f.nativeElement as HTMLElement;
    };
    const tiled = make(true);
    expect(tiled.querySelector('.frame__head')?.classList).toContain('frame__head--tile');
    // The subtitle is in the block of the title, beside the tile.
    expect(tiled.querySelector('.frame__titles .frame__sub')).not.toBeNull();
    expect(tiled.querySelector('.frame__titles .frame__title')).not.toBeNull();
    const plain = make(false);
    expect(plain.querySelector('.frame__head')?.classList).not.toContain('frame__head--tile');
  });

  it('a sheet without a tile keeps the old structure: the subtitle is a sibling after the head, and the head is a plain row', () => {
    const f = TestBed.createComponent(SheetFrame);
    f.componentRef.setInput('title', 'Escudo');
    f.componentRef.setInput('subtitle', 'Reação');
    f.detectChanges();
    const el = f.nativeElement as HTMLElement;
    const head = el.querySelector('.frame__head')!;
    expect(head.nextElementSibling).toBe(el.querySelector('.frame__sub'));
    expect(head.querySelector('.frame__sub')).toBeNull();
    expect(Array.from(head.children).map((c) => c.className)).toEqual([
      'frame__titles',
      'frame__close',
    ]);
    expect(getComputedStyle(el.querySelector('.frame__titles')!).display).toBe('block');
    expect(getComputedStyle(el.querySelector('.frame__sub')!).marginTop).toBe('-8px');
  });

  it('takes a glyph of its own before the title, and a subtitle with a mark and a bold lead', () => {
    @Component({
      imports: [SheetFrame],
      changeDetection: ChangeDetectionStrategy.OnPush,
      template: `<app-sheet-frame title="A montaria caiu" subtitleLead="Reação" subtitle=" · desmontar"
        ><i head-icon id="glyph"></i><i sub-mark id="mark"></i
      ></app-sheet-frame>`,
    })
    class Rich {}
    const fixture = TestBed.createComponent(Rich);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('.frame__head #glyph')).not.toBeNull();
    const sub = el.querySelector('.frame__sub')!;
    expect(sub.textContent?.trim()).toBe('Reação · desmontar');
    expect(sub.querySelector('b')?.textContent).toBe('Reação');
    expect(sub.firstElementChild?.id).toBe('mark');
  });
});
