import { ElementRef, Injector, afterNextRender } from '@angular/core';

/** The questions the page asks in place, and the button each one puts the focus on first. */
export type LeaveFrom = 'top' | 'foot';

/**
 * Where the focus and the scroll go as the level-up page changes: after a render, on the step, the checked class, the first
 * missing choice or the safe button of a question. The page decides when; this knows which element. It also keeps the
 * `--foot-h` of the fixed footer on a phone.
 */
export class LevelUpFocus {
  private footObserver: ResizeObserver | undefined;

  constructor(
    private readonly host: ElementRef<HTMLElement>,
    private readonly injector: Injector,
  ) {}

  private root(): HTMLElement {
    return this.host.nativeElement;
  }

  private after(run: () => void): void {
    afterNextRender(run, { injector: this.injector });
  }

  private find<T extends HTMLElement>(selector: string): T | null {
    return this.root().querySelector<T>(selector);
  }

  /** The first screen is up: the footer is watched and the focus is on the checked class card. */
  firstScreen(): void {
    this.after(() => {
      this.watchFoot();
      // The class step opens first: the focus is on the checked card.
      this.find<HTMLInputElement>('app-class-pick input:checked')?.focus({ preventScroll: true });
    });
  }

  disconnect(): void {
    this.footObserver?.disconnect();
  }

  /** The footer is fixed on a phone: the page keeps its height free at the bottom, whatever it holds (the reason, the
   * question, the stacked buttons), so nothing of the step is hidden behind it. */
  private watchFoot(): void {
    const foot = this.find('.foot');
    if (!foot || typeof ResizeObserver === 'undefined') {
      return;
    }
    this.footObserver?.disconnect();
    this.footObserver = new ResizeObserver(() =>
      this.root().style.setProperty('--foot-h', `${foot.offsetHeight}px`),
    );
    this.footObserver.observe(foot);
  }

  /** A new step: the page is at the top, with the focus on the step. */
  step(): void {
    this.after(() => {
      window.scrollTo({ top: 0 });
      this.find('.js-step')?.focus({ preventScroll: true });
    });
  }

  /** The first choice still missing is marked, comes into view between the bars and gets a visible focus. */
  missing(firstId: string | undefined): void {
    this.after(() => {
      this.clearAttention();
      const root = this.root();
      const card = firstId
        ? root.querySelector<HTMLElement>(`#pick-${firstId}`)
        : root.querySelector<HTMLElement>('.body section');
      // A half-done swap: the focus goes to the group that is half done.
      const target =
        card?.querySelector<HTMLElement>('[data-half] input:not(:disabled)') ??
        card?.querySelector<HTMLElement>(
          '.rows input:not(:disabled), input:not(:disabled), [data-pending] .choice__title, button',
        );
      card?.setAttribute('data-attn', '');
      target?.scrollIntoView({ block: 'center' });
      target?.focus({ preventScroll: true, focusVisible: true } as FocusOptions);
    });
  }

  /** The mark on a card goes when the player picks anything, or leaves the step. */
  clearAttention(): void {
    this.root()
      .querySelectorAll('[data-attn]')
      .forEach((el) => el.removeAttribute('data-attn'));
  }

  /** A question opened in place: its safe button comes into view and takes the focus. */
  question(selector: '.js-keep' | '.js-new-keep' | '.js-class-keep'): void {
    this.after(() => {
      const keep = this.find(selector);
      keep?.scrollIntoView({ block: 'center' });
      keep?.focus({ preventScroll: true });
    });
  }

  /** "Continuar escolhendo": the focus goes back to the control that asked. */
  afterKeepChoosing(from: LeaveFrom | null): void {
    this.after(() =>
      this.find(from === 'top' ? '.js-leave-top' : '.js-leave-foot, .js-next')?.focus(),
    );
  }

  /** "Voltar" of the new-class question: the footer's "Próximo" has the focus again. */
  nextButton(): void {
    this.after(() => this.find('.js-next')?.focus());
  }

  /** The failure is shown in place and comes into view. */
  failure(): void {
    this.after(() => this.find('.js-failure')?.scrollIntoView({ block: 'center' }));
  }

  /** The focus returns to the checked class card. */
  checkedClass(): void {
    this.after(() =>
      this.find<HTMLInputElement>('app-class-pick input:checked')?.focus({ preventScroll: true }),
    );
  }
}
