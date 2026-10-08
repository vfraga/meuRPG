import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { RetryImage } from './retry-image';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RetryImage],
  template: `<img appRetryImage [src]="url()" alt="" />`,
})
class Host {
  url = signal('/images/m1?w=800');
}

describe('RetryImage', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  function setup() {
    const fixture = TestBed.createComponent(Host);
    fixture.detectChanges();
    const img = (fixture.nativeElement as HTMLElement).querySelector('img')!;
    const fail = () => img.dispatchEvent(new Event('error'));
    return { fixture, img, fail };
  }

  it('asks again after 2, 4, 8, 16 and 30 s, with a new address each time, and then stops', () => {
    const { img, fail } = setup();
    expect(img.getAttribute('src')).toBe('/images/m1?w=800');
    const waits = [2, 4, 8, 16, 30];
    waits.forEach((seconds, i) => {
      fail();
      vi.advanceTimersByTime(seconds * 1000 - 1);
      expect(img.getAttribute('src')).toBe(
        i === 0 ? '/images/m1?w=800' : `/images/m1?w=800&retry=${i}`,
      );
      vi.advanceTimersByTime(1);
      expect(img.getAttribute('src')).toBe(`/images/m1?w=800&retry=${i + 1}`);
    });
    fail();
    vi.advanceTimersByTime(120_000);
    expect(img.getAttribute('src')).toBe('/images/m1?w=800&retry=5');
  });

  it('adds the first parameter with a question mark', () => {
    const fixture = TestBed.createComponent(Host);
    fixture.componentInstance.url.set('/images/m1');
    fixture.detectChanges();
    const img = (fixture.nativeElement as HTMLElement).querySelector('img')!;
    img.dispatchEvent(new Event('error'));
    vi.advanceTimersByTime(2000);
    expect(img.getAttribute('src')).toBe('/images/m1?retry=1');
  });

  it('starts over for another picture', () => {
    const { fixture, img, fail } = setup();
    fail();
    vi.advanceTimersByTime(2000);
    fail();
    vi.advanceTimersByTime(4000);
    expect(img.getAttribute('src')).toBe('/images/m1?w=800&retry=2');
    fixture.componentInstance.url.set('/images/m2');
    fixture.detectChanges();
    fail();
    vi.advanceTimersByTime(2000);
    expect(img.getAttribute('src')).toBe('/images/m2?retry=1');
  });

  it('does not ask again once the screen is gone', () => {
    const { fixture, img, fail } = setup();
    fail();
    fixture.destroy();
    vi.advanceTimersByTime(60_000);
    expect(img.getAttribute('src')).toBe('/images/m1?w=800');
  });
});
