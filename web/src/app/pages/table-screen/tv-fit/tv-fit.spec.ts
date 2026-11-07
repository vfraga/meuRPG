import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { TvFit } from './tv-fit';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TvFit],
  template: `
    <ul [appTvFit]="items()" (fitted)="left.set($event)">
      @for (i of items(); track i) {
        <li [attr.data-count]="i === 'sep' ? null : ''">{{ i }}</li>
      }
    </ul>
  `,
})
class Host {
  readonly items = signal(['a', 'b', 'sep', 'c', 'd']);
  readonly left = signal(-1);
}

/** The layout is the browser's; the spec gives each element a height and the list a ceiling. */
function lay(el: HTMLElement, ceiling: number, rowHeight = 10): void {
  const list = el.querySelector('ul')!;
  Object.defineProperty(list, 'clientHeight', { value: ceiling, configurable: true });
  list.getBoundingClientRect = () => ({ bottom: ceiling }) as DOMRect;
  [...list.children].forEach((li, n) => {
    (li as HTMLElement).getBoundingClientRect = () => ({ bottom: (n + 1) * rowHeight }) as DOMRect;
  });
}

describe('TvFit', () => {
  function render() {
    const fixture = TestBed.createComponent(Host);
    fixture.detectChanges();
    const list = fixture.nativeElement.querySelector('ul') as HTMLUListElement;
    const fit = () => {
      const directive = fixture.debugElement.children[0].injector.get(TvFit);
      directive.fit();
      fixture.detectChanges();
    };
    const shown = () =>
      [...list.children]
        .filter((li) => (li as HTMLElement).style.display !== 'none')
        .map((li) => li.textContent);
    return { fixture, list, fit, shown };
  }

  it('draws every child when they all fit', () => {
    const { fixture, fit, shown } = render();
    lay(fixture.nativeElement, 50);
    fit();
    expect(shown()).toEqual(['a', 'b', 'sep', 'c', 'd']);
    expect(fixture.componentInstance.left()).toBe(0);
  });

  it('hides the child that is cut and every one after it, and counts the ones with data-count', () => {
    const { fixture, fit, shown } = render();
    lay(fixture.nativeElement, 25);
    fit();
    expect(shown()).toEqual(['a', 'b']);
    // sep has no data-count; c and d do.
    expect(fixture.componentInstance.left()).toBe(2);
  });

  it('draws a child that ends exactly at the edge', () => {
    const { fixture, fit, shown } = render();
    lay(fixture.nativeElement, 30);
    fit();
    expect(shown()).toEqual(['a', 'b', 'sep']);
    expect(fixture.componentInstance.left()).toBe(2);
  });

  it('shows them again when there is room', () => {
    const { fixture, fit, shown } = render();
    lay(fixture.nativeElement, 15);
    fit();
    expect(shown()).toEqual(['a']);
    lay(fixture.nativeElement, 100);
    fit();
    expect(shown()).toEqual(['a', 'b', 'sep', 'c', 'd']);
    expect(fixture.componentInstance.left()).toBe(0);
  });

  it('keeps the first child even when not even it fits', () => {
    const { fixture, fit, shown } = render();
    lay(fixture.nativeElement, 5);
    fit();
    expect(shown()).toEqual(['a']);
    // b, c and d are counted; sep is not.
    expect(fixture.componentInstance.left()).toBe(3);
  });

  it('hides nothing in a list with no height of its own (a phone)', () => {
    const { fixture, fit, shown } = render();
    lay(fixture.nativeElement, 0);
    fit();
    expect(shown()).toEqual(['a', 'b', 'sep', 'c', 'd']);
  });

  it('lets a row go two pixels past the edge (sub-pixel rounding) but not three', () => {
    const { fixture, fit, shown } = render();
    lay(fixture.nativeElement, 28);
    fit();
    expect(shown()).toEqual(['a', 'b', 'sep']);
    lay(fixture.nativeElement, 27);
    fit();
    expect(shown()).toEqual(['a', 'b']);
  });

  describe('when the layout settles after the first fit', () => {
    const observed: Element[] = [];
    let notify: () => void = () => undefined;

    beforeEach(() => {
      observed.length = 0;
      vi.stubGlobal(
        'ResizeObserver',
        class {
          constructor(callback: () => void) {
            notify = callback;
          }
          observe(el: Element): void {
            observed.push(el);
          }
          unobserve(el: Element): void {
            observed.splice(observed.indexOf(el), 1);
          }
          disconnect(): void {
            observed.length = 0;
          }
        },
      );
    });
    afterEach(() => vi.unstubAllGlobals());

    it('watches the children too and shows the row that fits once a sibling shrinks', () => {
      const { fixture, list, fit, shown } = render();
      lay(fixture.nativeElement, 100);
      fit();
      expect(observed).toContain(list);
      for (const li of [...list.children]) {
        expect(observed).toContain(li);
      }
      // The "+N" slot grew while the rows were measured: the rows past the edge are hidden...
      lay(fixture.nativeElement, 25);
      fit();
      expect(shown()).toEqual(['a', 'b']);
      // ...and when it settles back, the observer refits and the row that fits comes back.
      lay(fixture.nativeElement, 40);
      notify();
      fixture.detectChanges();
      expect(shown()).toEqual(['a', 'b', 'sep', 'c']);
      expect(fixture.componentInstance.left()).toBe(1);
    });

    it('stops watching a child that left the list', () => {
      const { fixture, list, fit } = render();
      lay(fixture.nativeElement, 100);
      fit();
      const gone = list.children[3];
      fixture.componentInstance.items.set(['a', 'b', 'sep', 'd']);
      fixture.detectChanges();
      fit();
      expect(observed).not.toContain(gone);
    });
  });
});
