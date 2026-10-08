import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of } from 'rxjs';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { FakeMapsClient, mapMessage, mapResponse } from '../../../core/maps/maps-testing';
import { MapsClient } from '../../../core/maps/maps-client';
import { MapGrid } from './map-grid';

describe('MapGrid, the older grid page, on a calibrated map (RN-25)', () => {
  async function setup(map: ReturnType<typeof mapMessage>) {
    const api = new FakeMapsClient();
    api.responses.set(map.id, mapResponse(map));
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: MapsClient, useValue: api },
        {
          provide: CampaignsService,
          useValue: {
            getCampaign: () =>
              Promise.resolve({
                campaign: {
                  id: 'camp-1',
                  name: 'Mirathel',
                  myRole: Role.MASTER,
                  awaitingApproval: false,
                },
              }),
          },
        },
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: of(convertToParamMap({ id: 'camp-1', mapId: map.id })),
            queryParamMap: of(convertToParamMap({})),
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(MapGrid);
    fixture.detectChanges();
    for (let i = 0; i < 3; i++) {
      await new Promise((r) => setTimeout(r));
      await fixture.whenStable();
      fixture.detectChanges();
    }
    return { fixture, el: fixture.nativeElement as HTMLElement, api };
  }
  const text = (el: HTMLElement) =>
    (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');

  it('says what the marked square is worth, caps the columns at what 200 rules columns allow, and keeps the factor on save', async () => {
    // Factor 4 (6 m): the drawing has at most 50 columns, and each square of it is 6 m.
    const { fixture, el, api } = await setup(
      mapMessage('map-4', 'Torre', {
        gridColumns: 80,
        gridRows: 52,
        drawnColumns: 20,
        drawnRows: 13,
        squareFactor: 4,
      }),
    );
    expect(text(el)).toContain('O quadrado marcado tem 6 m.');
    expect(text(el)).toContain('De 5 a 50.');
    const input = el.querySelector<HTMLInputElement>('input')!;
    expect(input.value).toBe('20');
    input.value = '51';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(text(el)).toContain('Use um número inteiro de 5 a 50.');
    input.value = '30';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    // The rules' grid is the drawing's times 4: 120 × 80 squares.
    expect(text(el)).toContain('120 × 80');
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Salvar'))
      ?.click();
    await fixture.whenStable();
    expect(api.calls.some((c) => c === 'setGrid map-4 30 x4')).toBe(true);
  });

  it('a map never calibrated still says 1,5 m', async () => {
    const { el } = await setup(
      mapMessage('map-1', 'Caverna', {
        gridColumns: 24,
        gridRows: 16,
        drawnColumns: 24,
        drawnRows: 16,
        squareFactor: 1,
      }),
    );
    expect(text(el)).toContain('O quadrado marcado tem 1,5 m.');
    expect(text(el)).toContain('De 5 a 60.');
  });

  it('does not send the factor it read when the page opened after another tab recalibrated the map', async () => {
    const { fixture, el, api } = await setup(
      mapMessage('map-4', 'Torre', {
        gridColumns: 80,
        gridRows: 52,
        drawnColumns: 20,
        drawnRows: 13,
        squareFactor: 4,
      }),
    );
    // Another tab calibrates the map to 6 (9 m) while this page is open.
    api.responses.set(
      'map-4',
      mapResponse(
        mapMessage('map-4', 'Torre', {
          gridColumns: 120,
          gridRows: 78,
          drawnColumns: 20,
          drawnRows: 13,
          squareFactor: 6,
        }),
      ),
    );
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Salvar'))
      ?.click();
    await new Promise((r) => setTimeout(r));
    await fixture.whenStable();
    fixture.detectChanges();
    expect(api.calls.some((c) => c.startsWith('setGrid'))).toBe(false);
    // The page now shows the map as it is, and asks to be checked again.
    expect(text(el)).toContain('O quadrado marcado tem 9 m.');
    expect(text(el)).toContain('A calibração do mapa mudou em outra janela');
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Salvar'))
      ?.click();
    await new Promise((r) => setTimeout(r));
    await fixture.whenStable();
    expect(api.calls.some((c) => c === 'setGrid map-4 20 x6')).toBe(true);
  });
});
