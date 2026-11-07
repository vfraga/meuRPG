import type { MatBottomSheet } from '@angular/material/bottom-sheet';
import type { MatDialog } from '@angular/material/dialog';
import { of } from 'rxjs';
import { vi } from 'vitest';

import { openTableScreenSheet, type TableScreenSheetData } from './table-screen-sheet';

describe('openTableScreenSheet', () => {
  const original = window.matchMedia;
  afterEach(() => {
    window.matchMedia = original;
  });

  it('opens the phone bottom sheet with the title first', () => {
    window.matchMedia = ((query: string) => ({ matches: true, media: query })) as never;
    const open = vi.fn().mockReturnValue({ afterDismissed: () => of(undefined) });
    openTableScreenSheet(
      {} as MatDialog,
      { open } as unknown as MatBottomSheet,
      {} as unknown as TableScreenSheetData,
    ).subscribe();
    const config = open.mock.lastCall?.[1] as { ariaLabel: string; autoFocus: string };
    expect(config.ariaLabel).toBe('Tela da mesa');
    expect(config.autoFocus).toBe('first-heading');
  });

  it('opens a dialog labelled by its title from a tablet up', () => {
    window.matchMedia = ((query: string) => ({ matches: false, media: query })) as never;
    const open = vi.fn().mockReturnValue({ afterClosed: () => of(undefined) });
    openTableScreenSheet(
      { open } as unknown as MatDialog,
      {} as MatBottomSheet,
      {} as unknown as TableScreenSheetData,
    ).subscribe();
    const config = open.mock.lastCall?.[1] as { ariaLabelledBy: string; width: string };
    expect(config.ariaLabelledBy).toBe('tv-sheet-t');
    expect(config.width).toBe('400px');
  });
});
