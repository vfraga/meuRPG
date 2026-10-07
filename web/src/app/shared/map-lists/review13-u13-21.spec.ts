// Finding U13-21 (review/unit-13-web-live-rest.md): a creature's row in "Tokens no mapa" offers Esconder, which hides the OWNER's token (same characterId).
import { TestBed } from '@angular/core/testing';

import { MapsClient } from '../../core/maps/maps-client';
import { MapState } from '../../core/maps/map-state';
import { FakeMapsClient, mapMessage, mapResponse, mapToken } from '../../core/maps/maps-testing';
import { SessionTokens } from '../../pages/live-session/session-tokens/session-tokens';

describe('Review13 U13-21: creature row must not toggle the owner token', () => {
  async function mount() {
    const api = new FakeMapsClient();
    TestBed.configureTestingModule({ providers: [{ provide: MapsClient, useValue: api }] });
    const state = new MapState(async () =>
      mapResponse(
        mapMessage('map-1', 'Mirathel'),
        [],
        [mapToken('p', 'Pensantus'), mapToken('p', 'Corvo', { creatureId: 'raven' })],
      ),
    );
    await state.open('map-1');
    const fixture = TestBed.createComponent(SessionTokens);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('state', state);
    fixture.detectChanges();
    return { api, state, fixture, el: fixture.nativeElement as HTMLElement };
  }

  it('control: the owner row hides the owner token', async () => {
    const { api, el, fixture } = await mount();
    el.querySelectorAll<HTMLButtonElement>('li button')[0].click();
    await fixture.whenStable();
    expect(api.calls).toContain('setTokenHidden map-1 p true');
  });

  it('the creature row does not call setTokenHidden for the owner', async () => {
    const { api, state, el, fixture } = await mount();
    const rows = el.querySelectorAll('li');
    expect(rows[1].textContent).toContain('Corvo');
    rows[1].querySelector<HTMLButtonElement>('button')?.click();
    await fixture.whenStable();
    expect(api.calls.filter((c) => c.startsWith('setTokenHidden'))).toEqual([]);
    expect(state.tokens().find((t) => !t.creatureId)?.hidden).toBe(false);
  });

  it('rows have distinct track keys (no duplicate-key warning)', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    const { fixture } = await mount();
    fixture.detectChanges();
    const dup = warn.mock.calls.filter((c) => String(c[0]).includes('NG0955'));
    warn.mockRestore();
    expect(dup).toEqual([]);
  });
});
