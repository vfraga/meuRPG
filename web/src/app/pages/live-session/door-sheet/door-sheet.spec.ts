import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { MapLayer } from '../../../../gen/meurpg/maps/v1/maps_pb';
import type { DoorSquare } from '../../../core/maps/layers';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient } from '../../../core/maps/maps-testing';
import { DoorSheet, type DoorSheetData } from './door-sheet';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

describe('DoorSheet', () => {
  let api: FakeMapsClient;
  let closed: unknown[];

  function setup(door: DoorSquare, wallSquares: readonly { col: number; row: number }[] = []) {
    api = new FakeMapsClient();
    closed = [];
    const data: DoorSheetData = { campaignId: 'camp-1', mapId: 'map-1', door, wallSquares };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: (r: unknown) => closed.push(r) } },
      ],
    });
    const fixture = TestBed.createComponent(DoorSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const press = async (name: string) => {
      Array.from(el.querySelectorAll('button'))
        .find((b) => plain(b.textContent).includes(name))!
        .click();
      fixture.detectChanges();
      await new Promise((r) => setTimeout(r));
      await fixture.whenStable();
      fixture.detectChanges();
    };
    return { fixture, el, press };
  }
  const closedDoor: DoorSquare = { col: 4, row: 2, state: 2, axis: 'h' };

  it("has three choices as radios, 48 px or more, with what each means; the door's kind is checked", () => {
    const { el } = setup(closedDoor);
    expect(plain(el.querySelector('h2')?.textContent)).toBe('Porta');
    expect(plain(el.textContent)).toContain('Coluna 5, linha 3');
    const radios = Array.from(el.querySelectorAll<HTMLElement>('[role="radio"]'));
    expect(radios.map((r) => plain(r.querySelector('b')?.textContent))).toEqual([
      'Aberta',
      'Fechada',
      'Trancada',
    ]);
    expect(radios.map((r) => r.getAttribute('aria-checked'))).toEqual(['false', 'true', 'false']);
    expect(plain(radios[2].textContent)).toContain('Só você a destranca');
    expect(plain(el.textContent)).toContain('A mudança vale na hora, para todos.');
  });

  it('paints the choice at once, in the doors layer, keeps the sheet open with the new kind checked, and "Pronto" closes it saying it changed', async () => {
    const { el, press } = setup(closedDoor);
    await press('Trancada');
    expect(api.paints).toEqual([
      { layer: MapLayer.DOORS, value: 3, squares: [{ col: 4, row: 2 }] },
    ]);
    expect(el.querySelector('[role="radio"][aria-checked="true"] b')?.textContent).toBe('Trancada');
    expect(closed).toEqual([]);
    await press('Pronto');
    expect(closed).toEqual([true]);
  });

  it('paints the checked choice too: the door may have changed since the sheet opened', async () => {
    const door: DoorSquare = { ...closedDoor };
    const { press } = setup(door);
    // A player opened the door meanwhile; the sheet still shows it closed.
    (door as { state: number }).state = 1;
    await press('Fechada');
    expect(api.paints).toEqual([
      { layer: MapLayer.DOORS, value: 2, squares: [{ col: 4, row: 2 }] },
    ]);
  });

  it('says why when the server refused, and keeps the old kind', async () => {
    const { el, press } = setup(closedDoor);
    api.paint = () => Promise.reject(new ConnectError('x', Code.PermissionDenied));
    await press('Aberta');
    expect(el.querySelector('[role="alert"]')?.textContent).toContain(
      'Só o mestre da campanha muda os mapas.',
    );
    expect(el.querySelector('[role="radio"][aria-checked="true"] b')?.textContent).toBe('Fechada');
  });

  it('a grade offers "Aberta" and "Grade" (checked), never "Fechada", and says that opening it makes it a door', () => {
    const { el } = setup({ col: 1, row: 1, state: 4, axis: 'v' });
    expect(Array.from(el.querySelectorAll('[role="radio"] b'), (b) => b.textContent)).toEqual([
      'Aberta',
      'Grade',
    ]);
    expect(el.querySelector('[role="radio"][aria-checked="true"] b')?.textContent).toBe('Grade');
    expect(plain(el.textContent)).toContain('Abrir a grade a troca por uma porta aberta.');
  });

  it('every sheet starts with its state checked, and only the checked radio is a tab stop (the arrow keys move the choice)', async () => {
    const { el, fixture } = setup(closedDoor);
    const radios = Array.from(el.querySelectorAll<HTMLElement>('[role="radio"]'));
    expect(radios.map((r) => r.tabIndex)).toEqual([-1, 0, -1]);
    radios[1].focus();
    radios[1].dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    await new Promise((r) => setTimeout(r));
    await fixture.whenStable();
    fixture.detectChanges();
    expect(api.paints).toEqual([
      { layer: MapLayer.DOORS, value: 3, squares: [{ col: 4, row: 2 }] },
    ]);
    expect(el.querySelector('[role="radio"][aria-checked="true"] b')?.textContent).toBe('Trancada');
  });

  it('a secret door has one filled button, which asks in place before it reveals: "Voltar" or "Revelar"', async () => {
    const { fixture, el, press } = setup({ col: 7, row: 4, state: 5, axis: 'h' });
    expect(plain(el.querySelector('h2')?.textContent)).toBe('Porta secreta');
    expect(el.querySelectorAll('[role="radio"]')).toHaveLength(0);
    await press('Revelar a porta secreta');
    expect(plain(el.querySelector('h2')?.textContent)).toBe('Revelar a porta secreta?');
    expect(plain(el.textContent)).toContain('Os jogadores vão ver a porta. Revelar?');
    expect(api.paints).toEqual([]);
    await press('Voltar');
    expect(plain(el.querySelector('h2')?.textContent)).toBe('Porta secreta');
    await press('Revelar a porta secreta');
    await press('Revelar');
    fixture.detectChanges();
    expect(api.paints).toEqual([
      { layer: MapLayer.DOORS, value: 2, squares: [{ col: 7, row: 4 }] },
    ]);
    expect(closed).toEqual([true]);
  });

  it('reveals through the wall too, when a wall is painted under the secret door (it would still be a wall)', async () => {
    const { press } = setup({ col: 7, row: 4, state: 5, axis: 'h' }, [{ col: 7, row: 4 }]);
    await press('Revelar a porta secreta');
    await press('Revelar');
    expect(api.paints.map((p) => `${p.layer}:${p.value}`)).toEqual([
      `${MapLayer.WALL}:0`,
      `${MapLayer.DOORS}:2`,
    ]);
  });

  it("clears the wall under every square of the door's block on a calibrated map", async () => {
    const block = [
      { col: 6, row: 4 },
      { col: 7, row: 4 },
      { col: 6, row: 5 },
      { col: 7, row: 5 },
    ];
    const { press } = setup({ col: 7, row: 4, state: 5, axis: 'h' }, block);
    await press('Revelar a porta secreta');
    await press('Revelar');
    expect(api.paints[0]).toEqual({ layer: MapLayer.WALL, value: 0, squares: block });
  });
});
