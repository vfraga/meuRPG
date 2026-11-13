import { TestBed } from '@angular/core/testing';

import { WallSide } from '../../../gen/meurpg/play/v1/combat_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { CombatClient } from './combat-client';
import { newWall } from './spell-wall';

function setup() {
  TestBed.configureTestingModule({
    providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(CombatClient);
  const previews: Record<string, unknown>[] = [];
  const casts: Record<string, unknown>[] = [];
  (client as unknown as { base: { client: unknown } }).base.client = {
    previewSpellArea: (req: Record<string, unknown>) => {
      previews.push(req);
      return Promise.resolve({ squares: [], targets: [] });
    },
    castSpell: (req: Record<string, unknown>) => {
      casts.push(req);
      return Promise.resolve({ encounter: { id: 'enc' }, cast: { id: 'cast' } });
    },
  };
  return { client, previews, casts };
}

describe('CombatClient: the wall of a spell', () => {
  it('sends the wall with the preview, with the point in origin, a ring without direction', async () => {
    const { client, previews } = setup();
    const line = {
      ...newWall(false),
      direction: { dx: 0, dy: -1 },
      lengthFt: 35,
      side: WallSide.RIGHT,
    };
    await client.casts.previewSpellArea(
      'c',
      'e',
      'p',
      'spell:wall-of-fire',
      { level: 4, pact: false },
      { origin: { col: 4, row: 3 } },
      '',
      line,
    );
    await client.casts.previewSpellArea(
      'c',
      'e',
      'p',
      'spell:wall-of-fire',
      null,
      { origin: { col: 1, row: 1 } },
      '',
      { ...newWall(true), side: WallSide.INNER },
    );
    expect(previews[0]).toMatchObject({
      area: { case: 'origin', value: { col: 4, row: 3 } },
      wall: { ring: false, direction: { dx: 0, dy: -1 }, lengthFt: 35, side: WallSide.RIGHT },
    });
    expect(previews[1]['wall']).toEqual({ ring: true, lengthFt: 20, side: WallSide.INNER });
  });

  it('sends no wall for a spell that lays none', async () => {
    const { client, previews, casts } = setup();
    await client.casts.previewSpellArea('c', 'e', 'p', 'spell:fireball', null, null);
    await client.casts.castSpell('c', 'e', 'p', 'spell:fireball', null, [], null, 'k');
    expect(previews[0]['wall']).toBeUndefined();
    expect(casts[0]['wall']).toBeUndefined();
  });

  it('sends the wall with the cast', async () => {
    const { client, casts } = setup();
    await client.casts.castSpell(
      'c',
      'e',
      'p',
      'spell:wall-of-fire',
      { level: 4, pact: false },
      [],
      null,
      'k',
      undefined,
      '',
      undefined,
      [],
      { area: { origin: { col: 4, row: 3 } }, wall: { ...newWall(false), side: WallSide.LEFT } },
    );
    expect(casts[0]).toMatchObject({
      area: { case: 'origin', value: { col: 4, row: 3 } },
      wall: { ring: false, direction: { dx: 1, dy: 0 }, lengthFt: 60, side: WallSide.LEFT },
    });
  });
});
