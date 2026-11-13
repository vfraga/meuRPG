import { TestBed } from '@angular/core/testing';

import type { SunChange } from '../../../core/combat/sunlight-paint';
import { SunlightLayer } from './sunlight-layer';

const sq = (col: number, row: number) => ({ col, row });

function mount(inputs: Record<string, unknown> = {}) {
  const fixture = TestBed.createComponent(SunlightLayer);
  fixture.componentRef.setInput('columns', 10);
  fixture.componentRef.setInput('rows', 10);
  for (const [k, v] of Object.entries(inputs)) {
    fixture.componentRef.setInput(k, v);
  }
  const changes: SunChange[] = [];
  fixture.componentInstance.paint.subscribe((c) => changes.push(c));
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const pad = () => el.querySelector<HTMLElement>('.sl__pad');
  // The layer is 100 x 100 px at the origin: a square is 10 px.
  el.getBoundingClientRect = () => new DOMRect(0, 0, 100, 100);
  return { fixture, el, pad, changes };
}

const pointer = (type: string, x: number, y: number, pointerId = 1) =>
  new PointerEvent(type, { clientX: x, clientY: y, pointerId, bubbles: true, button: 0 });

describe('SunlightLayer', () => {
  it('draws crosses and a dotted edge for the squares, and nothing for none', () => {
    const none = mount();
    expect(none.el.querySelector('svg')).toBeNull();
    const some = mount({ squares: [sq(1, 1), sq(2, 1)] });
    expect(some.el.querySelector('.sl__dot')).not.toBeNull();
    expect(some.el.querySelector('.sl__edge')).not.toBeNull();
    expect(some.el.querySelector('.sl__frame')).toBeNull();
    expect(some.el.querySelector('.sl__pad')).toBeNull();
  });

  it('draws only a frame when the sun is over the whole map', () => {
    const { el } = mount({ wholeMap: true });
    expect(el.querySelector('.sl__frame')).not.toBeNull();
    expect(el.querySelector('.sl__dot')).toBeNull();
  });

  it('sends one change for a drag, when the pointer is released', () => {
    const { fixture, pad, changes } = mount({ painting: true });
    pad()!.setPointerCapture = () => undefined;
    pad()!.dispatchEvent(pointer('pointerdown', 5, 5));
    pad()!.dispatchEvent(pointer('pointermove', 35, 5));
    fixture.detectChanges();
    expect(changes).toEqual([]);
    pad()!.dispatchEvent(pointer('pointerup', 35, 5));
    expect(changes).toEqual([
      { addSquares: [sq(0, 0), sq(1, 0), sq(2, 0), sq(3, 0)], removeSquares: [] },
    ]);
  });

  it('sends the whole erase stroke, for the answer in the air may be older than it', () => {
    const { pad, changes } = mount({ painting: true, tool: 'erase', squares: [sq(1, 0)] });
    pad()!.setPointerCapture = () => undefined;
    pad()!.dispatchEvent(pointer('pointerdown', 5, 5));
    pad()!.dispatchEvent(pointer('pointermove', 15, 5));
    pad()!.dispatchEvent(pointer('pointerup', 15, 5));
    expect(changes).toEqual([{ addSquares: [], removeSquares: [sq(0, 0), sq(1, 0)] }]);
  });

  it('sends nothing for a cancelled stroke', () => {
    const { pad, changes } = mount({ painting: true });
    pad()!.setPointerCapture = () => undefined;
    pad()!.dispatchEvent(pointer('pointerdown', 5, 5));
    pad()!.dispatchEvent(pointer('pointercancel', 5, 5));
    expect(changes).toEqual([]);
  });

  it('moves a cursor with the arrows, announces it and paints with Space', () => {
    const { fixture, el, pad, changes } = mount({ painting: true, squares: [sq(1, 0)] });
    const key = (k: string) => {
      pad()!.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true }));
      fixture.detectChanges();
    };
    const note = () => el.querySelector('[role="status"]')!.textContent?.trim();
    pad()!.dispatchEvent(new Event('focus'));
    fixture.detectChanges();
    expect(note()).toBe('Coluna 1, linha 1: sem luz do sol');
    key('ArrowRight');
    expect(note()).toBe('Coluna 2, linha 1: luz do sol');
    key('ArrowRight');
    expect(note()).toBe('Coluna 3, linha 1: sem luz do sol');
    key(' ');
    expect(changes).toEqual([{ addSquares: [sq(2, 0)], removeSquares: [] }]);
    // The server's answer lights it: the live region says so.
    fixture.componentRef.setInput('squares', [sq(1, 0), sq(2, 0)]);
    fixture.detectChanges();
    expect(note()).toBe('Coluna 3, linha 1: luz do sol');
  });

  it('leaves one finger to paint and the pinch to the browser: touch-action pinch-zoom on the pad', () => {
    const { pad } = mount({ painting: true });
    expect(getComputedStyle(pad()!).touchAction).toContain('pinch-zoom');
  });

  it('drops the stroke when a second finger goes down, and scrolls the map by the two fingers', () => {
    const { fixture, el, pad, changes } = mount({ painting: true });
    const holder = document.createElement('div');
    holder.style.cssText = 'overflow-x:auto;overflow-y:auto;height:50px;width:50px';
    holder.append(el);
    document.body.append(holder);
    const by = vi.fn();
    holder.scrollBy = by;
    Object.defineProperty(holder, 'scrollHeight', { value: 500 });
    pad()!.setPointerCapture = () => undefined;
    pad()!.dispatchEvent(pointer('pointerdown', 5, 5, 1));
    pad()!.dispatchEvent(pointer('pointerdown', 25, 5, 2));
    pad()!.dispatchEvent(pointer('pointermove', 5, 5, 1));
    pad()!.dispatchEvent(pointer('pointermove', 25, 5, 2));
    pad()!.dispatchEvent(pointer('pointermove', 5, 0, 1));
    pad()!.dispatchEvent(pointer('pointermove', 25, 0, 2));
    // Both fingers rose by 5 px: the midpoint moved 5 px (2.5 per finger move), so the map scrolls 5 px down.
    expect(by).toHaveBeenCalled();
    expect(by.mock.calls.reduce((n, c) => n + (c[1] as number), 0)).toBe(5);
    pad()!.dispatchEvent(pointer('pointerup', 5, 0, 1));
    pad()!.dispatchEvent(pointer('pointerup', 25, 0, 2));
    fixture.detectChanges();
    expect(changes).toEqual([]);
    holder.remove();
  });

  it('paints nothing with the finger left down after the other lifts', () => {
    const { pad, changes } = mount({ painting: true });
    pad()!.setPointerCapture = () => undefined;
    pad()!.dispatchEvent(pointer('pointerdown', 5, 5, 1));
    pad()!.dispatchEvent(pointer('pointerdown', 25, 5, 2));
    pad()!.dispatchEvent(pointer('pointerup', 5, 5, 1));
    pad()!.dispatchEvent(pointer('pointermove', 45, 5, 2));
    pad()!.dispatchEvent(pointer('pointerup', 45, 5, 2));
    expect(changes).toEqual([]);
  });

  it('keeps the keys to itself while painting', () => {
    const { pad } = mount({ painting: true });
    const outer = vi.fn();
    (pad()!.parentElement as HTMLElement).addEventListener('keydown', outer);
    pad()!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    expect(outer).not.toHaveBeenCalled();
  });
});
