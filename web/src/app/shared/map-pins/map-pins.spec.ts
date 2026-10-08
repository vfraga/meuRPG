import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';

import { MapPointKind, MapPointSchema, TrapState } from '../../../gen/meurpg/maps/v1/maps_pb';
import { MapPins } from './map-pins';

const trap = (id: string, state = TrapState.ARMED, extra = {}) =>
  create(MapPointSchema, {
    id,
    kind: MapPointKind.TRAP,
    xBp: 4792,
    yBp: 4688,
    trap: { state, areaSize: 2 },
    ...extra,
  });

function draw(points: ReturnType<typeof trap>[], master: boolean) {
  const fixture = TestBed.createComponent(MapPins);
  fixture.componentRef.setInput('points', points);
  fixture.componentRef.setInput('columns', 24);
  fixture.componentRef.setInput('rows', 16);
  fixture.componentRef.setInput('isMaster', master);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('MapPins', () => {
  it('draws the same trap in every state, and only the master gets the crossed eye on an unrevealed one', () => {
    for (const state of [TrapState.ARMED, TrapState.TRIGGERED, TrapState.DISARMED]) {
      const el = draw([trap('a', state, { revealed: true })], false);
      expect(el.querySelectorAll('.area')).toHaveLength(1);
      expect(el.querySelector('.area__eye')).toBeNull();
    }
    expect(draw([trap('a')], true).querySelector('.area__eye')).toBeTruthy();
    expect(draw([trap('a')], false).querySelector('.area__eye')).toBeNull();
  });

  it('puts a chest beside the trap on its square, hidden dashed and found solid, and the light only for the master', () => {
    const chest = create(MapPointSchema, {
      id: 'c',
      kind: MapPointKind.TREASURE,
      xBp: 4792,
      yBp: 4688,
    });
    const found = create(MapPointSchema, {
      id: 'f',
      kind: MapPointKind.TREASURE,
      xBp: 100,
      yBp: 100,
      treasureFoundAt: timestampFromDate(new Date()),
    });
    const light = create(MapPointSchema, { id: 'l', kind: MapPointKind.LIGHT, xBp: 200, yBp: 200 });
    const el = draw([trap('a'), chest, found, light] as never, true);
    const pins = Array.from(el.querySelectorAll<HTMLElement>('.pin'));
    expect(pins.map((p) => p.className)).toEqual(
      expect.arrayContaining([
        expect.stringContaining('pin--hidden'),
        expect.stringContaining('pin--found'),
        expect.stringContaining('pin--light'),
      ]),
    );
    expect(pins[0].style.getPropertyValue('--n')).toBe('1'); // beside the trap's glyph, not on it
    expect(draw([light] as never, false).querySelectorAll('.pin')).toHaveLength(0);
  });

  it('never draws for a player what is still hidden, even if the read sent it', () => {
    const chest = create(MapPointSchema, {
      id: 'c',
      kind: MapPointKind.TREASURE,
      xBp: 100,
      yBp: 100,
    });
    const known = trap('known', TrapState.TRIGGERED);
    const hidden = [trap('hidden'), chest] as never;
    expect(draw(hidden, false).querySelectorAll('.area, .pin')).toHaveLength(0);
    expect(draw([...(hidden as []), known] as never, false).querySelectorAll('.area')).toHaveLength(
      1,
    );
    expect(draw(hidden, true).querySelectorAll('.area, .pin').length).toBeGreaterThan(0);
  });
});
