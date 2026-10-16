import { describe, expect, it } from 'vitest';

import { MapPointKind } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { newPointBody } from './new-point';

describe('new point', () => {
  const at = { xBp: 10, yBp: 20 };
  const presets = {
    lights: { list: async () => [{ key: 'light:torch' }] },
    traps: { list: async () => ({ presets: [{ findDc: 14 }], severities: [] }) },
  } as never;
  const none = {
    lights: { list: async () => [] },
    traps: { list: async () => ({ presets: [], severities: [] }) },
  } as never;

  it('starts a trap one square wide and a scene with only its name and place', async () => {
    expect(await newPointBody(MapPointKind.TRAP, at, 'c', presets)).toMatchObject({
      trap: { findDc: 14, areaSize: 1 },
    });
    expect(await newPointBody(MapPointKind.SCENE, at, 'c', presets)).toEqual({
      kind: MapPointKind.SCENE,
      name: 'Nova cena',
      description: '',
      xBp: 10,
      yBp: 20,
    });
  });

  it('falls back to an empty preset and a DC of 10 when the server lists none', async () => {
    expect(await newPointBody(MapPointKind.LIGHT, at, 'c', none)).toMatchObject({
      light: { presetKey: '' },
    });
    expect(await newPointBody(MapPointKind.TRAP, at, 'c', none)).toMatchObject({
      trap: { findDc: 10 },
    });
  });
});
