import {
  Component,
  DOCUMENT,
  DestroyRef,
  Injector,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog, MatDialogRef } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import type { ShownImageVm } from '../live-session.types';
import { ShownImageViewer } from './shown-image-viewer';

/** How long the block fades out and collapses before it leaves the DOM
 * (150 ms fade + 200 ms collapse, README-C). */
const LEAVE_MS = 350;

/**
 * "O mestre está mostrando" (E5-12, E5-13, MR-028): the image the master
 * shows, on the player's session page, above the map. It comes and goes
 * live, so nothing may jump:
 *
 * - The frame is reserved from the image's size, before its bytes arrive:
 *   on a phone full width by 160px; on a computer 320px tall, as wide as
 *   the image's shape up to 440px. The image is `contain` on `ground`, so
 *   a portrait and a landscape take the same space.
 * - The caption is the image's name; "Fica aqui enquanto o mestre mostrar."
 *   says it is live and temporary.
 * - "Ver em tela cheia" (named after the image) opens a read-only view that
 *   follows the live state; the whole image is a pointer shortcut to it.
 * - It grows and fades in (200 ms), and fades out then collapses, only as
 *   an answer to what the master did; with reduced motion it just appears
 *   and disappears. It never takes focus or scroll; the page announces it.
 */
@Component({
  selector: 'app-shown-image-block',
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './shown-image-block.html',
  styleUrl: './shown-image-block.scss',
})
export class ShownImageBlock {
  private readonly dialog = inject(MatDialog);
  private readonly injector = inject(Injector);
  private readonly document = inject(DOCUMENT);

  /** The image on show, or `null`. */
  readonly image = input<ShownImageVm | null>(null);
  /** The first value comes with the snapshot and appears without motion. */
  readonly animate = input(true);

  protected readonly phone = mediaQuery(PHONE_QUERY);
  /** What the block draws: the input, kept for the fade-out after it ends. */
  protected readonly shown = signal<ShownImageVm | null>(null);
  protected readonly leaving = signal(false);
  protected readonly entering = signal(false);
  protected readonly failed = signal(false);
  protected readonly loaded = signal(false);
  protected readonly aspect = computed(() => {
    const s = this.shown();
    return s && s.width > 0 && s.height > 0 ? `${s.width} / ${s.height}` : '4 / 3';
  });

  private readonly viewerData = signal<ShownImageVm | null>(null);
  private viewer: MatDialogRef<ShownImageViewer> | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private first = true;

  constructor() {
    effect(() => {
      const next = this.image();
      untracked(() => this.follow(next));
    });
    inject(DestroyRef).onDestroy(() => {
      this.clearTimer();
      this.viewer?.close();
    });
  }

  private follow(next: ShownImageVm | null): void {
    const previous = this.shown();
    this.viewerData.set(next);
    this.clearTimer();
    if (next) {
      const appears = previous === null || this.leaving();
      this.leaving.set(false);
      // A picture shown again while it was leaving, after its load failed, gets a new try.
      if (previous?.id !== next.id || (appears && this.failed())) {
        this.failed.set(false);
        this.loaded.set(false);
      }
      this.shown.set(next);
      this.entering.set(appears && !this.first && this.animate() && !this.reduced());
    } else if (previous) {
      if (this.reduced() || !this.animate()) {
        this.shown.set(null);
      } else {
        this.entering.set(false);
        this.leaving.set(true);
        this.timer = setTimeout(() => {
          this.leaving.set(false);
          this.shown.set(null);
          this.timer = null;
        }, LEAVE_MS);
      }
      // The view closes with the image; focus goes to the map's heading.
      if (this.viewer) {
        this.viewer = null;
        this.document.getElementById('session-map-heading')?.focus();
      }
    }
    this.first = false;
  }

  private reduced(): boolean {
    return (
      this.document.defaultView?.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
    );
  }

  private clearTimer(): void {
    if (this.timer !== null) {
      clearTimeout(this.timer);
      this.timer = null;
    }
  }

  protected openFull(): void {
    if (this.viewer) {
      return;
    }
    const phone = this.phone();
    this.viewer = this.dialog.open<ShownImageViewer, unknown>(ShownImageViewer, {
      data: this.viewerData.asReadonly(),
      injector: this.injector,
      width: phone ? '100vw' : '90vw',
      maxWidth: phone ? '100vw' : '1100px',
      height: phone ? '100dvh' : '88dvh',
      maxHeight: phone ? '100dvh' : '88dvh',
      autoFocus: 'dialog',
    });
    this.viewer.afterClosed().subscribe(() => (this.viewer = null));
  }

  protected retry(): void {
    this.failed.set(false);
    this.loaded.set(false);
    const current = this.shown();
    // A new object makes the image request again.
    this.shown.set(current ? { ...current } : null);
  }
}
