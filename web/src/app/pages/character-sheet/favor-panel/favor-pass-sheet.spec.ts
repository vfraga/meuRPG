import { inject } from '@angular/core';
import { type ComponentFixture, TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { FavorBlockedReason } from '../../../../gen/meurpg/play/v1/favor_pb';
import { FavorClient } from '../../../core/play/favor-client';
import { FavorState } from '../../../core/play/favor-state';
import { FakeFavorClient, favorPass, favorRefused } from '../../../core/play/favor-testing';
import { FavorPassSheet, type FavorPassData } from './favor-pass-sheet';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

const recipients = [
  { characterId: 'tor', name: 'Tor', detail: 'Guerreiro 5' },
  { characterId: 'pens', name: 'Pensantus', detail: 'Mago 5' },
  { characterId: 'jab', name: 'Jabuti', detail: 'Ladino 5' },
];

function setup() {
  const client = new FakeFavorClient();
  client.passAnswer = favorPass();
  const close = vi.fn();
  const dialogRef = { close, disableClose: false };
  TestBed.configureTestingModule({
    providers: [
      FavorState,
      { provide: FavorClient, useValue: client.asClient() },
      { provide: MatDialogRef, useValue: dialogRef },
      {
        provide: MAT_DIALOG_DATA,
        useFactory: (): FavorPassData => ({
          favor: inject(FavorState),
          fromId: 'brisa',
          fromName: 'Brisa',
          recipients,
        }),
      },
    ],
  });
  const favor = TestBed.inject(FavorState);
  favor.campaignId.set('camp');
  const fixture = TestBed.createComponent(FavorPassSheet);
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement, client, close, dialogRef, favor };
}

async function settle(fixture: ComponentFixture<FavorPassSheet>) {
  await fixture.whenStable();
  await Promise.resolve();
  fixture.detectChanges();
}

describe('FavorPassSheet ("Passar a Inspiração", board 581-04)', () => {
  beforeEach(() => TestBed.resetTestingModule());

  it('reads the title, the subtitle, "Para quem" and the line that nothing moves until the master answers', () => {
    const { el } = setup();
    expect(flat(el.querySelector('h2'))).toBe('Passar a Inspiração');
    expect(flat(el.querySelector('.frame__sub'))).toBe('Brisa · o mestre aceita ou recusa');
    expect(flat(el.querySelector('legend'))).toBe('Para quem');
    expect(flat(el)).toContain(
      'Até o mestre responder, ela continua com você e não pode ser usada.',
    );
  });

  it('lists the other player characters as radios, each with its class and level, and chooses the first', () => {
    const { el } = setup();
    const options = Array.from(el.querySelectorAll('label.opt'));
    expect(options.map((o) => flat(o.querySelector('b')))).toEqual(['Tor', 'Pensantus', 'Jabuti']);
    expect(options.map((o) => flat(o.querySelector('.opt__sub')))).toEqual([
      'Guerreiro 5',
      'Mago 5',
      'Ladino 5',
    ]);
    const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type=radio]'));
    expect(radios.map((r) => r.checked)).toEqual([true, false, false]);
  });

  it('has the two buttons the board names, and "Pedir ao mestre" is the filled one', () => {
    const { el } = setup();
    expect(flat(el.querySelector('.pf__cancel'))).toBe('Cancelar');
    expect(flat(el.querySelector('.pf__go'))).toBe('keyboard_tabPedir ao mestre');
  });

  it('asks for the character chosen, with a key, and closes with true when the master is asked', async () => {
    const { fixture, el, client, close } = setup();
    el.querySelectorAll<HTMLInputElement>('input[type=radio]')[1].click();
    fixture.detectChanges();
    el.querySelector<HTMLButtonElement>('.pf__go')!.click();
    await settle(fixture);
    expect(client.asks).toHaveLength(1);
    expect(client.asks[0]).toMatchObject({ campaignId: 'camp', fromId: 'brisa', toId: 'pens' });
    expect(client.asks[0].key).not.toBe('');
    expect(close).toHaveBeenCalledWith(true);
  });

  it('reads "Pedindo ao mestre…" while the call is on its way, dashes "Cancelar" and does not ask twice', async () => {
    const { fixture, el, client, close, dialogRef } = setup();
    client.hold = 'ask';
    el.querySelector<HTMLButtonElement>('.pf__go')!.click();
    fixture.detectChanges();
    await Promise.resolve();
    fixture.detectChanges();
    const ask = el.querySelector<HTMLButtonElement>('.pf__off')!;
    const cancel = el.querySelector<HTMLButtonElement>('.pf__cancel')!;
    expect(flat(ask)).toContain('Pedindo ao mestre…');
    expect(ask.getAttribute('aria-disabled')).toBe('true');
    expect(cancel.classList.contains('mr-button--off')).toBe(true);
    // Esc and the backdrop wait for the answer.
    expect(dialogRef.disableClose).toBe(true);
    ask.click();
    cancel.click();
    expect(client.asks).toHaveLength(1);
    expect(close).not.toHaveBeenCalled();
    client.release[0]();
    await settle(fixture);
    expect(close).toHaveBeenCalledWith(true);
  });

  it('stays open with the reason in an alert when the server refuses', async () => {
    const { fixture, el, client, close } = setup();
    client.passError = favorRefused(FavorBlockedReason.NO_SESSION);
    el.querySelector<HTMLButtonElement>('.pf__go')!.click();
    await settle(fixture);
    expect(close).not.toHaveBeenCalled();
    expect(flat(el.querySelector('[role=alert]'))).toBe('errorSó numa sessão aberta.');
  });

  it('sends the same key for a retry of the same ask, and another for another choice', async () => {
    const { fixture, el, client } = setup();
    client.passError = new Error('down');
    el.querySelector<HTMLButtonElement>('.pf__go')!.click();
    await settle(fixture);
    el.querySelector<HTMLButtonElement>('.pf__go')!.click();
    await settle(fixture);
    expect(client.asks[0].key).toBe(client.asks[1].key);
    el.querySelectorAll<HTMLInputElement>('input[type=radio]')[2].click();
    fixture.detectChanges();
    el.querySelector<HTMLButtonElement>('.pf__go')!.click();
    await settle(fixture);
    expect(client.asks[2].key).not.toBe(client.asks[0].key);
  });

  it('closes with false on "Cancelar" and asks nothing', () => {
    const { el, client, close } = setup();
    el.querySelector<HTMLButtonElement>('.pf__cancel')!.click();
    expect(close).toHaveBeenCalledWith(false);
    expect(client.asks).toEqual([]);
  });
});
