import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { Code, ConnectError } from '@connectrpc/connect';

import type { Map as MapMessage } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { describeConnectError } from '../../../core/connect/connect-errors';
import { GalleryClient } from '../../../core/images/gallery-client';
import { openImagePicker } from '../../../shared/gallery-picker/image-picker-dialog/image-picker-dialog';
import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import { LeftImagesList } from '../left-images-list/left-images-list';
import { LiveSessionSource, ShownImageVm } from '../live-session.types';
import { KeepSwitch } from './keep-switch';

/** What the dialog's line says under the grid (E5-10). */
export function shownImageNote(
  image: { name: string } | null,
  current: { name: string } | null,
): string {
  return current && image
    ? `Os jogadores passam a ver ${image.name} no lugar de ${current.name}.`
    : 'Os jogadores veem a imagem na hora, com o nome dela como legenda. O mapa atual continua na tela deles.';
}

/** The images that are the background of a hidden map, with the map's
 * name: showing one reveals the image and its name, not the map. */
export function hiddenMapImages(maps: readonly MapMessage[]): Map<string, string> {
  const hidden = new Map<string, string>();
  for (const map of maps) {
    if (!map.revealed && map.image) {
      hidden.set(map.image.id, map.name);
    }
  }
  return hidden;
}

/**
 * The master's "Imagem para os jogadores" panel on the session page (E5-10,
 * E5-11, MR-028): show a gallery image as a handout, apart from the current
 * map.
 *
 * - Empty: "Nenhuma imagem à mostra." and an outlined "Mostrar imagem" that
 *   opens the picker. With an empty gallery the text says to upload one.
 * - Showing: an accent frame, the thumbnail, "Mostrando agora", the image's
 *   name and two outlined buttons: "Parar de mostrar" (at once, no confirm:
 *   showing it again undoes it) and "Trocar imagem".
 * - No filled button on the page: the filled "Mostrar aos jogadores" is the
 *   dialog's.
 * - "Deixar com os jogadores" (E6-25): a switch on the image shown. On,
 *   "Parar de mostrar" and "Trocar imagem" move the image to "Deixadas com
 *   os jogadores", where "Tirar" takes it back at once. The server keeps the
 *   switch and the list; this panel only asks and shows them.
 * - Each change is announced (`role="status"`) and focus follows: to
 *   "Mostrar imagem" after stopping, to "Parar de mostrar" after showing
 *   (the button that opened the dialog no longer exists).
 */
@Component({
  selector: 'app-shown-image-panel',
  imports: [MatButtonModule, MatIconModule, KeepSwitch, LeftImagesList],
  templateUrl: './shown-image-panel.html',
  styleUrl: './shown-image-panel.scss',
})
export class ShownImagePanel {
  private readonly source = inject(LiveSessionSource);
  private readonly gallery = inject(GalleryClient);
  private readonly dialog = inject(MatDialog);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly campaignId = input.required<string>();
  readonly shown = input<ShownImageVm | null>(null);
  /** The campaign's maps, to tag the images that back a hidden map. */
  readonly maps = input<readonly MapMessage[]>([]);
  /** The switch of the image shown, and the images left with the players. */
  readonly keep = input(false);
  readonly left = input<readonly ShownImageVm[]>([]);
  /** The server accepted a change: the page shows it at once. */
  readonly changed = output<ShownImageVm | null>();
  /** The server accepted a new state of the switch. */
  readonly keepChange = output<boolean>();
  /** An image left, or was taken back: the page reads the list again. */
  readonly leftChanged = output<void>();
  /** The master took this image back: it leaves the list at once. */
  readonly taken = output<ShownImageVm>();

  protected readonly phone = mediaQuery(PHONE_QUERY);
  protected readonly status = signal('');
  protected readonly error = signal<string | null>(null);
  protected readonly stopping = signal(false);
  protected readonly keeping = signal(false);
  /** The images whose "Tirar" is on its way: a second click on one would find it already gone. */
  private readonly takingBack = new Set<string>();
  protected readonly galleryEmpty = signal(false);
  protected readonly thumb = computed(() => {
    const s = this.shown();
    return s ? `${s.url}/thumb` : '';
  });

  private readonly showButton = viewChild('showButton', { read: ElementRef<HTMLButtonElement> });
  private readonly stopButton = viewChild('stopButton', { read: ElementRef<HTMLButtonElement> });

  constructor() {
    afterNextRender(() => {
      this.gallery.list(this.campaignId()).then(
        ({ images }) => this.galleryEmpty.set(images.length === 0),
        () => undefined,
      );
    });
  }

