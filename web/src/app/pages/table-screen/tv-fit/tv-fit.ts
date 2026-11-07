import {
  DestroyRef,
  Directive,
  ElementRef,
  Injector,
  afterNextRender,
  effect,
  inject,
  input,
  output,
} from '@angular/core';

/** Pixels a row may go past the host's edge (sub-pixel rounding) and still count as fitting. */
const TOLERANCE = 2;

/**
 * Draws only the children of a list that fit in its height, so a TV never shows a row cut in half and never scrolls.
 * The host must have a height of its own (a flex or grid cell with `min-height: 0` and `overflow: hidden`); where it
 * has none (a phone, one column of natural height) every child fits and nothing is hidden. A child with a
 * `data-count` attribute is one the `fitted` output counts when it is left out, for the "+N".
 *
 * `appTvFit` takes whatever changes with the content (the list the template draws): the fit is made again after the
 * next render, and again whenever the host's size changes.
 */
@Directive({ selector: '[appTvFit]' })
export class TvFit {
  readonly content = input<unknown>(null, { alias: 'appTvFit' });
  /** How many counted children are left out. */
  readonly fitted = output<number>();

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement;
  private readonly injector = inject(Injector);
  private last = -1;
  private observer?: ResizeObserver;
  private watched: HTMLElement[] = [];

  constructor() {
    effect(() => {
      this.content();
      afterNextRender(() => this.fit(), { injector: this.injector });
    });
    if (typeof ResizeObserver !== 'undefined') {
      const observer = new ResizeObserver(() => this.fit());
      this.observer = observer;
      observer.observe(this.host);
      inject(DestroyRef).onDestroy(() => observer.disconnect());
    }
    // A web font that arrives late changes every row's height: the fit is made again once the fonts are in.
    const fonts = typeof document === 'undefined' ? undefined : document.fonts;
    if (fonts) {
      const refit = (): void => this.fit();
      void fonts.ready.then(refit);
      fonts.addEventListener?.('loadingdone', refit);
      inject(DestroyRef).onDestroy(() => fonts.removeEventListener?.('loadingdone', refit));
    }
  }

  /** Shows every child, then hides, from the first one that goes past the host's bottom edge, that one and the rest; the very first child is never hidden. */
  fit(): void {
    const children = [...this.host.children] as HTMLElement[];
    this.watch(children);
    children.forEach((child) => child.style.removeProperty('display'));
    const limit = this.host.getBoundingClientRect().bottom;
    let left = 0;
    if (this.host.clientHeight > 0) {
      let past = false;
      for (const [at, child] of children.entries()) {
        past = past || child.getBoundingClientRect().bottom > limit + TOLERANCE;
        // The first child stays even when it does not fit: a TV never shows an empty list (the current turn's row).
        if (past && at > 0) {
          child.style.display = 'none';
          left += child.dataset['count'] === undefined ? 0 : 1;
        }
      }
    }
    if (left !== this.last) {
      this.last = left;
      this.fitted.emit(left);
    }
  }

  /** Watches each child's size too: a row that grows or a "+N" slot that changes makes the fit again. */
  private watch(children: HTMLElement[]): void {
    if (!this.observer) {
      return;
    }
    for (const child of this.watched) {
      if (!children.includes(child)) {
        this.observer.unobserve(child);
      }
    }
    for (const child of children) {
      if (!this.watched.includes(child)) {
        this.observer.observe(child);
      }
    }
    this.watched = children;
  }
}
