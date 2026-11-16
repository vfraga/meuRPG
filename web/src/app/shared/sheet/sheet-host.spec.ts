import { ChangeDetectionStrategy, Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { of } from 'rxjs';

import { PHONE_QUERY } from '../map-view/media-query';
import { type SheetHandle, injectSheet, openSheet } from './sheet-host';

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

describe('openSheet, where the focus starts', () => {
  function opened(config: { alert?: boolean; focus?: string }, phone = false) {
    vi.stubGlobal('matchMedia', (query: string) => ({ matches: phone && query === PHONE_QUERY }));
    const seen: { autoFocus?: unknown }[] = [];
    const dialog = {
      open: (_c: unknown, cfg: { autoFocus?: unknown }) => {
        seen.push(cfg);
        return { afterClosed: () => of(undefined) };
      },
    };
    const bottomSheet = {
      open: (_c: unknown, cfg: { autoFocus?: unknown }) => {
        seen.push(cfg);
        return { afterDismissed: () => of(undefined) };
      },
    };
    openSheet(dialog as never, bottomSheet as never, Probe, {
      data: 'dados',
      ariaLabel: 'Folha',
      ...config,
    });
    vi.unstubAllGlobals();
    return seen[0].autoFocus;
  }

  it('starts an alert on its safe button and any other sheet on the title', () => {
    expect(opened({ alert: true })).toBe('[data-initial-focus]');
    expect(opened({})).toBe('first-heading');
    expect(opened({ alert: true }, true)).toBe('[data-initial-focus]');
  });

  it('starts an alert with no safe answer where it is told to: the title, on a dialog and on a bottom sheet', () => {
    expect(opened({ alert: true, focus: 'first-heading' })).toBe('first-heading');
    expect(opened({ alert: true, focus: 'first-heading' }, true)).toBe('first-heading');
  });
});
