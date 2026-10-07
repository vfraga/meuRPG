import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { of } from 'rxjs';

import { ImageGenClient } from '../../core/images/imagegen-client';
import { FakeImageGenClient } from '../../core/images/imagegen-testing';
import { GenerateImageButton } from './generate-image-button';

// Finding U14-6 in review/unit-14-web-maps.md
describe('Review14 U14-6: a second click while the dialog chunk loads opens a second dialog', () => {
  it('opens exactly one generate dialog/sheet for two clicks before the lazy import resolves', async () => {
    const opened: unknown[] = [];
    const sheets: unknown[] = [];
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: ImageGenClient, useValue: new FakeImageGenClient() },
        {
          provide: MatDialog,
          useValue: {
            open: (...args: unknown[]) => (
              opened.push(args),
              { afterClosed: () => of(undefined) }
            ),
          },
        },
        {
          provide: MatBottomSheet,
          useValue: {
            open: (...args: unknown[]) => (
              sheets.push(args),
              { afterDismissed: () => of(undefined) }
            ),
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(GenerateImageButton);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('origin', { kind: 'gallery' });
    fixture.detectChanges();
    await fixture.whenStable();
    await new Promise((r) => setTimeout(r));
    const button = (fixture.nativeElement as HTMLElement).querySelector('button')!;

    button.click();
    button.click(); // second tap while the first import() is still pending

    await vi.waitFor(() => expect(opened.length + sheets.length).toBeGreaterThan(0), {
      timeout: 15_000,
    });
    await new Promise((r) => setTimeout(r, 200));
    expect(opened.length + sheets.length).toBe(1);
  });
});
