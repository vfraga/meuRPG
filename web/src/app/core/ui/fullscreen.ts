import { DOCUMENT } from '@angular/common';
import { DestroyRef, ElementRef, inject, signal } from '@angular/core';

import { FullscreenOverlay } from './fullscreen-overlay';

/** The state and the toggle of one element's browser full screen. */
export interface FullscreenControl {
  /** Whether the browser has the Fullscreen API (iPhone Safari does not): the button is left out when not. */
  readonly available: boolean;
  /** Whether this element is the one on full screen right now, however the mode started or ended (Esc included). */
  readonly active: () => boolean;
  /** Enters full screen on the element, or leaves it. */
  toggle(): void;
}

/**
 * Full screen for the host element of the component that calls it (call it in an injection context, e.g. a field
 * initializer). The browser shows only the full-screen element's subtree, so the whole map and its controls go in;
 * the app's dialogs and sheets follow through `FullscreenOverlay`. The state comes from
 * `fullscreenchange`, never from the click, so the icon is right when Esc ends the mode.
 */
export function fullscreenOfHost(): FullscreenControl {
  const doc = inject(DOCUMENT);
  // Starts the service that moves the dialogs' container into the full-screen element (see its comment).
  inject(FullscreenOverlay);
  const host = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement;
  const active = signal(false);
  const sync = () => active.set(doc.fullscreenElement === host);
  doc.addEventListener('fullscreenchange', sync);
  inject(DestroyRef).onDestroy(() => {
    doc.removeEventListener('fullscreenchange', sync);
    if (doc.fullscreenElement === host) {
      void doc.exitFullscreen?.()?.catch(() => undefined);
    }
  });
  return {
    available: doc.fullscreenEnabled === true,
    active: active.asReadonly(),
    toggle: () => {
      const request =
        doc.fullscreenElement === host ? doc.exitFullscreen() : host.requestFullscreen();
      // A refusal (a policy, a lost user gesture) leaves the page as it was.
      request?.catch(() => undefined);
    },
  };
}
