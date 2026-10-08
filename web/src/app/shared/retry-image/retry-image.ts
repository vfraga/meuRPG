import { DestroyRef, Directive, ElementRef, inject } from '@angular/core';

const MS_PER_SECOND = 1000;
/** An image the server did not give (it throttles with a 429 or a 503; an `<img>` cannot read the `Retry-After`) is asked
 * for again after 2, 4, 8, 16 and 30 s, the way the fog tiles are; after the last attempt it stays as the browser shows it. */
const RETRIES = 5;
const RETRY_CAP_SECONDS = 30;
const RETRY_PARAM = 'retry';

/** The address without the `retry` we added. */
function baseOf(src: string): string {
  return src.replace(new RegExp(`[?&]${RETRY_PARAM}=\\d+$`), '');
}

/**
 * Asks again for an `<img>` that failed to load, with a growing wait (`<img [src]="url" appRetryImage />`). The new
 * address carries `retry=N`, so the browser does not answer from the failed attempt; a new `src` from the screen starts
 * over. Leave it off an image whose failure the screen already handles with its own fallback.
 */
@Directive({
  selector: 'img[appRetryImage]',
  host: { '(error)': 'failed()' },
})
export class RetryImage {
  private readonly img = inject<ElementRef<HTMLImageElement>>(ElementRef).nativeElement;
  private base = '';
  private attempt = 0;
  private timer: ReturnType<typeof setTimeout> | null = null;

  constructor() {
    inject(DestroyRef).onDestroy(() => this.cancel());
  }

  protected failed(): void {
    const current = this.img.getAttribute('src') ?? '';
    const base = baseOf(current);
    if (base !== this.base) {
      // Another picture than the one we were retrying: its first failure.
      this.base = base;
      this.attempt = 0;
      this.cancel();
    }
    if (this.attempt >= RETRIES || this.timer !== null || base === '') {
      return;
    }
    const attempt = ++this.attempt;
    this.timer = setTimeout(
      () => {
        this.timer = null;
        this.img.setAttribute(
          'src',
          `${base}${base.includes('?') ? '&' : '?'}${RETRY_PARAM}=${attempt}`,
        );
      },
      Math.min(2 * 2 ** (attempt - 1), RETRY_CAP_SECONDS) * MS_PER_SECOND,
    );
  }

  private cancel(): void {
    if (this.timer !== null) {
      clearTimeout(this.timer);
      this.timer = null;
    }
  }
}
