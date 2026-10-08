import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  type OnChanges,
  type SimpleChanges,
  afterNextRender,
  computed,
  inject,
  input,
  model,
  output,
  signal,
  viewChildren,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { GalleryImage, GalleryUsage } from '../../../gen/meurpg/maps/v1/gallery_pb';
import { describeConnectError } from '../../core/connect/connect-errors';
import { GalleryClient } from '../../core/images/gallery-client';
import { formatBytes, formatDimensions } from '../../core/images/image-format';
import { ImageUploader } from '../../core/images/image-uploader';
import { ACCEPT_ATTRIBUTE, DEFAULT_LIMITS } from '../../core/images/upload-errors';
import { UploadQueue } from '../../core/images/upload-queue';
import { ImagePrivacyNote } from './image-privacy-note';
import { RetryImage } from '../retry-image/retry-image';
import { UploadProgress } from './upload-progress/upload-progress';

/** A tag under a tile's name. */
export interface PickerTag {
  readonly icon: string;
  readonly text: string;
}

type PickerState =
  { status: 'loading' } | { status: 'error'; message: string } | { status: 'ready' };

/**
 * Picks one image from a campaign's gallery (E5-31, "Imagem"): the map form
 * (5.3), the document editor's "Imagem da galeria" (5.4) and "Mostrar
 * imagem" (MR-028) all use it.
 *
 * ```html
 * <app-gallery-picker
 *   [campaignId]="campaignId"
 *   [(selectedId)]="imageId"
 *   label="Imagem do mapa"
 *   [describedBy]="imageError ? 'map-image-error' : null"
 *   (picked)="image = $event"
 * />
 * ```
 *
 * - `campaignId` (required): whose gallery. The master's only; the server
 *   refuses it to a player.
 * - `selectedId` (two-way): the chosen image's id, or `null`. Setting it
 *   from outside (an existing map's image) checks that tile.
 * - `label`: the radio group's accessible name (default "Imagem da
 *   galeria"). The visible label ("Imagem") belongs to the form around it.
 * - `describedBy`: the id of the form's error or hint for this field, if
 *   any ("Escolha uma imagem para o mapa.").
 * - `variant` and `tags`: the dialog's bigger tiles and the tags under
 *   their names (MR-028).
 * - `picked`: the chosen `GalleryImage`, whenever the choice changes (a
 *   click, the arrow keys, or an upload that finished).
 *
 * The tiles are a `radiogroup` (the WAI-ARIA radio pattern): Tab enters on
 * the checked tile, the arrows (and Home/End) move and check, and the
 * checked tile has the 2px accent border, the accent-soft footer and a
 * check. The "Enviar imagem" tile runs the gallery's own upload (one file,
 * the same client-side checks and messages), and checks the new image when
 * it is ready.
 */
@Component({
  selector: 'app-gallery-picker',
  imports: [ImagePrivacyNote, MatButtonModule, MatIconModule, RetryImage, UploadProgress],
  templateUrl: './gallery-picker.html',
  styleUrl: './gallery-picker.scss',
})
export class GalleryPicker implements OnChanges {
  private readonly gallery = inject(GalleryClient);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly selectedId = model<string | null>(null);
  readonly label = input('Imagem da galeria');
  readonly describedBy = input<string | null>(null);
  /** `dialog` is the bigger grid of "Mostrar uma imagem aos jogadores"
   * (E5-10): the name only under each tile, no dimensions. */
  readonly variant = input<'form' | 'dialog'>('form');
  /** A tag under a tile's name, by image ID ("Fundo de mapa escondido",
   * "À mostra agora"): an icon and words. */
  readonly tags = input<ReadonlyMap<string, PickerTag>>(new Map());
  /** Images left out of the grid (the portraits of NPCs the players do not see, for a reference to a picture made from a map). */
  readonly excluded = input<ReadonlySet<string>>(new Set());
  readonly picked = output<GalleryImage>();
  /** The gallery's images arrived (a dialog moves focus to the tiles then). */
  readonly loaded = output<readonly GalleryImage[]>();

  protected readonly state = signal<PickerState>({ status: 'loading' });
  protected readonly images = signal<readonly GalleryImage[]>([]);
  private readonly usage = signal<GalleryUsage | null>(null);
  protected readonly announcement = signal('');
  protected readonly accept = ACCEPT_ATTRIBUTE;
  protected readonly dimensions = formatDimensions;
  protected readonly hint = computed(
    () =>
      `JPEG, PNG ou WebP, até\u00a0${formatBytes(this.usage()?.maxImageBytes ?? DEFAULT_LIMITS.maxImageBytes)}`,
  );

  /** The one tile Tab lands on: the checked one, else the first. */
  protected readonly tabStop = computed(() => {
    const index = this.images().findIndex((image) => image.id === this.selectedId());
    return Math.max(0, index);
  });

  protected readonly queue = new UploadQueue({
    uploader: inject(ImageUploader),
    campaignId: () => this.campaignId(),
    usage: () => this.usage(),
    uploaded: (image) => this.onUploaded(image),
  });

  private readonly tiles = viewChildren<ElementRef<HTMLButtonElement>>('tile');

  constructor() {
    inject(DestroyRef).onDestroy(() => this.queue.destroy());
  }

  // (Re)load whenever the campaign changes.
  ngOnChanges(changes: SimpleChanges): void {
    if (changes['campaignId']) {
      this.load(this.campaignId());
    }
  }

  /** Goes up on every load: an answer that comes after a newer load was asked is not drawn. */
  private loadSeq = 0;

  protected load(campaignId = this.campaignId()): void {
    const seq = ++this.loadSeq;
    this.state.set({ status: 'loading' });
    this.gallery.list(campaignId).then(
      ({ images: all, usage }) => {
        if (seq !== this.loadSeq) {
          return;
        }
        const images = all.filter((i) => !this.excluded().has(i.id));
        this.images.set(images);
        this.usage.set(usage);
        this.state.set({ status: 'ready' });
        this.loaded.emit(images);
      },
      (err: unknown) => {
        if (seq === this.loadSeq) {
          this.state.set({ status: 'error', message: describeConnectError(err, {}) });
        }
      },
    );
  }

  protected choose(image: GalleryImage): void {
    if (this.selectedId() !== image.id) {
      this.selectedId.set(image.id);
      this.picked.emit(image);
    }
  }

  /** The radio pattern's keys: arrows (both axes, wrapping) and Home/End
   * move focus and check the tile they land on. */
  protected onKeydown(event: KeyboardEvent, index: number): void {
    const count = this.images().length;
    let target: number;
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        target = (index + 1) % count;
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        target = (index - 1 + count) % count;
        break;
      case 'Home':
        target = 0;
        break;
      case 'End':
        target = count - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    this.choose(this.images()[target]);
    this.tiles()[target]?.nativeElement.focus();
  }

  protected onFileChange(input: HTMLInputElement): void {
    const files = Array.from(input.files ?? []);
    input.value = '';
    if (files.length > 0) {
      this.queue.add(files.slice(0, 1));
    }
  }

  private onUploaded(image: GalleryImage): void {
    this.images.update((list) => [image, ...list.filter((i) => i.id !== image.id)]);
    this.usage.update(
      (usage) =>
        usage && {
          ...usage,
          imageCount: usage.imageCount + 1,
          byteCount: usage.byteCount + image.byteSize,
        },
    );
    this.selectedId.set(image.id);
    this.picked.emit(image);
    this.announcement.set('');
    afterNextRender(() => this.announcement.set(`Imagem enviada e escolhida: ${image.name}.`), {
      injector: this.injector,
    });
  }
}
