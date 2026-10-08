import {
  Component,
  DestroyRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  signal,
  viewChild,
  viewChildren,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import type { GalleryImage, GalleryUsage } from '../../../gen/meurpg/maps/v1/gallery_pb';
import { describeConnectError } from '../../core/connect/connect-errors';
import { GalleryClient } from '../../core/images/gallery-client';
import { ImageUploader } from '../../core/images/image-uploader';
import { quotaNearlyFull, usageLine } from '../../core/images/image-format';
import { DEFAULT_LIMITS } from '../../core/images/upload-errors';
import { UploadQueue } from '../../core/images/upload-queue';
import { GenerateImageButton } from '../../shared/image-generate/generate-image-button';
import { UploadProgress } from '../../shared/gallery-picker/upload-progress/upload-progress';
import { GalleryCard } from './gallery-card/gallery-card';
import { GalleryLightbox } from './gallery-lightbox/gallery-lightbox';
import { UploadZone } from './upload-zone/upload-zone';

type PageState =
  | { status: 'loading' }
  | { status: 'not-found' }
  | { status: 'forbidden' }
  | { status: 'error'; message: string }
  | { status: 'ready' };

/** What the lightbox asked for; done once it has closed, so focus lands
 * in the card's field or confirmation instead of going back to the card. */
type AfterLightbox = { kind: 'rename' | 'delete'; imageId: string } | null;

/**
 * "/campaigns/:id/gallery" (MR-019): the master's gallery. E5-20 (desktop),
 * E5-21 (phone) and E5-22 (empty).
 *
 * The images come newest first from `ListGalleryImages`, with the quota
 * (`GalleryUsage`). "Enviar imagem" (or dropping files anywhere on the
 * page) queues the files in an `UploadQueue`, which sends them one after
 * the other, each with its own card and, if it fails, its own notice at the
 * top. A new image joins the start of the grid, and the quota line follows.
 *
 * Only the master may see the gallery (RN-10: it would show a map before
 * it's revealed). A player gets `permission_denied`: a calm "Só o mestre vê
 * a galeria da campanha." A non-member, and a campaign that does not exist,
 * both get `not_found` and the same "campanha não encontrada" (ADR-0011).
 */
@Component({
  selector: 'app-gallery',
  imports: [
    GenerateImageButton,
    GalleryCard,
    GalleryLightbox,
    MatButtonModule,
    MatIconModule,
    MatProgressSpinnerModule,
    RouterLink,
    UploadProgress,
    UploadZone,
  ],
  templateUrl: './gallery.html',
  styleUrl: './gallery.scss',
  host: {
    '(document:dragover)': 'onDragOver($event)',
    '(document:dragleave)': 'onDragLeave($event)',
    '(document:drop)': 'onDrop($event)',
  },
})
export class GalleryPage {
  private readonly gallery = inject(GalleryClient);
  private readonly route = inject(ActivatedRoute);
  private readonly injector = inject(Injector);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);

  protected readonly campaignId = signal('');
  protected readonly state = signal<PageState>({ status: 'loading' });
  protected readonly images = signal<readonly GalleryImage[]>([]);
  protected readonly usage = signal<GalleryUsage | null>(null);
  protected readonly dragging = signal(false);
  /** The image open in the lightbox, by id (an upload that lands while it
   * is open shifts the indexes), or null when it is closed. */
  private readonly viewingId = signal<string | null>(null);
  protected readonly viewing = computed(() => {
    const id = this.viewingId();
    const index = id === null ? -1 : this.images().findIndex((i) => i.id === id);
    return index < 0 ? null : index;
  });
  /** The polite live region's text: "Imagem enviada: …" and the like. */
  protected readonly announcement = signal('');

  protected readonly queue = new UploadQueue({
    uploader: inject(ImageUploader),
    campaignId: () => this.campaignId(),
    usage: () => this.usage(),
    uploaded: (image) => this.onUploaded(image),
  });

  protected readonly isEmpty = computed(
    () => this.images().length === 0 && this.queue.items().length === 0,
  );
  protected readonly usageText = computed(() => {
    const usage = this.usage();
    return usage ? usageLine(usage) : '';
  });
  protected readonly maxImageBytes = computed(
    () => this.usage()?.maxImageBytes ?? DEFAULT_LIMITS.maxImageBytes,
  );
  protected readonly nearlyFull = computed(() => {
    const usage = this.usage();
    return usage ? quotaNearlyFull(usage) : false;
  });

  private readonly cards = viewChildren(GalleryCard);
  private readonly zone = viewChild(UploadZone);
  private afterLightbox: AfterLightbox = null;
  private dragTimer: ReturnType<typeof setTimeout> | undefined;

  constructor() {
    this.route.paramMap.pipe(takeUntilDestroyed()).subscribe((params) => {
      const id = params.get('id');
      if (id) {
        if (id !== this.campaignId()) {
          // Files queued for the campaign left behind must not be sent to this one.
          this.queue.reset();
          this.images.set([]);
          this.usage.set(null);
        }
        this.campaignId.set(id);
        this.load(id);
      }
    });
    inject(DestroyRef).onDestroy(() => {
      this.queue.destroy();
      clearTimeout(this.dragTimer);
    });
  }

  protected load(campaignId = this.campaignId()): void {
    this.state.set({ status: 'loading' });
    this.gallery.list(campaignId).then(
      ({ images, usage }) => {
        if (campaignId !== this.campaignId()) {
          return;
        }
        this.images.set(images);
        this.usage.set(usage);
        this.state.set({ status: 'ready' });
      },
      (err: unknown) => {
        if (campaignId !== this.campaignId()) {
          return;
        }
        const code = ConnectError.from(err, Code.Unavailable).code;
        if (code === Code.NotFound) {
          this.state.set({ status: 'not-found' });
        } else if (code === Code.PermissionDenied) {
          this.state.set({ status: 'forbidden' });
        } else {
          this.state.set({ status: 'error', message: describeConnectError(err, {}) });
        }
      },
    );
  }

  /** Reads the images again without the loading state (a picture was generated: it is in the gallery now). */
  protected refresh(): void {
    const campaignId = this.campaignId();
    this.gallery.list(campaignId).then(
      ({ images, usage }) => {
        if (campaignId !== this.campaignId()) {
          return;
        }
        this.images.set(images);
        this.usage.set(usage);
      },
      () => undefined,
    );
  }

  protected addFiles(files: File[]): void {
    this.queue.add(files);
  }

  protected open(image: GalleryImage): void {
    this.afterLightbox = null;
    this.viewingId.set(image.id);
  }

  protected turn(index: number): void {
    this.viewingId.set(this.images()[index]?.id ?? null);
  }

  protected onRenamed(image: GalleryImage): void {
    this.images.update((list) => list.map((i) => (i.id === image.id ? image : i)));
    this.announce('Nome salvo.');
  }

  protected onDeleted(image: GalleryImage): void {
    const list = this.images();
    const at = list.findIndex((i) => i.id === image.id);
    if (at < 0) {
      return;
    }
    const rest = list.filter((i) => i.id !== image.id);
    this.images.set(rest);
    this.usage.update(
      (usage) =>
        usage && {
          ...usage,
          imageCount: Math.max(0, usage.imageCount - 1),
          byteCount: Math.max(0, usage.byteCount - image.byteSize),
        },
    );
    this.announce(`Imagem apagada: ${image.name}.`);
    // Focus goes to the image that took its place (or the one before it),
    // or to "Enviar imagem" when the gallery is empty now.
    const next = rest[Math.min(at, rest.length - 1)];
    afterNextRender(
      () => {
        const card = next && this.cardFor(next.id);
        if (card) {
          card.focusView();
        } else {
          this.zone()?.focus();
        }
      },
      { injector: this.injector },
    );
  }

  /** "Editada de Imagem 1": the name of the image an adjustment came from, when it is still in the gallery. */
  protected parentNameOf(image: GalleryImage): string {
    return image.parentImageId === ''
      ? ''
      : (this.images().find((i) => i.id === image.parentImageId)?.name ?? '');
  }

  /** "Pedir um ajuste" in the lightbox: it closes, and the generate dialog opens on that image (its chain, "Mostrar aos jogadores", the adjustment). */
  protected async onLightboxAdjust(image: GalleryImage): Promise<void> {
    this.afterLightbox = null;
    this.viewingId.set(null);
    const { openImageGenerate } = await import('../../shared/image-generate/image-generate-dialog');
    openImageGenerate(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      origin: { kind: 'gallery' },
      image,
    }).subscribe((outcome) => {
      if (outcome && outcome.generated > 0) {
        this.refresh();
      }
      this.cardFor(image.id)?.focusView();
    });
  }

  protected onLightboxRename(image: GalleryImage): void {
    this.afterLightbox = { kind: 'rename', imageId: image.id };
    this.viewingId.set(null);
  }

  protected onLightboxDelete(image: GalleryImage): void {
    this.afterLightbox = { kind: 'delete', imageId: image.id };
    this.viewingId.set(null);
  }

  /** The lightbox closed (Esc, "Fechar", the scrim, or one of its actions):
   * focus goes back to the card of the image it was showing. */
  protected onLightboxClosed(): void {
    const shownId = this.viewingId();
    this.viewingId.set(null);
    const after = this.afterLightbox;
    this.afterLightbox = null;
    if (after) {
      const card = this.cardFor(after.imageId);
      if (after.kind === 'rename') {
        card?.startRename();
      } else {
        card?.askDelete();
      }
      return;
    }
    if (shownId) {
      this.cardFor(shownId)?.focusView();
    }
  }

  protected onDragOver(event: DragEvent): void {
    if (this.state().status !== 'ready' || !hasFiles(event)) {
      return;
    }
    // Needed for the browser to allow a drop here instead of opening the
    // file in the tab.
    event.preventDefault();
    if (event.dataTransfer) {
      event.dataTransfer.dropEffect = 'copy';
    }
    this.dragging.set(true);
    // `dragover` repeats every few milliseconds while a file is over the
    // page; when it stops (the drag left, or was canceled with Esc, which
    // not every browser reports), the highlight goes away by itself.
    clearTimeout(this.dragTimer);
    this.dragTimer = setTimeout(() => this.dragging.set(false), 300);
  }

  protected onDragLeave(event: DragEvent): void {
    // `relatedTarget` is null only when the drag leaves the window.
    if (event.relatedTarget === null) {
      this.dragging.set(false);
    }
  }

  protected onDrop(event: DragEvent): void {
    if (this.state().status !== 'ready' || !hasFiles(event)) {
      return;
    }
    event.preventDefault();
    this.dragging.set(false);
    const files = Array.from(event.dataTransfer?.files ?? []);
    if (files.length > 0) {
      this.addFiles(files);
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
    this.announce(`Imagem enviada: ${image.name}.`);
  }

  private cardFor(imageId: string): GalleryCard | undefined {
    return this.cards().find((card) => card.image().id === imageId);
  }

  private announce(text: string): void {
    // Cleared first, so the same words twice are read twice.
    this.announcement.set('');
    setTimeout(() => this.announcement.set(text), 50);
  }
}

function hasFiles(event: DragEvent): boolean {
  return Array.from(event.dataTransfer?.types ?? []).includes('Files');
}
