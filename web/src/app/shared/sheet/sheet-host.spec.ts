import { ChangeDetectionStrategy, Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { type SheetHandle, injectSheet } from './sheet-host';

@Component({ selector: 'app-probe', template: '', changeDetection: ChangeDetectionStrategy.OnPush })
class Probe {
  readonly sheet: SheetHandle<string, void> = injectSheet<string>();
}

describe('injectSheet().lock', () => {
  function open(disableClose: boolean) {
    const ref = { disableClose, close: vi.fn() };
    TestBed.configureTestingModule({
      providers: [
        { provide: MatDialogRef, useValue: ref },
        { provide: MAT_DIALOG_DATA, useValue: 'dados' },
      ],
    });
    return { ref, probe: TestBed.createComponent(Probe).componentInstance };
  }

  it('keeps Esc and the backdrop from closing the sheet while locked, and gives them back after', () => {
    const { ref, probe } = open(false);
    probe.sheet.lock(true);
    expect(ref.disableClose).toBe(true);
    probe.sheet.lock(false);
    expect(ref.disableClose).toBe(false);
  });

  it('leaves an alert that must be answered undismissable after the lock is released', () => {
    const { ref, probe } = open(true);
    probe.sheet.lock(true);
    probe.sheet.lock(false);
    expect(ref.disableClose).toBe(true);
  });
});
