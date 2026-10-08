import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { type Vision, decodeVision } from '../../core/maps/vision';
import { visionResponse } from '../../core/maps/vision-testing';
import { FogBase } from './fog-base';

const rows = ['....', '.gB.', '.dr.', '....'];

function tiled(partial: Parameters<typeof visionResponse>[1] = {}): Vision {
  return decodeVision(
    visionResponse(rows, {
      tilesPath: '/images/maps/m1/tiles/',
      tileSquares: 2,
      tiles: [
        { $typeName: 'meurpg.maps.v1.MapTile', tx: 0, ty: 0, revision: 3 },
        { $typeName: 'meurpg.maps.v1.MapTile', tx: 1, ty: 1, revision: 1 },
      ],
      ...partial,
    }),
  );
}

describe('FogBase', () => {
  beforeEach(() => TestBed.configureTestingModule({ imports: [FogBase] }));

  function create(vision: Vision, inputs: Record<string, unknown> = {}) {
    const fixture = TestBed.createComponent(FogBase);
    fixture.componentRef.setInput('vision', vision);
    for (const [k, v] of Object.entries(inputs)) {
      fixture.componentRef.setInput(k, v);
    }
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  it('is a drawing: hidden from assistive tech, on the grid of the map', () => {
    const { el } = create(tiled());
    expect(el.getAttribute('aria-hidden')).toBe('true');
    expect(el.style.getPropertyValue('--cols')).toBe('4');
    expect(el.style.getPropertyValue('--rows')).toBe('4');
  });

  it('shades each state its own way and says how many squares of each, for a test to read without pixels', () => {
    const { el } = create(tiled());
    expect(el.getAttribute('data-shaded')).toBe('dim:1 grey:1 remembered:1 unseen:12');
    expect(el.querySelectorAll('[data-shade="grey"]').length).toBe(1);
    expect(el.querySelectorAll('[data-shade="remembered"]').length).toBe(1);
    expect(el.querySelectorAll('[data-shade="dim"]').length).toBe(1);
    // "Visto" (bright light) is the image as it is: no shade of its own.
    expect(el.querySelectorAll('[data-shade]').length).toBe(
      el.querySelectorAll('[data-shade="unseen"]').length + 3,
    );
  });

  it('puts "já visto" under dots and "no escuro" under a filter that takes the colour away', () => {
    const { el } = create(tiled());
    expect(el.querySelector('.fb__dots')).not.toBeNull();
    expect(el.querySelector('.fb__grey')).not.toBeNull();
  });

  it('draws each tile the server made at its place, with its revision in the URL, and never the whole image', () => {
    const { el } = create(tiled());
    const tiles = Array.from(el.querySelectorAll<HTMLImageElement>('.fb__tile'));
    expect(tiles.map((t) => t.getAttribute('src'))).toEqual([
      '/images/maps/m1/tiles/0/0?r=3',
      '/images/maps/m1/tiles/1/1?r=1',
    ]);
    expect(tiles[1].style.left).toBe('50%');
    expect(tiles[1].style.top).toBe('50%');
    expect(tiles[1].style.width).toBe('50%');
    expect(el.querySelector('.fb__img')).toBeNull();
  });

  it('asks as the character the master reads as', () => {
    const { el } = create(tiled(), { forCharacter: 'toren-id' });
    expect(el.querySelector('.fb__tile')?.getAttribute('src')).toBe(
      '/images/maps/m1/tiles/0/0?r=3&as=toren-id',
    );
  });

  it('shows a still, striped place with a dashed border until a tile arrives, and counts them in', () => {
    const { fixture, el } = create(tiled());
    expect(el.querySelectorAll('[data-pending]').length).toBe(2);
    const settled: ReadonlySet<string>[] = [];
    fixture.componentInstance.settledChange.subscribe((s) => settled.push(s));
    const first = el.querySelector<HTMLImageElement>('.fb__tile')!;
    first.dispatchEvent(new Event('load'));
    fixture.detectChanges();
    expect(el.querySelectorAll('[data-pending]').length).toBe(1);
    expect(el.getAttribute('data-tiles-ready')).toBe('1');
    expect(el.getAttribute('data-tiles')).toBe('2');
    expect(Array.from(settled.at(-1) ?? [])).toEqual(['0:0']);
  });

  describe('a tile the server did not give', () => {
    beforeEach(() => vi.useFakeTimers());
    afterEach(() => vi.useRealTimers());

    const fail = (el: HTMLElement) =>
      el
        .querySelectorAll<HTMLImageElement>('.fb__tile')
        .forEach((t) => t.dispatchEvent(new Event('error')));

    it('is asked for again after a wait, by a new URL, and keeps its place waiting', () => {
      const { fixture, el } = create(tiled());
      fail(el);
      fixture.detectChanges();
      expect(el.querySelectorAll('[data-pending]').length).toBe(2);
      vi.advanceTimersByTime(2_000);
      fixture.detectChanges();
      expect(el.querySelector('.fb__tile')?.getAttribute('src')).toBe(
        '/images/maps/m1/tiles/0/0?r=3&retry=1',
      );
      el.querySelectorAll<HTMLImageElement>('.fb__tile').forEach((t) =>
        t.dispatchEvent(new Event('load')),
      );
      fixture.detectChanges();
      expect(el.querySelectorAll('[data-pending]').length).toBe(0);
      expect(el.getAttribute('data-tiles-ready')).toBe('2');
    });

    it('stops waiting for a tile that failed every attempt: its place is black, not a spinner that never ends', () => {
      const { fixture, el } = create(tiled());
      for (let attempt = 0; attempt < 6; attempt++) {
        fail(el);
        fixture.detectChanges();
        vi.advanceTimersByTime(30_000);
        fixture.detectChanges();
      }
      expect(el.querySelectorAll('[data-pending]').length).toBe(0);
    });

    it('does not ask again once the component is gone', () => {
      const { fixture, el } = create(tiled());
      fail(el);
      const waiting = vi.getTimerCount();
      fixture.destroy();
      // The two waits for the two tiles are cancelled.
      expect(vi.getTimerCount()).toBeLessThanOrEqual(waiting - 2);
    });
  });

  it('keeps the old pixels of a tile while a changed one is fetched: the place is not waiting again', () => {
    const { fixture, el } = create(tiled());
    el.querySelectorAll<HTMLImageElement>('.fb__tile').forEach((t) =>
      t.dispatchEvent(new Event('load')),
    );
    fixture.componentRef.setInput(
      'vision',
      tiled({
        revision: 2,
        tiles: [
          { $typeName: 'meurpg.maps.v1.MapTile', tx: 0, ty: 0, revision: 9 },
          { $typeName: 'meurpg.maps.v1.MapTile', tx: 1, ty: 1, revision: 1 },
        ],
      }),
    );
    fixture.detectChanges();
    expect(el.querySelector('.fb__tile')?.getAttribute('src')).toBe(
      '/images/maps/m1/tiles/0/0?r=9',
    );
    expect(el.querySelectorAll('[data-pending]').length).toBe(0);
    // The unchanged tile keeps its URL, so the browser does not fetch it again.
    expect(el.querySelectorAll<HTMLImageElement>('.fb__tile')[1].getAttribute('src')).toBe(
      '/images/maps/m1/tiles/1/1?r=1',
    );
  });

  it('draws the whole image for a viewer that reads it whole, with no tile and no waiting', () => {
    const whole = decodeVision(visionResponse(['BB', 'BB']));
    const { el } = create(whole, { image: { url: '/images/abc', width: 960, height: 640 } });
    expect(el.querySelector('.fb__img')?.getAttribute('src')).toBe('/images/abc');
    expect(el.querySelectorAll('.fb__tile').length).toBe(0);
    expect(el.querySelectorAll('[data-pending]').length).toBe(0);
    expect(el.getAttribute('data-shaded')).toBe('dim:0 grey:0 remembered:0 unseen:0');
  });
});
