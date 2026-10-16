import { Component, Signal, effect, inject } from '@angular/core';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import type { ShownImageVm } from '../../../core/play/live-session.types';

/**
 * "Ver em tela cheia" (E5-12, E5-13): the gallery lightbox in a read-only
 * mode. The name is the title, the image is `contain` on `ground`, and
 * there is "Fechar": no previous or next, no "Renomear", no "Apagar". It
 * follows the live state: a swap updates it in place, and when the master
 * stops showing the image it closes.
 */
@Component({
  selector: 'app-shown-image-viewer',
  imports: [MatIconModule],
  template: `
    @if (image(); as img) {
      <div class="viewer" role="group" aria-labelledby="viewer-title">
        <header class="viewer__head">
          <h2 class="viewer__title" id="viewer-title">{{ img.name }}</h2>
          <button type="button" class="viewer__close" aria-label="Fechar" (click)="ref.close()">
            <mat-icon aria-hidden="true">close</mat-icon>
          </button>
        </header>
        <div class="viewer__stage">
          <img [src]="img.url" [alt]="img.name" [width]="img.width" [height]="img.height" />
        </div>
      </div>
    }
  `,
  styles: `
    :host {
      display: block;
      height: 100%;
    }

    .viewer {
      display: flex;
      flex-direction: column;
      height: 100%;
      background: var(--mr-surface);
      color: var(--mr-ink);
    }

    .viewer__head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--mr-space-3);
      padding: var(--mr-space-3) var(--mr-space-3) var(--mr-space-3) var(--mr-space-5);
    }

    .viewer__title {
      margin: 0;
      font-family: var(--mr-font-display);
      font-size: 24px;
      font-weight: 700;
      line-height: 30px;
      overflow-wrap: anywhere;
    }

    .viewer__close {
      display: flex;
      flex: none;
      align-items: center;
      justify-content: center;
      width: 44px;
      height: 44px;
      padding: 0;
      border: 0;
      border-radius: var(--mr-radius-sm);
      background: none;
      color: var(--mr-ink);
      cursor: pointer;
    }

    .viewer__stage {
      display: flex;
      flex: 1 1 auto;
      align-items: center;
      justify-content: center;
      min-height: 0;
      background: var(--mr-ground);

      img {
        display: block;
        // The width and height attributes keep the room reserved; the picture fills the stage, whole (contain), also
        // when the image is smaller than the window.
        width: 100%;
        height: 100%;
        object-fit: contain;
        // Pinch zoom on a phone is the browser's own.
        touch-action: pinch-zoom;
      }
    }
  `,
})
export class ShownImageViewer {
  protected readonly image = inject<Signal<ShownImageVm | null>>(MAT_DIALOG_DATA);
  protected readonly ref = inject<MatDialogRef<ShownImageViewer>>(MatDialogRef);

  constructor() {
    // The master stopped showing it: nothing left to look at.
    effect(() => {
      if (this.image() === null) {
        this.ref.close('stopped');
      }
    });
  }
}
