import { signal } from '@angular/core';

import type { GalleryImage, GalleryUsage } from '../../../gen/meurpg/maps/v1/gallery_pb';
import type { ImageUploader } from './image-uploader';
import {
  DEFAULT_LIMITS,
  UploadFailed,
  type UploadFailureKind,
  precheckImageFile,
  uploadFailureMessage,
} from './upload-errors';

/** A file waiting in line or on its way (its card in the grid). */
export interface UploadItem {
  /** Unique within the queue: tracks the card in `@for`. */
  readonly key: number;
  readonly fileName: string;
  /** `processing`: every byte went out, and the server is checking and
   * encoding the image again. */
  readonly status: 'queued' | 'sending' | 'processing';
  /** 0 to 100. */
  readonly percent: number;
}

/** A file that did not become an image (its notice at the top). */
export interface UploadFailure {
  readonly key: number;
  readonly fileName: string;
  readonly kind: UploadFailureKind;
  /** The Portuguese sentence that follows "Não deu para enviar <arquivo>." */
  readonly message: string;
}

export interface UploadQueueOptions {
  readonly uploader: Pick<ImageUploader, 'upload'>;
  readonly campaignId: () => string;
  /** The gallery's usage as the caller knows it now, for the client-side
   * checks; `null` before the first `ListGalleryImages` answers. */
  readonly usage: () => GalleryUsage | null;
  /** A file became an image. The caller adds it to its list and its usage. */
  readonly uploaded: (image: GalleryImage, fileName: string) => void;
}

/**
 * Sends files to the gallery one after the other (never in parallel: the
 * server processes one image at a time per instance anyway), each with its
 * own progress and its own error, as E5-20 and E5-21 draw them. The gallery
 * page and the gallery picker each own one.
 *
 * Checks each file on the client first (`precheckImageFile`), so a GIF or a
 * 12 MB photo fails at once instead of after the upload; the server stays
 * the authority. A new batch clears the previous batch's notices. A canceled
 * file just leaves; it is not a failure.
 *
 * Plain class with signals, not a service: its lifetime is the component's,
 * which calls `destroy()` on the way out (aborting any upload in flight).
 */
export class UploadQueue {
  private readonly _items = signal<readonly UploadItem[]>([]);
  private readonly _failures = signal<readonly UploadFailure[]>([]);
  readonly items = this._items.asReadonly();
  readonly failures = this._failures.asReadonly();

  private nextKey = 1;
  private readonly files = new Map<number, File>();
  private current: { key: number; controller: AbortController } | null = null;
  private destroyed = false;
  /** Goes up on `reset()`: an upload that was on its way for the old campaign must not report to the new one. */
  private epoch = 0;

  constructor(private readonly options: UploadQueueOptions) {}

  /** Queues the files, in order, and starts sending if nothing is. */
  add(files: Iterable<File>): void {
    if (this.destroyed) {
      return;
    }
    this._failures.set([]);
    const usage = this.limits();
    let pending = this._items().length;
    for (const file of files) {
      const key = this.nextKey++;
      const refused = precheckImageFile(file, usage, pending);
      if (refused) {
        this.fail(key, file.name, refused);
        continue;
      }
      pending++;
      this.files.set(key, file);
      this._items.update((items) => [
        ...items,
        { key, fileName: file.name, status: 'queued', percent: 0 },
      ]);
    }
    void this.pump();
  }

  /** Cancels one file: aborts it if it is on its way, or takes it out of
   * line. */
  cancel(key: number): void {
    if (this.current?.key === key) {
      this.current.controller.abort();
      return;
    }
    this.files.delete(key);
    this.remove(key);
  }

  /** Aborts the upload in flight, forgets the rest and what failed, and keeps the queue usable (the page moved to another campaign). */
  reset(): void {
    this.epoch++;
    this.current?.controller.abort();
    this.files.clear();
    this._items.set([]);
    this._failures.set([]);
  }

  /** Aborts the upload in flight and forgets the rest. */
  destroy(): void {
    this.destroyed = true;
    this.current?.controller.abort();
    this.files.clear();
    this._items.set([]);
  }

  private limits(): Pick<
    GalleryUsage,
    'imageCount' | 'byteCount' | 'maxImages' | 'maxBytes' | 'maxImageBytes'
  > {
    return this.options.usage() ?? { imageCount: 0, byteCount: 0, ...DEFAULT_LIMITS };
  }

  private async pump(): Promise<void> {
    if (this.current || this.destroyed) {
      return;
    }
    const next = this._items().find((item) => item.status === 'queued');
    const file = next && this.files.get(next.key);
    if (!next || !file) {
      return;
    }
    const epoch = this.epoch;
    const controller = new AbortController();
    this.current = { key: next.key, controller };
    this.patch(next.key, { status: 'sending', percent: 0 });
    try {
      const image = await this.options.uploader.upload(this.options.campaignId(), file, {
        signal: controller.signal,
        onProgress: (fraction) =>
          this.patch(
            next.key,
            fraction >= 1
              ? { status: 'processing', percent: 100 }
              : { percent: Math.round(fraction * 100) },
          ),
      });
      this.remove(next.key);
      if (!this.destroyed && epoch === this.epoch) {
        this.options.uploaded(image, file.name);
      }
    } catch (err) {
      this.remove(next.key);
      const kind = err instanceof UploadFailed ? err.kind : 'UNKNOWN';
      if (kind !== 'CANCELED' && !this.destroyed && epoch === this.epoch) {
        this.fail(next.key, file.name, kind);
      }
    } finally {
      this.files.delete(next.key);
      this.current = null;
    }
    void this.pump();
  }

  private fail(key: number, fileName: string, kind: UploadFailureKind): void {
    const message = uploadFailureMessage(kind, this.limits());
    this._failures.update((failures) => [...failures, { key, fileName, kind, message }]);
  }

  private patch(key: number, change: Partial<Pick<UploadItem, 'status' | 'percent'>>): void {
    this._items.update((items) =>
      items.map((item) => (item.key === key ? { ...item, ...change } : item)),
    );
  }

  private remove(key: number): void {
    this._items.update((items) => items.filter((item) => item.key !== key));
  }
}
