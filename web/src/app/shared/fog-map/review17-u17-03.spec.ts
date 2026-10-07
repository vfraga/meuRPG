import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { decodeVision } from '../../core/maps/vision';
import { visionResponse } from '../../core/maps/vision-testing';
import { FogBase } from './fog-base';

// Finding U17-3 (review/unit-17-contract.md)
describe('Review17 U17-3: failed fog tiles are never requested again', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    TestBed.configureTestingModule({ imports: [FogBase] });
  });
  afterEach(() => vi.useRealTimers());

  it('asks again for a tile whose request failed (429/503 with Retry-After)', () => {
    const vision = decodeVision(
      visionResponse(['....', '....', '....', '....'], {
        tilesPath: '/images/maps/m1/tiles/',
        tileSquares: 2,
        tiles: [{ $typeName: 'meurpg.maps.v1.MapTile', tx: 0, ty: 0, revision: 3 }],
      }),
    );
    const fixture = TestBed.createComponent(FogBase);
    fixture.componentRef.setInput('vision', vision);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const img = el.querySelector<HTMLImageElement>('.fb__tile')!;
    const requests: string[] = [];
    let current = img.getAttribute('src');
    requests.push(current!);

    img.dispatchEvent(new Event('error'));
    vi.advanceTimersByTime(120_000);
    fixture.detectChanges();

    const now = el.querySelector<HTMLImageElement>('.fb__tile')!;
    const after = now.getAttribute('src');
    if (now !== img || after !== current) {
      requests.push(after!);
    }
    current = after;
    // A retry shows as a new request: a different element or a changed URL (cache-bust).
    expect(requests.length).toBeGreaterThan(1);
  });
});
