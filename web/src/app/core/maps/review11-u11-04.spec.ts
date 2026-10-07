// Finding U11-04: MapReveals applies the RPC answer to whatever map is open
// when the answer arrives, not to the map the click was made on.
import { MapReveals } from './map-reveals';
import { MapState } from './map-state';
import { mapMessage, mapPoint, mapResponse, mapToken } from './maps-testing';

describe('Review11 U11-04: reveal answer lands on the map that is open now', () => {
  it('does not insert a point/token of map A into map B after a map switch', async () => {
    const state = new MapState(async (id) =>
      id === 'map-a'
        ? mapResponse(mapMessage('map-a', 'A'), [mapPoint('pa', 'Torre')], [mapToken('ca', 'Goblin')])
        : mapResponse(mapMessage('map-b', 'B', { pointCount: 1 }), [mapPoint('pb', 'Porto')], []),
    );
    await state.open('map-a');

    let resolvePoint!: (p: ReturnType<typeof mapPoint>) => void;
    let resolveToken!: (t: ReturnType<typeof mapToken>) => void;
    const api = {
      setPointRevealed: () =>
        new Promise<ReturnType<typeof mapPoint>>((r) => (resolvePoint = r)),
      setTokenHidden: () => new Promise<ReturnType<typeof mapToken>>((r) => (resolveToken = r)),
    };
    const reveals = new MapReveals(api as never, () => state, () => 'camp-1');

    const pending = reveals.togglePoint(state.points()[0], true);
    await state.open('map-b');
    resolvePoint(mapPoint('pa', 'Torre', { revealed: true }));
    await pending;

    expect(state.map()?.id).toBe('map-b');
    expect(state.points().map((p) => p.id)).toEqual(['pb']);
    expect(state.map()?.pointCount).toBe(1);

    // Same for tokens.
    await state.open('map-a');
    const pendingT = reveals.toggleToken(state.tokens()[0], true);
    await state.open('map-b');
    resolveToken(mapToken('ca', 'Goblin', { hidden: true }));
    await pendingT;
    expect(state.tokens()).toEqual([]);
  });
});
