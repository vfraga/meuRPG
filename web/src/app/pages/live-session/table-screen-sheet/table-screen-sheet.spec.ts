import { signal } from '@angular/core';
import { type ComponentFixture, TestBed } from '@angular/core/testing';
import { MatBottomSheetRef } from '@angular/material/bottom-sheet';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';
import { vi } from 'vitest';

import {
  TableScreenKind as K,
  TableScreenNotice as N,
  TableScreenUnavailable as U,
} from '../../../../gen/meurpg/play/v1/table_screen_pb';
import { TableScreenPanel } from '../../../core/play/table-screen-panel';
import { screenResponse } from '../../../core/play/table-screen-testing';
import { TableScreenSheet, type TableScreenSheetData } from './table-screen-sheet';
import { tableScreenLink } from './table-screen-link';

/** The words of an element, with the icons' ligature names left out. */
const text = (el: Element | null | undefined) => {
  const copy = el?.cloneNode(true) as Element | undefined;
  copy?.querySelectorAll('mat-icon').forEach((icon) => icon.remove());
  return (copy?.textContent ?? '').replace(/\s+/g, ' ').trim();
};

const OPTIONS = [
  { kind: K.MAP, available: true },
  { kind: K.IMAGE, available: true },
  { kind: K.SCENE, available: true },
  { kind: K.COMBAT, available: false, unavailable: U.NO_COMBAT },
  { kind: K.IDLE, available: true },
];
const OPEN = { open: true };

interface Harness {
  readonly fixture: ComponentFixture<TableScreenSheet>;
  readonly el: HTMLElement;
  readonly get: ReturnType<typeof vi.fn>;
  readonly set: ReturnType<typeof vi.fn>;
  readonly panel: TableScreenPanel;
  readonly ref: { close: ReturnType<typeof vi.fn>; disableClose: boolean };
  flush(): Promise<void>;
}

async function mount(
  answer: Parameters<typeof screenResponse>[0] | 'fail' = { options: OPTIONS, presence: OPEN },
): Promise<Harness> {
  const get = vi.fn(() =>
    answer === 'fail' ? Promise.reject(new Error('down')) : Promise.resolve(screenResponse(answer)),
  );
  const set = vi.fn(() => Promise.resolve({}));
  const panel = new TableScreenPanel({ get, set } as never, () => 'c1');
  const data: TableScreenSheetData = {
    panel,
    campaignId: 'c1',
    names: signal({ map: 'Cripta do Corvo', image: 'Vista do salão', scene: '' }).asReadonly(),
  };
  const ref = { close: vi.fn(), disableClose: false };
  TestBed.configureTestingModule({
    providers: [
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: ref },
    ],
  });
  const fixture = TestBed.createComponent(TableScreenSheet);
  const flush = async () => {
    for (let i = 0; i < 8; i++) {
      await Promise.resolve();
    }
    fixture.detectChanges();
  };
  fixture.detectChanges();
  await flush();
  return { fixture, el: fixture.nativeElement as HTMLElement, get, set, panel, ref, flush };
}

const radios = (el: HTMLElement) =>
  Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
const byId = (el: HTMLElement, id: string) => el.querySelector(`[data-testid="${id}"]`);

