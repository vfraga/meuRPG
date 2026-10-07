// Finding U11-03: a local mutation made while a refresh() GetMap is in flight is
// overwritten by the older snapshot when that read lands last.
import type { GetMapResponse, MapPoint, MapToken } from '../../../gen/meurpg/maps/v1/maps_pb';
import { MapState } from './map-state';

function snapshot(revealed: boolean, tokenX: number): GetMapResponse {
  return {
    map: { id: 'a', name: 'Mapa a', pointCount: 1 },
    points: [{ id: 'p1', name: 'Taverna', revealed }],
    tokens: [{ characterId: 'c1', name: 'Pensantus', xBp: tokenX, yBp: 2000 }],
  } as unknown as GetMapResponse;
}

describe('Review11 U11-03: refresh() in flight overwrites later local mutations', () => {
  async function setup() {
    const pending: Array<(r: GetMapResponse) => void> = [];
    let first = true;
    const state = new MapState(() => {
      if (first) {
        first = false;
        return Promise.resolve(snapshot(false, 1000));
      }
      return new Promise<GetMapResponse>((resolve) => pending.push(resolve));
    });
    await state.open('a');
    const refreshing = state.refresh(); // map_changed: GetMap now in flight
    return { state, pending, refreshing };
  }

  it('keeps a point revealed locally while an older GetMap lands', async () => {
    const { state, pending, refreshing } = await setup();
    state.upsertPoint({ id: 'p1', name: 'Taverna', revealed: true } as unknown as MapPoint);
    pending[0](snapshot(false, 1000)); // pre-change snapshot arrives last
    await refreshing;
    expect((state.points()[0] as unknown as { revealed: boolean }).revealed).toBe(true);
  });

  it('keeps a token upserted/moved locally while an older GetMap lands', async () => {
    const { state, pending, refreshing } = await setup();
    state.upsertToken({ characterId: 'c1', name: 'Pensantus', xBp: 5000, yBp: 2000 } as unknown as MapToken);
    expect(state.moveToken('a', 'c1', 7000, 2000)).toBe(true);
    pending[0](snapshot(false, 1000));
    await refreshing;
    expect(state.tokens()[0].xBp).toBe(7000);
  });
});
