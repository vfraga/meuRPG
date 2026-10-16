import { OverlayContainer } from '@angular/cdk/overlay';
import { DOCUMENT } from '@angular/common';
import { DestroyRef, Injectable, inject } from '@angular/core';

/**
 * Makes the app's dialogs and sheets follow a browser full screen: the browser paints only the full-screen element's
 * subtree, so while a map is on full screen the CDK overlay container moves into it, and back to the body when it ends.
 *
 * It does what the CDK's `FullscreenOverlayContainer` does, but as a service the pages that can go full screen
 * (`fullscreenOfHost`) inject, instead of a replacement of `OverlayContainer` in `app.config.ts`: that provider needs the
 * CDK overlay in the initial bundle (about 58 kB, on every first screen), and only a map page needs it.
 */
@Injectable({ providedIn: 'root' })
export class FullscreenOverlay {
  private readonly container = inject(OverlayContainer);
  private readonly doc = inject(DOCUMENT);

  constructor() {
    const follow = () =>
      (this.doc.fullscreenElement ?? this.doc.body).appendChild(
        this.container.getContainerElement(),
      );
    this.doc.addEventListener('fullscreenchange', follow);
    inject(DestroyRef).onDestroy(() => this.doc.removeEventListener('fullscreenchange', follow));
  }
}
