import {
  ChangeDetectionStrategy,
  Component,
  afterNextRender,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { ImageGenClient } from '../../core/images/imagegen-client';
import type { GenerateOrigin } from '../../core/images/imagegen-copy';
import type { GenerateOutcome } from './image-generate-dialog';

let nextId = 0;

/**
 * "Gerar imagem com IA" (MR-039; E10-07 1, 2, 7 and 8): the button that opens the generate dialog from a map, a scene or the gallery. It asks
 * the server once whether generation is on; when it is not, the button stays, dashed and quiet, with the reason written under it, and no
 * dialog opens (a state is never colour alone). When the dialog closes after making something, `done` tells the page what was made and the map
 * it changed, so the page reads its own data again. Focus returns to this button.
 */
@Component({
  selector: 'app-generate-image-button',
  imports: [MatButtonModule, MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <button
      [matButton]="variant()"
      type="button"
      class="btn"
      [class.mr-button--off]="off()"
      [disabled]="off()"
      disabledInteractive
      [attr.aria-describedby]="off() ? reasonId : null"
      (click)="open()"
    >
      <mat-icon aria-hidden="true">auto_awesome</mat-icon>Gerar imagem com IA
    </button>
    @if (off()) {
      <p class="reason" [id]="reasonId">A geração de imagens não está ligada neste servidor.</p>
    }
  `,
  styles: `
    :host {
      display: inline-flex;
      flex-direction: column;
      align-items: stretch;
      gap: var(--mr-space-2);
    }

    .btn {
      min-height: 44px;

      @media (max-width: 767.98px) {
        min-height: 48px;
      }
    }

    .reason {
      margin: 0;
      color: var(--mr-ink-muted);
      font-size: 14px;
      line-height: 19px;
    }
  `,
})
export class GenerateImageButton {
  private readonly api = inject(ImageGenClient);
  private readonly dialog = inject(MatDialog);
  private readonly sheet = inject(MatBottomSheet);

  readonly campaignId = input.required<string>();
  readonly origin = input.required<GenerateOrigin>();
  /** `text` in a line of the page's own text buttons (the map's header), `outlined` as a button of its own. */
  readonly variant = input<'text' | 'outlined'>('outlined');
  readonly done = output<GenerateOutcome>();

  protected readonly reasonId = `gen-off-${nextId++}`;
  protected readonly off = signal(false);
  private opening = false;

  constructor() {
    afterNextRender(() => {
      this.api.status(this.campaignId()).then(
        (status) => this.off.set(!status.enabled),
        // A status that cannot be read is no reason to block: the dialog says what is wrong.
        () => undefined,
      );
    });
  }

  /** The dialog is loaded when it is first asked for: a player's page, and a master's who never generates, never download it. */
  protected async open(): Promise<void> {
    // One dialog at a time: a second tap while the chunk is on its way, or while the dialog is open, would spend a second slot.
    if (this.off() || this.opening) {
      return;
    }
    this.opening = true;
    try {
      const { openImageGenerate } = await import('./image-generate-dialog');
      openImageGenerate(this.dialog, this.sheet, {
        campaignId: this.campaignId(),
        origin: this.origin(),
      }).subscribe({
        next: (outcome) => {
          if (outcome && (outcome.generated > 0 || outcome.map)) {
            this.done.emit(outcome);
          }
        },
        complete: () => (this.opening = false),
        error: () => (this.opening = false),
      });
    } catch (err) {
      this.opening = false;
      throw err;
    }
  }
}
