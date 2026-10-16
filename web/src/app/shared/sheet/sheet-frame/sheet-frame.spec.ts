import { Component, signal } from '@angular/core';
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
});
