import { Component, effect, inject, signal } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { newKey } from '../../core/connect/idempotency';
import { FamiliarEyesClient, familiarSightMessage } from '../../core/play/familiar-eyes';
import { SheetFrame } from '../../pages/live-session/combat/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../pages/live-session/combat/sheet-host';

/** What the question needs: whose eyes, whose character, and whether a combat is on (it costs the action there). */
export interface FamiliarEyesData {
  readonly campaignId: string;
  readonly characterId: string;
  readonly characterName: string;
  readonly familiarName: string;
  /** The characters sees through their familiar in a combat: it costs the action and lasts to the next turn. */
  readonly inCombat: boolean;
}

/** Opens "Ver pelos olhos do ...?": a bottom sheet on a phone, a dialog from a tablet up. It answers `true` when the sight started. */
export function openFamiliarEyes(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: FamiliarEyesData,
) {
  return openSheet<FamiliarEyesSheet, FamiliarEyesData, boolean>(
    dialog,
    bottomSheet,
    FamiliarEyesSheet,
    {
      data,
      ariaLabel: `Ver pelos olhos do ${data.familiarName}?`,
      labelledBy: 'sheet-t',
      width: '480px',
    },
  );
}

/**
 * "Ver pelos olhos do Nanquim?" (MR-036, E9-04 state 3): the question that says what
 * it costs before it does it — the character is blind and deaf meanwhile (and, in a
 * combat, it spends the action and lasts until the start of the next turn) — with
 * "Cancelar" first and "Ver pelos olhos" beside it, the same size. The server decides
 * whether the familiar is within 30 m; a refusal is said here, by its reason. The key
 * is made once, so a repeated tap never starts it twice.
 */
@Component({
  selector: 'app-familiar-eyes-sheet',
  imports: [MatButtonModule, MatIconModule, SheetFrame],
  templateUrl: './familiar-eyes-sheet.html',
  styleUrl: './familiar-eyes-sheet.scss',
})
export class FamiliarEyesSheet {
  private readonly api = inject(FamiliarEyesClient);
  private readonly sheet = injectSheet<FamiliarEyesData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  private readonly key = newKey();

  constructor() {
    // A request in the air cannot be dismissed (Esc, the backdrop, ✕, Cancelar): its answer is always shown.
    effect(() => this.sheet.lock(this.busy()));
  }

  protected async confirm(): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      await this.api.start(this.data.campaignId, this.data.characterId, this.key);
      this.sheet.close(true);
    } catch (err) {
      this.error.set(familiarSightMessage(err, this.data.familiarName));
    } finally {
      this.busy.set(false);
    }
  }

  protected cancel(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(false);
  }
}
