import { Component, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';

import type { ShownImageVm } from '../../../core/play/live-session.types';

/**
 * "Deixadas com os jogadores" (E6-25, MR-028): the images the master left
 * with the players, inside the "Imagem para os jogadores" panel. A row each:
 * a 64 x 48 thumbnail, the name and "Tirar" (named after the image), which
 * acts at once: the image stays in the gallery, and "Mostrar imagem" brings
 * it back. It lists only; the panel calls the server and moves focus.
 */
@Component({
  selector: 'app-left-images-list',
  imports: [MatButtonModule],
  template: `
    @if (images().length > 0) {
      <section class="left" aria-labelledby="left-heading">
        <h3 class="left__title" id="left-heading">Deixadas com os jogadores</h3>
        <p class="left__note">Os jogadores veem estas imagens até você tirar.</p>
        <ul class="left__list">
          @for (image of images(); track image.id) {
            <li class="left__row">
              <img
                class="left__thumb"
                [src]="image.url + '/thumb'"
                alt=""
                width="64"
                height="48"
                loading="lazy"
              />
              <span class="left__name">{{ image.name }}</span>
              <button
                matButton="outlined"
                type="button"
                class="left__take"
                [attr.data-image-id]="image.id"
                [attr.aria-label]="'Tirar ' + image.name + ' dos jogadores'"
                (click)="take.emit(image)"
              >
                Tirar
              </button>
            </li>
          }
        </ul>
      </section>
    }
  `,
  styles: `
    .left {
      display: flex;
      flex-direction: column;
      gap: 4px;
      padding-top: 4px;
    }

    .left__title {
      margin: 0;
      font-family: var(--mr-font-display);
      font-size: 18px;
      font-weight: 700;
      line-height: 22px;
    }

    .left__note {
      margin: 0;
      font-size: 14px;
      line-height: 19px;
      color: var(--mr-ink-muted);
    }

    .left__list {
      margin: 0;
      padding: 0;
      list-style: none;
    }

    .left__row {
      display: flex;
      align-items: center;
      gap: var(--mr-space-3);
      min-height: 64px;
      padding: 8px 0;
    }

    .left__thumb {
      flex: none;
      width: 64px;
      height: 48px;
      border-radius: var(--mr-radius-sm);
      outline: 1px solid var(--mr-line);
      outline-offset: -1px;
      object-fit: cover;
      background: var(--mr-ground);
    }

    .left__name {
      flex: 1 1 auto;
      min-width: 0;
      font-weight: 700;
      overflow-wrap: anywhere;
    }

    .left__take {
      flex: none;
      min-height: 44px;
    }
  `,
})
export class LeftImagesList {
  readonly images = input<readonly ShownImageVm[]>([]);
  /** "Tirar": the master takes this image back. */
  readonly take = output<ShownImageVm>();
}
