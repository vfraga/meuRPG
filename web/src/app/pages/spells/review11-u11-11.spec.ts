// Finding U11-11 (relates to RN-10): after a `content_changed` the open spell card must show the master's edit.
import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';

import { CampaignSchema, Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  GetSpellDetailsResponseSchema,
  ListContentResponseSchema,
  ListSpellsResponseSchema,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import { fakeContentWatcher } from '../../core/content/content-testing';
import { RosterClient } from '../../core/maps/roster-client';
import { Spells } from './spells';

const flat = (n: Element | null) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

describe('Review11 U11-11: open spell card is stale after content_changed', () => {
  it('shows the edited spell text once the master changes it', async () => {
    let version = 'v1';
    let name = 'Mãos Flamejantes';
    const transport = {
      unary: async (method: { name: string }) => {
        const message =
          method.name === 'ListSpells'
            ? create(ListSpellsResponseSchema, { spells: [], nextPageToken: '', total: 0, contentVersion: version })
            : method.name === 'GetSpellDetails'
              ? create(GetSpellDetailsResponseSchema, {
                  spell: { spell: { key: 'spell:burning-hands', namePt: name, name: 'Burning Hands', level: 1 } },
                })
              : create(ListContentResponseSchema, {});
        return { stream: false, service: {}, method, header: new Headers(), trailer: new Headers(), message };
      },
    };
    const watcher = fakeContentWatcher();
    TestBed.overrideComponent(Spells, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([{ path: 'campaigns/:id/spells', component: Spells }]),
        { provide: CONNECT_TRANSPORT, useValue: transport },
        {
          provide: CampaignsService,
          useValue: {
            getCampaign: async () => ({
              campaign: create(CampaignSchema, { id: 'camp-1', name: 'Mirathel', myRole: Role.PLAYER }),
            }),
          },
        },
        { provide: RosterClient, useValue: { list: async () => [] } },
      ],
    });
    const harness = await RouterTestingHarness.create();
    await harness.navigateByUrl('/campaigns/camp-1/spells?spell=spell:burning-hands', Spells);
    const settle = async () => {
      for (let i = 0; i < 6; i++) {
        harness.detectChanges();
        await harness.fixture.whenStable();
        await new Promise((r) => setTimeout(r));
      }
    };
    await settle();
    const el = harness.routeNativeElement as HTMLElement;
    expect(flat(el.querySelector('#spell-card-title'))).toBe('Mãos Flamejantes');

    // The master edits the spell: the server now answers another version and other text.
    version = 'v2';
    name = 'Mãos Flamejantes (editada)';
    watcher.hint();
    await settle();

    expect(flat(el.querySelector('#spell-card-title'))).toBe('Mãos Flamejantes (editada)');
  });
});
