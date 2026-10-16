import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { SceneImage } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { tight } from '../../../../core/format/text';
import { RetryImage } from '../../../../shared/retry-image/retry-image';
import { LiveSessionSource, ShownImageVm } from '../../../../core/play/live-session.types';
import { showErrorMessage } from '../../shown-image-panel/shown-image-panel';

/**
 * "Imagens da cena" in the master's open scene (MR-015, MR-019): the images the
 * master prepared on the scene point, each with "Mostrar aos jogadores", which is
 * the gallery's own action (`SetShownImage`): it replaces the image on display
 * (or leaves it with the players when "Deixar com os jogadores" is on) and
 * follows the same rules. The one on display is marked "À mostra agora" and
 * offers "Parar de mostrar". An image that shows the whole map asks first, in
 * place, as the gallery picker does. The list is the master's; the players only
 * ever receive the image shown.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-scene-images-show',
  imports: [MatButtonModule, MatIconModule, RetryImage],
  templateUrl: './scene-images.html',
  styleUrl: './scene-images.scss',
})
export class SceneImagesShow {
  private readonly source = inject(LiveSessionSource);

  readonly campaignId = input.required<string>();
  readonly images = input.required<readonly SceneImage[]>();
  /** The image on the players' screens now, if any. */
  readonly shown = input<ShownImageVm | null>(null);
  /** The master's "Deixar com os jogadores" switch: the image replaced stays with the players. */
  readonly keep = input(false);
  /** The server accepted a change: the page shows it at once. */
  readonly changed = output<ShownImageVm | null>();
  /** An image was left with the players: the page reads that list again. */
  readonly leftChanged = output<void>();

  protected readonly count = computed(() => tight(`${this.images().length} de 8`));
  protected readonly busyId = signal<string | null>(null);
  protected readonly askingId = signal<string | null>(null);
  protected readonly error = signal('');
  protected readonly status = signal('');

  protected thumb(image: SceneImage): string {
    return `/images/${image.id}/thumb`;
  }

  protected isShown(image: SceneImage): boolean {
    return this.shown()?.id === image.id;
  }

  protected async show(image: SceneImage, confirmed = false): Promise<void> {
    if (this.busyId() !== null) {
      return;
    }
    if (image.showsWholeMap && !confirmed) {
      this.error.set('');
      this.askingId.set(image.id);
      return;
    }
    const leaving = this.keep() ? this.shown() : null;
    this.busyId.set(image.id);
    this.error.set('');
    try {
      const result = await this.source.setShownImage(this.campaignId(), image.id);
      this.askingId.set(null);
      this.changed.emit(result);
      if (leaving) {
        this.leftChanged.emit();
      }
      this.status.set(`${image.name} está na tela dos jogadores.`);
    } catch (err) {
      this.askingId.set(null);
      this.error.set(showErrorMessage(err));
    } finally {
      this.busyId.set(null);
    }
  }

  protected async stop(image: SceneImage): Promise<void> {
    if (this.busyId() !== null) {
      return;
    }
    const leaving = this.keep() ? this.shown() : null;
    this.busyId.set(image.id);
    this.error.set('');
    try {
      await this.source.setShownImage(this.campaignId(), null);
      this.changed.emit(null);
      if (leaving) {
        this.leftChanged.emit();
      }
      this.status.set(
        leaving
          ? `${leaving.name} continua com os jogadores.`
          : 'Imagem retirada da tela dos jogadores.',
      );
    } catch (err) {
      this.error.set(showErrorMessage(err));
    } finally {
      this.busyId.set(null);
    }
  }

  protected cancelAsk(): void {
    this.askingId.set(null);
  }
}
