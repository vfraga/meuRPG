import { create } from '@bufbuild/protobuf';

import {
  AreaPlacement,
  PreviewSpellAreaResponseSchema,
  WallSide,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { AreaFlow } from './area-flow';
import type { AreaChoice } from './combat-client-types';
import { type WallChoice, askable, newWall, stepLength, switchShape } from './spell-wall';
import { nextTurn } from '../../../test-settle';

const answer = () =>
  create(PreviewSpellAreaResponseSchema, {
    origin: { col: 4, row: 3 },
    squares: [{ col: 4, row: 3 }],
    burningSquares: [{ col: 4, row: 4 }],
    lengthFt: 60,
  });

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function wallFlow() {
  const asked: { area: AreaChoice | null; wall: WallChoice | null }[] = [];
  const flow = new AreaFlow(
    AreaPlacement.POINT,
    (area, wall) => {
      asked.push({ area, wall });
      return Promise.resolve(answer());
    },
    true,
  );
  return { flow, asked };
}

describe('AreaFlow for a wall', () => {
  it('has no wall for any other area, and asks with none', async () => {
    const asked: (WallChoice | null)[] = [];
    const flow = new AreaFlow(AreaPlacement.POINT, (_area, wall) => {
      asked.push(wall);
      return Promise.resolve(answer());
    });
    expect(flow.wall()).toBeNull();
    flow.place({ kind: 'point', square: { col: 1, row: 1 } });
    await nextTurn();
    expect(asked).toEqual([null]);
  });

  it('starts as a line to the east and sends the wall with the point it asks about', async () => {
    const { flow, asked } = wallFlow();
    expect(flow.wall()).toEqual(newWall(false));
    flow.place({ kind: 'point', square: { col: 4, row: 3 } });
    await nextTurn();
    expect(asked).toEqual([
      { area: { origin: { col: 4, row: 3 } }, wall: askable(newWall(false)) },
    ]);
    expect(flow.preview()?.burningSquares).toHaveLength(1);
  });

  it('asks again on every change of the wall once a point is placed, and not before', async () => {
    const { flow, asked } = wallFlow();
    flow.setWall(stepLength(newWall(false), -1));
    await nextTurn();
    expect(asked).toEqual([]);
    flow.place({ kind: 'point', square: { col: 4, row: 3 } });
    await nextTurn();
    flow.setWall({ ...flow.wall()!, direction: { dx: 0, dy: 1 } });
    await nextTurn();
    flow.setWall({ ...flow.wall()!, side: WallSide.LEFT });
    await nextTurn();
    flow.setWall({ ...flow.wall()!, side: WallSide.RIGHT });
    await nextTurn();
    const base = { ...newWall(false), lengthFt: 55 };
    expect(asked.map((a) => a.wall)).toEqual([
      { ...base, side: WallSide.LEFT },
      { ...base, direction: { dx: 0, dy: 1 }, side: WallSide.LEFT },
      { ...base, direction: { dx: 0, dy: 1 }, side: WallSide.LEFT },
      { ...base, direction: { dx: 0, dy: 1 }, side: WallSide.RIGHT },
    ]);
  });

  it('is not ready, and cannot go to the list, until a side is picked, although the wall is previewed', async () => {
    const { flow, asked } = wallFlow();
    flow.place({ kind: 'point', square: { col: 4, row: 3 } });
    await nextTurn();
    expect(asked[0].wall?.side).toBe(WallSide.LEFT);
    expect(flow.ready()).toBe(false);
    expect(await flow.confirm()).toBe(false);
    expect(flow.step()).toBe('place');
    flow.setWall({ ...flow.wall()!, side: WallSide.RIGHT });
    expect(flow.ready()).toBe(true);
    await nextTurn();
    expect(await flow.confirm()).toBe(true);
  });

  it('a drag to another way takes the picked side back, a drag the same way keeps it', async () => {
    const { flow } = wallFlow();
    flow.setWall({ ...flow.wall()!, side: WallSide.RIGHT });
    flow.placeWall({ col: 2, row: 2 }, { dx: 1, dy: 0 });
    expect(flow.wall()?.side).toBe(WallSide.RIGHT);
    flow.placeWall({ col: 2, row: 2 }, { dx: 0, dy: 1 });
    expect(flow.wall()?.side).toBe(WallSide.UNSPECIFIED);
    expect(flow.ready()).toBe(false);
    await nextTurn();
  });

  it('says the wall stopped at a wall of the map when the server laid fewer squares than the open floor holds', async () => {
    const { flow } = wallFlow();
    flow.setWall({ ...flow.wall()!, lengthFt: 40 });
    flow.place({ kind: 'point', square: { col: 4, row: 3 } });
    await nextTurn();
    // The answer holds 1 square against the 8 of 12 m on open floor.
    expect(flow.stops()).toContain('A muralha para na parede');
    expect(flow.stops()).toContain('ficou com 1,5');
    flow.setWall({ ...flow.wall()!, lengthFt: 5 });
    await nextTurn();
    expect(flow.stops()).toBeNull();
  });

  it('a drag places the start and sets the direction with one question', async () => {
    const { flow, asked } = wallFlow();
    flow.placeWall({ col: 2, row: 2 }, { dx: 1, dy: 1 });
    await nextTurn();
    expect(asked).toHaveLength(1);
    expect(asked[0].area).toEqual({ origin: { col: 2, row: 2 } });
    expect(asked[0].wall?.direction).toEqual({ dx: 1, dy: 1 });
    expect(flow.placed()).toEqual({ kind: 'point', square: { col: 2, row: 2 } });
  });

  it('a ring has no direction in what it asks', async () => {
    const { flow, asked } = wallFlow();
    flow.setWall(switchShape(flow.wall()!, true));
    flow.place({ kind: 'point', square: { col: 5, row: 5 } });
    await nextTurn();
    expect(asked[0].wall).toEqual({
      ring: true,
      direction: null,
      lengthFt: 20,
      side: WallSide.INNER,
    });
    expect(flow.wall()?.side).toBe(WallSide.UNSPECIFIED);
  });

  it('keeps the wall when it goes back to the map, and cannot change it on the list', async () => {
    const { flow, asked } = wallFlow();
    flow.setWall({ ...flow.wall()!, side: WallSide.RIGHT });
    flow.place({ kind: 'point', square: { col: 4, row: 3 } });
    await nextTurn();
    await flow.confirm();
    flow.setWall({ ...flow.wall()!, side: WallSide.LEFT });
    await nextTurn();
    expect(flow.wall()?.side).toBe(WallSide.RIGHT);
    expect(asked).toHaveLength(1);
    flow.back();
    expect(flow.wall()?.side).toBe(WallSide.RIGHT);
  });

  it('keeps only the answer of the latest wall: a slow older answer does not overwrite the newer', async () => {
    const slow = deferred<ReturnType<typeof answer>>();
    const fast = deferred<ReturnType<typeof answer>>();
    const queue = [slow, fast];
    const flow = new AreaFlow(AreaPlacement.POINT, () => queue.shift()!.promise, true);
    flow.place({ kind: 'point', square: { col: 4, row: 3 } });
    flow.setWall({ ...flow.wall()!, side: WallSide.LEFT });
    const newer = create(PreviewSpellAreaResponseSchema, { lengthFt: 30 });
    fast.resolve(newer);
    await nextTurn();
    slow.resolve(create(PreviewSpellAreaResponseSchema, { lengthFt: 60 }));
    await nextTurn();
    expect(flow.preview()?.lengthFt).toBe(30);
    expect(flow.loading()).toBe(false);
  });

  it('a slow answer to the older wall cannot be confirmed over the newer one', async () => {
    const slow = deferred<ReturnType<typeof answer>>();
    const fast = deferred<ReturnType<typeof answer>>();
    const queue = [slow, fast];
    const flow = new AreaFlow(AreaPlacement.POINT, () => queue.shift()!.promise, true);
    flow.setWall({ ...flow.wall()!, side: WallSide.RIGHT });
    flow.placeWall({ col: 2, row: 2 }, { dx: 1, dy: 0 });
    flow.setWall({ ...flow.wall()!, lengthFt: 25 });
    const confirming = flow.confirm();
    slow.resolve(create(PreviewSpellAreaResponseSchema, { lengthFt: 60 }));
    fast.resolve(create(PreviewSpellAreaResponseSchema, { lengthFt: 25 }));
    expect(await confirming).toBe(true);
    expect(flow.preview()?.lengthFt).toBe(25);
  });
});