describe('TableScreenSheet', () => {
  it('reads the panel when it opens, and says it is reading until then', async () => {
    const h = await mount();
    expect(h.get).toHaveBeenCalledTimes(1);
    expect(text(h.el.querySelector('h2'))).toBe('Tela da mesa');
    TestBed.resetTestingModule();
    let release!: () => void;
    const get = vi.fn(
      () =>
        new Promise((resolve) => {
          release = () => resolve(screenResponse());
        }),
    );
    const panel = new TableScreenPanel({ get, set: vi.fn() } as never, () => 'c1');
    TestBed.configureTestingModule({
      providers: [
        {
          provide: MAT_DIALOG_DATA,
          useValue: { panel, campaignId: 'c1', names: signal({ map: '', image: '', scene: '' }) },
        },
        { provide: MatDialogRef, useValue: { close: vi.fn(), disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(TableScreenSheet);
    fixture.detectChanges();
    expect(text(fixture.nativeElement)).toContain('Lendo a Tela da mesa…');
    release();
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
    fixture.detectChanges();
    expect(text(fixture.nativeElement)).not.toContain('Lendo a Tela da mesa…');
  });

  it('says the first read failed, and reads again on "Tentar de novo"', async () => {
    const h = await mount('fail');
    expect(text(h.el)).toContain('Não deu para ler a Tela da mesa.');
    h.el.querySelector<HTMLButtonElement>('.js-read-again')!.click();
    await h.flush();
    expect(h.get).toHaveBeenCalledTimes(2);
  });

  it('shows no rows and says so while no TV is open', async () => {
    const h = await mount({ options: OPTIONS, presence: { open: false } });
    expect(radios(h.el)).toHaveLength(0);
    expect(text(byId(h.el, 'tv-state'))).toBe('Fechada');
    expect(text(byId(h.el, 'tv-closed'))).toBe(
      'Nenhuma Tela da mesa aberta agora. O que você escolher vale assim que abrir uma.',
    );
  });

  it('says when the connection was lost, with the time', async () => {
    const h = await mount({
      options: OPTIONS,
      presence: { open: false, lostAt: new Date(2026, 0, 5, 21, 14) },
    });
    expect(text(byId(h.el, 'tv-state'))).toBe('Sem conexão');
    expect(text(byId(h.el, 'tv-lost'))).toContain('Sem conexão desde 21:14.');
    expect(radios(h.el)).toHaveLength(0);
  });

  it('is a radio group: "Espelhar" chosen, and one row per pin, the unavailable one dashed with its reason', async () => {
    const h = await mount();
    expect(text(byId(h.el, 'tv-state'))).toBe('Aberta');
    const group = h.el.querySelector('[role="radiogroup"]')!;
    expect(group.getAttribute('aria-labelledby')).toBe('tv-what-t');
    expect(radios(h.el)).toHaveLength(6);
    expect(radios(h.el)[0].checked).toBe(true);
    expect(text(byId(h.el, 'tv-pin-map'))).toContain('Cripta do Corvo, com o que o grupo vê');
    const combat = byId(h.el, 'tv-pin-combat')!;
    expect(combat.querySelector('input')?.disabled).toBe(true);
    expect(combat.querySelector('.row--off')).not.toBeNull();
    expect(text(combat)).toContain('Nenhum combate agora.');
    expect(text(byId(h.el, 'tv-now'))).toBe('A tela de espera.');
  });

  it('pins a kind on tap and says it was done', async () => {
    const h = await mount();
    const input = byId(h.el, 'tv-pin-image')!.querySelector('input')!;
    input.dispatchEvent(new Event('change'));
    await h.flush();
    expect(h.set).toHaveBeenCalledWith('c1', 1, { mirror: false, pin: K.IMAGE });
    expect(text(h.el.querySelector('p.mr-visually-hidden[role="status"]'))).toBe(
      'A Tela da mesa mudou.',
    );
  });

  it('goes back to the mirror', async () => {
    const h = await mount({
      options: OPTIONS,
      presence: OPEN,
      choice: { mirror: false, pin: K.MAP },
    });
    expect(radios(h.el)[1].checked).toBe(true);
    expect(text(byId(h.el, 'tv-now'))).toContain('fixa');
    byId(h.el, 'tv-mirror')!.querySelector('input')!.dispatchEvent(new Event('change'));
    await h.flush();
    expect(h.set).toHaveBeenCalledWith('c1', 1, { mirror: true, pin: K.UNSPECIFIED });
  });

  it('says the TV stayed as it was when the save fails, and tries again with the same choice', async () => {
    const h = await mount();
    h.set.mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
    byId(h.el, 'tv-pin-map')!.querySelector('input')!.dispatchEvent(new Event('change'));
    await h.flush();
    const alert = byId(h.el, 'tv-not-saved')!;
    expect(alert.getAttribute('role')).toBe('alert');
    expect(text(alert)).toBe('Não deu para mudar a Tela da mesa. A TV continua como estava.');
    // The notice and the retry are all there is: the rows give way, and the focus goes to the retry.
    expect(radios(h.el)).toHaveLength(0);
    expect(document.activeElement).toBe(h.el.querySelector('.js-retry'));
    // Said once: the status region is silent for a refusal.
    expect(text(h.el.querySelector('p.mr-visually-hidden[role="status"]'))).toBe('');
    h.el.querySelector<HTMLButtonElement>('.js-retry')!.click();
    await h.flush();
    expect(h.set).toHaveBeenCalledTimes(2);
    expect(h.set.mock.lastCall?.[2]).toEqual({ mirror: false, pin: K.MAP });
    expect(byId(h.el, 'tv-not-saved')).toBeNull();
  });

  it('says the same when the choice had moved (stale), after reading the new one', async () => {
    const h = await mount();
    h.set.mockRejectedValueOnce(new ConnectError('x', Code.Aborted));
    byId(h.el, 'tv-pin-map')!.querySelector('input')!.dispatchEvent(new Event('change'));
    await h.flush();
    expect(text(byId(h.el, 'tv-not-saved'))).toContain('A TV continua como estava.');
    expect(radios(h.el)).toHaveLength(0);
    expect(h.get).toHaveBeenCalledTimes(2);
  });

  it('explains the mirror order under the ⓘ, and the button says whether it is open', async () => {
    const h = await mount();
    const info = h.el.querySelector<HTMLButtonElement>('.info')!;
    expect(info.getAttribute('aria-expanded')).toBe('false');
    expect(byId(h.el, 'tv-order')).toBeNull();
    info.click();
    h.fixture.detectChanges();
    expect(info.getAttribute('aria-expanded')).toBe('true');
    // The order is all there is under "Espelhar" while it is open; the pins come back when it closes.
    expect(radios(h.el)).toHaveLength(1);
    expect(byId(h.el, 'tv-now')).toBeNull();
    const items = Array.from(byId(h.el, 'tv-order')!.querySelectorAll('li')).map(text);
    expect(items).toEqual([
      'uma imagem mostrada a toda a mesa;',
      'o combate;',
      'a cena aberta a todos;',
      'o mapa;',
      'a tela de espera.',
    ]);
    info.click();
    h.fixture.detectChanges();
    expect(byId(h.el, 'tv-order')).toBeNull();
    expect(radios(h.el)).toHaveLength(6);
  });

  it('says why the stage is not what was asked for', async () => {
    const h = await mount({
      options: OPTIONS,
      presence: OPEN,
      notice: N.PIN_ENDED,
      choice: { mirror: false, pin: K.COMBAT },
    });
    expect(text(byId(h.el, 'tv-notice'))).toContain(
      'O combate acabou. A TV foi para a tela de espera.',
    );
  });

  it('has the privacy line and opens the tab in a new window without the opener', async () => {
    const h = await mount();
    expect(text(h.el)).toContain('A TV mostra só o que toda a mesa pode ver.');
    const open = h.el.querySelector<HTMLAnchorElement>('a.js-open')!;
    expect(open.getAttribute('href')).toBe(`${location.origin}/campaigns/c1/table-screen`);
    expect(open.target).toBe('_blank');
    expect(open.rel).toContain('noopener');
  });

  it('waits for the TV after "Abrir", and stops when one connects', async () => {
    const h = await mount({ options: OPTIONS, presence: { open: false } });
    const open = h.el.querySelector<HTMLAnchorElement>('a.js-open')!;
    open.addEventListener('click', (e) => e.preventDefault());
    open.click();
    h.fixture.detectChanges();
    expect(text(byId(h.el, 'tv-state'))).toBe('Abrindo…');
    expect(text(h.el)).toContain('Esperando a tela se conectar.');
    h.panel.heard({ presenceKnown: true, open: true, lostAt: null, choiceRevision: 0 } as never);
    h.fixture.detectChanges();
    expect(text(byId(h.el, 'tv-state'))).toBe('Aberta');
    expect(text(h.el)).not.toContain('Esperando a tela se conectar.');
  });

  it('copies the tab address only: no token, nothing in browser storage', async () => {
    const h = await mount();
    const writeText = vi.fn(() => Promise.resolve());
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    const before = [localStorage.length, sessionStorage.length];
    h.el.querySelector<HTMLButtonElement>('.js-copy')!.click();
    await h.flush();
    expect(writeText).toHaveBeenCalledWith(`${location.origin}/campaigns/c1/table-screen`);
    expect(text(h.el.querySelector('.js-copy'))).toBe('Link copiado');
    expect([localStorage.length, sessionStorage.length]).toEqual(before);
    h.fixture.destroy();
  });

  it('shows the address in a field when the clipboard refuses', async () => {
    const h = await mount();
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true });
    h.el.querySelector<HTMLButtonElement>('.js-copy')!.click();
    await h.flush();
    const field = h.el.querySelector<HTMLInputElement>('input.field')!;
    expect(field.value).toBe(`${location.origin}/campaigns/c1/table-screen`);
    expect(field.readOnly).toBe(true);
  });

  it('tells the phone sheet how the TV gets the page, and the dialog does not', async () => {
    const h = await mount();
    expect(byId(h.el, 'tv-how')).toBeNull();
    TestBed.resetTestingModule();
    const get = vi.fn(() => Promise.resolve(screenResponse({ options: OPTIONS, presence: OPEN })));
    const panel = new TableScreenPanel({ get, set: vi.fn() } as never, () => 'c1');
    TestBed.configureTestingModule({
      providers: [
        {
          provide: MAT_DIALOG_DATA,
          useValue: { panel, campaignId: 'c1', names: signal({ map: '', image: '', scene: '' }) },
        },
        { provide: MatDialogRef, useValue: { close: vi.fn(), disableClose: false } },
        // The bottom sheet's own refs: the phone's container.
        { provide: MatBottomSheetRef, useValue: { dismiss: vi.fn(), disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(TableScreenSheet);
    fixture.detectChanges();
    expect(text(byId(fixture.nativeElement, 'tv-how'))).toContain(
      'Para a TV: no computador ligado a ela, abra esta sessão e use Tela da mesa na barra',
    );
  });

  it('closes from the frame', async () => {
    const h = await mount();
    h.el.querySelector<HTMLButtonElement>('.frame__close')!.click();
    expect(h.ref.close).toHaveBeenCalled();
  });
});

describe('tableScreenLink', () => {
  it('is the tab address with nothing after the path', () => {
    expect(tableScreenLink('c1', 'https://meurpg.example')).toBe(
      'https://meurpg.example/campaigns/c1/table-screen',
    );
  });
});