  protected open(): void {
    const shown = this.shown();
    const ref = openImagePicker(
      this.dialog,
      {
        campaignId: this.campaignId(),
        title: 'Mostrar uma imagem aos jogadores',
        confirmLabel: 'Mostrar aos jogadores',
        confirmIcon: 'cast',
        current: shown ? { id: shown.id, name: shown.name } : null,
        currentTag: 'À mostra agora',
        currentNote: 'Essa imagem já está à mostra.',
        note: shownImageNote,
        hiddenMapImages: hiddenMapImages(this.maps()),
        emptyError: 'Escolha uma imagem para mostrar.',
        wholeMapConfirmLabel: 'Mostrar mesmo assim',
        submit: async (image) => {
          const leaving = this.keep() ? shown : null;
          const result = await this.source.setShownImage(this.campaignId(), image.id);
          this.changed.emit(result);
          if (leaving) {
            this.leftChanged.emit();
          }
          this.status.set(
            `${leaving ? `${leaving.name} continua com os jogadores. ` : ''}${image.name} está na tela dos jogadores.`,
          );
        },
        errorMessage: showErrorMessage,
      },
      this.injector,
      this.phone(),
      // The button that opened the dialog may be gone when it closes.
      false,
    );
    ref.afterClosed().subscribe((done) => {
      afterNextRender(
        () => {
          if (done) {
            this.stopButton()?.nativeElement.focus();
          } else {
            (this.showButton() ?? this.stopButton())?.nativeElement.focus();
          }
        },
        { injector: this.injector },
      );
    });
  }

  protected async stop(): Promise<void> {
    if (this.stopping() || this.keeping()) {
      return;
    }
    const leaving = this.keep() ? this.shown() : null;
    this.stopping.set(true);
    this.error.set(null);
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
      afterNextRender(() => this.showButton()?.nativeElement.focus(), { injector: this.injector });
    } catch (err) {
      this.error.set(showErrorMessage(err));
    } finally {
      this.stopping.set(false);
    }
  }

  /** The switch: asks for the new state with the image already shown. */
  protected async setKeep(next: boolean): Promise<void> {
    const shown = this.shown();
    // The server has no "keep only": a switch flipped during a stop would show the withdrawn image again.
    if (!shown || this.keeping() || this.stopping()) {
      return;
    }
    this.keeping.set(true);
    this.error.set(null);
    try {
      await this.source.setShownImage(this.campaignId(), shown.id, next);
      this.keepChange.emit(next);
    } catch (err) {
      this.error.set(showErrorMessage(err));
    } finally {
      this.keeping.set(false);
    }
  }

  /** "Tirar": at once, no confirmation (showing the image again undoes it).
   * Focus goes to the next row's "Tirar", or the one before, or the panel's
   * main button when the list empties. */
  protected async takeBack(image: ShownImageVm): Promise<void> {
    if (this.takingBack.has(image.id)) {
      return;
    }
    this.takingBack.add(image.id);
    const ids = this.left().map((i) => i.id);
    const at = ids.indexOf(image.id);
    const neighbour = ids[at + 1] ?? ids[at - 1];
    this.error.set(null);
    try {
      await this.source.takeBackLeftImage(this.campaignId(), image.id);
      this.taken.emit(image);
      this.status.set(`${image.name} foi tirada.`);
    } catch (err) {
      if (ConnectError.from(err).code === Code.NotFound) {
        // Taken back in another tab: the list is stale, read it again.
        this.leftChanged.emit();
      }
      this.error.set(takeBackErrorMessage(err));
      return;
    } finally {
      this.takingBack.delete(image.id);
    }
    afterNextRender(
      () => {
        const next = neighbour
          ? this.host.nativeElement.querySelector<HTMLElement>(`[data-image-id="${neighbour}"]`)
          : (this.showButton() ?? this.stopButton())?.nativeElement;
        next?.focus();
      },
      { injector: this.injector },
    );
  }
}

/** TakeBackLeftImage's errors, in Portuguese (play.proto). */
export function takeBackErrorMessage(err: unknown): string {
  return describeConnectError(err, {
    [Code.NotFound]: 'Essa imagem já não estava com os jogadores.',
    [Code.PermissionDenied]: 'Só o mestre da campanha tira imagens dos jogadores.',
  });
}

/** SetShownImage's errors, in Portuguese (play.proto). */
export function showErrorMessage(err: unknown): string {
  if (ConnectError.from(err).code === Code.FailedPrecondition) {
    return 'A sessão acabou: as imagens só são mostradas durante a sessão.';
  }
  return describeConnectError(err, {
    [Code.NotFound]: 'Essa imagem não está mais na galeria. Escolha outra.',
    [Code.PermissionDenied]: 'Só o mestre da campanha mostra imagens.',
    [Code.ResourceExhausted]:
      'A galeria da campanha está cheia: apague uma imagem para mostrar o fundo deste mapa.',
  });
}
