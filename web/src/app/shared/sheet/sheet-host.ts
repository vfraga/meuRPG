import { type ComponentType } from '@angular/cdk/portal';
import { inject } from '@angular/core';
import {
  MAT_BOTTOM_SHEET_DATA,
  MatBottomSheet,
  type MatBottomSheetConfig,
  MatBottomSheetRef,
} from '@angular/material/bottom-sheet';
import { MAT_DIALOG_DATA, MatDialog, MatDialogRef } from '@angular/material/dialog';
import type { Observable } from 'rxjs';

import { PHONE_QUERY } from '../map-view/media-query';

/**
 * One component, two containers (the pattern of "Ajustar", E5-05): the
 * attack sheet, the NPC's "Dano/Cura" and the log are a bottom sheet on a
 * phone and a dialog from a tablet up. `openSheet` picks the container;
 * `injectSheet` is what the component asks inside it, for its data and for
 * closing, without caring which one it is in.
 */
export function openSheet<C, D, R>(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  component: ComponentType<C>,
  config: {
    data: D;
    ariaLabel: string;
    labelledBy?: string;
    width?: string;
    tall?: boolean;
    /** The bottom sheet's own panel class, instead of `mr-sheet` / `mr-sheet-tall` (a sheet with its own distance from the top). */
    panelClass?: string;
    /** An alert that must be answered (Escudo): Esc and the backdrop do not close it. */
    alert?: boolean;
    /** A form's first field (a CSS selector inside the sheet), instead of the title. */
    focus?: string;
    /** `false`: the sheet does not put the focus back on its opener when it closes (the opener does it,
     * with the focus ring: the browser draws none for a focus a dialog restores). */
    restoreFocus?: boolean;
  },
): Observable<R | undefined> {
  const phone = typeof matchMedia === 'function' && matchMedia(PHONE_QUERY).matches;
  if (phone) {
    return bottomSheet
      .open<C, D, R>(component, {
        data: config.data,
        ariaLabel: config.ariaLabel,
        // The title first (README-A): a stray Enter can't roll or confirm.
        // An alert starts on its safe button (`data-initial-focus`), the others on the title.
        autoFocus: config.alert ? '[data-initial-focus]' : (config.focus ?? 'first-heading'),
        panelClass: config.panelClass ?? (config.tall ? 'mr-sheet-tall' : 'mr-sheet'),
        disableClose: config.alert,
        restoreFocus: config.restoreFocus ?? true,
        // MatBottomSheet hands its whole config to the CDK dialog, which knows
        // `role`; the sheet's own type does not list it.
        ...(config.alert ? { role: 'alertdialog' } : {}),
      } as MatBottomSheetConfig<D>)
      .afterDismissed();
  }
  return dialog
    .open<C, D, R>(component, {
      data: config.data,
      width: config.width ?? '480px',
      maxWidth: 'calc(100vw - 32px)',
      maxHeight: '92dvh',
      ariaLabelledBy: config.labelledBy,
      ariaLabel: config.labelledBy ? undefined : config.ariaLabel,
      autoFocus: config.alert ? '[data-initial-focus]' : (config.focus ?? 'first-heading'),
      disableClose: config.alert,
      restoreFocus: config.restoreFocus ?? true,
      role: config.alert ? 'alertdialog' : 'dialog',
    })
    .afterClosed();
}

export interface SheetHandle<D, R> {
  readonly data: D;
  /** True in the phone's bottom sheet: it draws its grab bar. */
  readonly inSheet: boolean;
  close(result?: R): void;
  /** While locked, Esc and a click on the backdrop do not dismiss the sheet (a request is in the air). Unlocking puts back what the
   * sheet was opened with: an alert that must be answered stays so. */
  lock(locked: boolean): void;
}

export function injectSheet<D, R = void>(): SheetHandle<D, R> {
  const dialogRef = inject<MatDialogRef<unknown, R>>(MatDialogRef, { optional: true });
  const sheetRef = inject<MatBottomSheetRef<unknown, R>>(MatBottomSheetRef, { optional: true });
  const data =
    inject<D | null>(MAT_DIALOG_DATA, { optional: true }) ?? inject<D>(MAT_BOTTOM_SHEET_DATA);
  const opened = dialogRef?.disableClose ?? sheetRef?.disableClose ?? false;
  return {
    data,
    inSheet: sheetRef !== null,
    close: (result) => {
      dialogRef?.close(result);
      sheetRef?.dismiss(result);
    },
    lock: (locked) => {
      if (dialogRef) {
        dialogRef.disableClose = locked || opened;
      }
      if (sheetRef) {
        sheetRef.disableClose = locked || opened;
      }
    },
  };
}
