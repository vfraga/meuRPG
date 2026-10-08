import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { of } from 'rxjs';

import { ImageGenClient } from '../../core/images/imagegen-client';
import { FakeImageGenClient, imageStatus } from '../../core/images/imagegen-testing';
import { GenerateImageButton } from './generate-image-button';

const plain = (t: string | null | undefined) =>
  (t ?? '')
    .replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();

describe('GenerateImageButton (E10-07 8: generation off)', () => {
  let api: FakeImageGenClient;
  let opened: unknown[];

  async function setup(origin = { kind: 'gallery' } as const) {
    opened = [];
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: ImageGenClient, useValue: api },
        {
          provide: MatDialog,
          useValue: {
            open: (...args: unknown[]) => (
              opened.push(args),
              { afterClosed: () => of({ generated: 1, map: null }) }
            ),
          },
        },
        {
          provide: MatBottomSheet,
          useValue: { open: () => ({ afterDismissed: () => of(undefined) }) },
        },
      ],
    });
    const fixture = TestBed.createComponent(GenerateImageButton);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('origin', origin);
    const done: unknown[] = [];
    fixture.componentInstance.done.subscribe((o) => done.push(o));
    fixture.detectChanges();
    await fixture.whenStable();
    await new Promise((r) => setTimeout(r));
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, done };
  }

  beforeEach(() => {
    api = new FakeImageGenClient();
  });

  it('opens the dialog, and tells the page what was made when it closes', async () => {
    const { el, done, fixture } = await setup();
    const button = el.querySelector('button')!;
    expect(plain(button.textContent)).toContain('Gerar imagem com IA');
    expect(button.getAttribute('aria-disabled')).not.toBe('true');
    button.click();
    // The dialog is loaded when it is asked for.
    await vi.waitFor(() => expect(opened).toHaveLength(1), { timeout: 15_000 });
    fixture.detectChanges();
    expect(done).toEqual([{ generated: 1, map: null }]);
  });

  it('with generation off the button stays, dashed, with the reason written under it, and no dialog opens', async () => {
    api.statusResult = imageStatus({ enabled: false });
    const { el, done } = await setup();
    const button = el.querySelector('button')!;
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(plain(el.querySelector('.reason')?.textContent)).toBe(
      'A geração de imagens não está ligada neste servidor.',
    );
    expect(button.getAttribute('aria-describedby')).toBe(el.querySelector('.reason')?.id);
    button.click();
    expect(opened).toEqual([]);
    expect(done).toEqual([]);
  });

  it('a status that cannot be read does not block: the dialog says what is wrong', async () => {
    api.statusResult = new Error('down');
    const { el } = await setup();
    expect(el.querySelector('button')!.getAttribute('aria-disabled')).not.toBe('true');
    expect(el.querySelector('.reason')).toBeNull();
  });
});

describe('GenerateImageButton, two taps in a row', () => {
  it('opens one dialog for two taps before the lazy chunk arrives', async () => {
    const opened: unknown[] = [];
    const sheets: unknown[] = [];
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: ImageGenClient, useValue: new FakeImageGenClient() },
        {
          provide: MatDialog,
          useValue: {
            open: (...args: unknown[]) => (opened.push(args), { afterClosed: () => of(undefined) }),
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
