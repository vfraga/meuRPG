import type { GetMapResponse } from '../../../gen/meurpg/maps/v1/maps_pb';
import { MapState } from './map-state';
import { mapMessage, mapResponse, mapToken } from './maps-testing';

// Finding U10-5, see review/unit-10-web-core-stream.md
describe('Review10 U10-5: a map read in flight overwrites a token move applied in place', () => {
  it('keeps the moved position when an older read answers after token_moved', async () => {
    const pending: Array<(r: GetMapResponse) => void> = [];
    let first = true;
    const state = new MapState((id) => {
      if (first) {
        first = false;
        return Promise.resolve(
          mapResponse(mapMessage(id, 'M'), [], [mapToken('c1', 'Pensantus', { xBp: 1000, yBp: 2000 })]),
        );
      }
      return new Promise<GetMapResponse>((resolve) => pending.push(resolve));
    });
    await state.open('m');
    expect(state.status()).toBe('ready');

    // A read starts (e.g. map_changed) and the server answers with the OLD position later.
    const reading = state.refresh();
    expect(pending).toHaveLength(1);

    // token_moved arrives meanwhile and is applied in place.
    expect(state.moveToken('m', 'c1', 4000, 5000)).toBe(true);
    expect(state.tokens()[0]).toMatchObject({ xBp: 4000, yBp: 5000 });

    // The older read answers after the move, carrying the pre-move position.
    pending[0](
      mapResponse(mapMessage('m', 'M'), [], [mapToken('c1', 'Pensantus', { xBp: 1000, yBp: 2000 })]),
    );
    await reading;

    expect(state.tokens()[0]).toMatchObject({ xBp: 4000, yBp: 5000 });
  });
});
