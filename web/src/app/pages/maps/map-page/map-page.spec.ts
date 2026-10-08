import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';
import { BehaviorSubject } from 'rxjs';
import { describe, expect, it, vi } from 'vitest';

import {
  MapBlockedReason,
  MapBlockedSchema,
  MapSchema,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { CombatOnMap } from '../../../core/maps/combat-on-map';
import { MapsClient } from '../../../core/maps/maps-client';
import { OpenSessionLookup } from '../../../core/play/open-session';
import { MapPage } from './map-page';

describe('MapPage, "Apagar mapa"', () => {
  /** The refusal exactly as DeleteMap sends it: failed_precondition + MapBlocked detail. */
  function refusal(reason: MapBlockedReason): ConnectError {
    return new ConnectError('refused', Code.FailedPrecondition, undefined, [
      { desc: MapBlockedSchema, value: create(MapBlockedSchema, { reason }) },
    ]);
  }

  async function deleteMessage(reason: MapBlockedReason): Promise<string> {
    const api = { delete: vi.fn().mockRejectedValue(refusal(reason)), get: vi.fn() };
    TestBed.configureTestingModule({
      providers: [
        { provide: MapsClient, useValue: api },
        { provide: CampaignsService, useValue: {} },
        { provide: CombatOnMap, useValue: {} },
        { provide: OpenSessionLookup, useValue: {} },
        { provide: Router, useValue: { navigate: vi.fn() } },
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: new BehaviorSubject(convertToParamMap({})),
            queryParamMap: new BehaviorSubject(convertToParamMap({})),
          },
        },
      ],
    });
    const page = TestBed.createComponent(MapPage).componentInstance as unknown as {
      state: { setMap(m: unknown): void };
      campaignId: { set(v: string): void };
      deleteMap: () => Promise<void>;
    };
    page.campaignId.set('c1');
    page.state.setMap(create(MapSchema, { id: 'm1', name: 'Cripta' }));
    try {
      await page.deleteMap();
    } catch (e) {
      return (e as Error).message;
    }
    return '';
  }

  for (const [name, reason, words] of [
    ['COMBAT_RUNNING', MapBlockedReason.COMBAT_RUNNING, 'Há um combate neste mapa'],
    ['TREASURE_CONVERTED', MapBlockedReason.TREASURE_CONVERTED, 'virou XP'],
    ['TREASURE_FOUND', MapBlockedReason.TREASURE_FOUND, 'Desmarque antes de apagar'],
  ] as const) {
    it(`says why a refusal for ${name} cannot be done, not that the server is unreachable`, async () => {
      const message = await deleteMessage(reason);
      expect(message).toContain(words);
      expect(message).not.toMatch(/falar com o servidor/);
    });
  }
});
