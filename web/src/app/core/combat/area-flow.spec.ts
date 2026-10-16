import { type MessageInitShape, create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  AreaPlacement,
  AreaTargetSchema,
  EncounterBlockedReason,
  EncounterBlockedSchema,
  PreviewSpellAreaResponseSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { AreaFlow, samePick } from './area-flow';
import type { AreaChoice } from './combat-client';
import { nextTurn } from '../../../test-settle';

const answer = (
  over: {
    origin?: { col: number; row: number };
    moved?: boolean;
    targets?: MessageInitShape<typeof AreaTargetSchema>[];
  } = {},
) =>
  create(PreviewSpellAreaResponseSchema, {
    origin: over.origin ?? { col: 6, row: 4 },
    squares: [{ col: 6, row: 4 }],
    targets: over.targets ?? [{ combatantId: 'g1', label: 'Goblin 1' }],
    coverCounts: true,
    moved: over.moved ?? false,
  });

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('AreaFlow', () => {
  it('asks the server once the point is placed, never before, and lists who is inside on confirming', async () => {
    const asked: (AreaChoice | null)[] = [];
    const flow = new AreaFlow(AreaPlacement.POINT, (area) => {
      asked.push(area);
      return Promise.resolve(answer());
    });
    expect(flow.ready()).toBe(false);
    expect(await flow.confirm()).toBe(false);
    expect(asked).toEqual([]);
    flow.place({ kind: 'point', square: { col: 6, row: 4 } });
    expect(asked).toEqual([{ origin: { col: 6, row: 4 } }]);
    expect(await flow.confirm()).toBe(true);
    expect(flow.step()).toBe('list');
    expect(asked).toHaveLength(1);
  });

  it('keeps only the answer for the latest place', async () => {
    const first = deferred<ReturnType<typeof answer>>();
    const second = deferred<ReturnType<typeof answer>>();
    const queue = [first, second];
    const flow = new AreaFlow(AreaPlacement.POINT, () => queue.shift()!.promise);
    flow.place({ kind: 'point', square: { col: 1, row: 1 } });
    flow.place({ kind: 'point', square: { col: 2, row: 2 } });
    second.resolve(answer({ origin: { col: 2, row: 2 } }));
    await Promise.resolve();
    first.resolve(answer({ origin: { col: 1, row: 1 } }));
    await nextTurn();
    expect(flow.preview()?.origin).toMatchObject({ col: 2, row: 2 });
  });

  it("casts at the server's point when a wall moved it, and goes back with the point where it was", async () => {
    const flow = new AreaFlow(AreaPlacement.POINT, () =>
      Promise.resolve(answer({ origin: { col: 3, row: 4 }, moved: true })),
    );
    flow.place({ kind: 'point', square: { col: 8, row: 4 } });
    await flow.confirm();
    expect(flow.choice()).toEqual({ origin: { col: 3, row: 4 } });
    flow.back();
    expect(flow.step()).toBe('place');
    expect(flow.placed()).toEqual({ kind: 'point', square: { col: 8, row: 4 } });
  });

  it('asks before casting when nobody the caster sees is inside', async () => {
    const flow = new AreaFlow(AreaPlacement.DIRECTION, () =>
      Promise.resolve(answer({ targets: [] })),
    );
    flow.place({ kind: 'direction', direction: { dx: 1, dy: 0 } });
    await flow.confirm();
    expect(flow.nobody()).toBe(true);
    expect(flow.choice()).toEqual({ direction: { dx: 1, dy: 0 } });
  });

  it('drops a point the server says is out of range, with the reason', async () => {
    const refused = new ConnectError('far', Code.FailedPrecondition, undefined, [
      {
        desc: EncounterBlockedSchema,
        value: create(EncounterBlockedSchema, {
          reason: EncounterBlockedReason.TARGET_OUT_OF_REACH,
          missingFt: 15,
        }),
      },
    ]);
    const flow = new AreaFlow(AreaPlacement.POINT, () => Promise.reject(refused));
    flow.place({ kind: 'point', square: { col: 30, row: 4 } });
    expect(await flow.confirm()).toBe(false);
    expect(flow.placed()).toBeNull();
    expect(flow.error()).toContain('Longe demais');
    expect(flow.step()).toBe('place');
  });

  it('starts at the list for a sphere around the caster, with no point', async () => {
    const asked: (AreaChoice | null)[] = [];
    const flow = new AreaFlow(AreaPlacement.CASTER, (area) => {
      asked.push(area);
      return Promise.resolve(answer());
    });
    await flow.start();
    expect(asked).toEqual([null]);
    expect(flow.step()).toBe('list');
  });

  it('tells a second tap on the same place from a new one', () => {
    expect(
      samePick(
        { kind: 'point', square: { col: 1, row: 2 } },
        { kind: 'point', square: { col: 1, row: 2 } },
      ),
    ).toBe(true);
    expect(
      samePick(
        { kind: 'direction', direction: { dx: 1, dy: 0 } },
        { kind: 'direction', direction: { dx: 1, dy: 1 } },
      ),
    ).toBe(false);
  });
});
