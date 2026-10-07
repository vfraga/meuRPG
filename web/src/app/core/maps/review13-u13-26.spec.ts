// Finding U13-26 (review/unit-13-web-live-rest.md): a GetMap read in flight overwrites a newer token move/upsert with its older answer (moveToken/upsertToken do not bump the generation).
import type { GetMapResponse } from '../../../gen/meurpg/maps/v1/maps_pb';
import { MapState } from './map-state';

function response(id: string, x: number): GetMapResponse {
  return {
    map: { id, name: `Mapa ${id}`, pointCount: 0 },
    points: [],
    tokens: [{ characterId: 'c1', name: 'Pensantus', xBp: x, yBp: 2000 }],
  } as unknown as GetMapResponse;
}

describe('Review13 U13-26: a late GetMap read keeps the newer token position', () => {
  async function setup() {
    let resolveLate!: (r: GetMapResponse) => void;
    let calls = 0;
    const state = new MapState((id) => {
      calls++;
      if (calls === 1) {
        return Promise.resolve(response(id, 1000));
      }
      return new Promise<GetMapResponse>((resolve) => (resolveLate = resolve));
    });
    await state.open('a');
    const refreshing = state.refresh();
    return { state, refreshing, resolveLate: (r: GetMapResponse) => resolveLate(r) };
  }

  it('control: without a move in between, the read lands', async () => {
    const { state, refreshing, resolveLate } = await setup();
    resolveLate(response('a', 1000));
    await refreshing;
    expect(state.tokens()[0].xBp).toBe(1000);
  });

  it('keeps a stream move (token_moved) made while a read was in flight', async () => {
    const { state, refreshing, resolveLate } = await setup();
    state.moveToken('a', 'c1', 4000, 5000);
    resolveLate(response('a', 1000)); // served before the move committed
    await refreshing;
    expect(state.tokens()[0].xBp).toBe(4000);
  });

  it('keeps an optimistic upsert made while a read was in flight', async () => {
    const { state, refreshing, resolveLate } = await setup();
    state.upsertToken({ characterId: 'c1', name: 'Pensantus', xBp: 4000, yBp: 5000 } as never);
    resolveLate(response('a', 1000));
    await refreshing;
    expect(state.tokens()[0].xBp).toBe(4000);
  });
});
