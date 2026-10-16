import { TestBed } from '@angular/core/testing';
import { describe, expect, it } from 'vitest';

import { EncounterStatus, type Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../combat/combat-client';
import { CombatOnMap } from './combat-on-map';
import { fakeCombatClient } from '../combat/combat-client-testing';

function lookup(answer: () => Promise<Encounter | null>): CombatOnMap {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [{ provide: CombatClient, useValue: fakeCombatClient({ get: answer }) }],
  });
  return TestBed.inject(CombatOnMap);
}

const encounter = (mapId: string, status: EncounterStatus) => ({ mapId, status }) as Encounter;

describe('CombatOnMap', () => {
  it('is true for a combat that has not ended on this map', async () => {
    expect(
      await lookup(async () => encounter('map-1', EncounterStatus.ACTIVE)).running(
        'camp-1',
        'map-1',
      ),
    ).toBe(true);
    expect(
      await lookup(async () => encounter('map-1', EncounterStatus.SETUP)).running(
        'camp-1',
        'map-1',
      ),
    ).toBe(true);
  });

  it('is false for an ended combat, a combat on another map, no combat and a failed read', async () => {
    expect(
      await lookup(async () => encounter('map-1', EncounterStatus.ENDED)).running(
        'camp-1',
        'map-1',
      ),
    ).toBe(false);
    expect(
      await lookup(async () => encounter('map-2', EncounterStatus.ACTIVE)).running(
        'camp-1',
        'map-1',
      ),
    ).toBe(false);
    expect(await lookup(async () => null).running('camp-1', 'map-1')).toBe(false);
    expect(
      await lookup(async () => {
        throw new Error('no open session');
      }).running('camp-1', 'map-1'),
    ).toBe(false);
  });
});
