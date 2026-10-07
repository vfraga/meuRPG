// Finding U16-05 in review/unit-16-web-content-campaigns.md
import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { CampaignSchema, Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  ListSpellsResponseSchema,
  SpellDetailsSchema,
  SpellSchema,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { fakeContentWatcher } from '../../core/content/content-testing';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import { RosterClient } from '../../core/maps/roster-client';
import { Spells } from './spells';

const flat = (n: Element | null) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

describe('Review16 U16-05: the open spell card refreshes after content_changed with the real SpellsClient', () => {
  const originalMatchMedia = window.matchMedia;
  beforeEach(() => {
    window.matchMedia = ((q: string) => ({
      matches: q.includes('1100'),
      media: q,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    })) as unknown as typeof window.matchMedia;
  });
  afterEach(() => {
    window.matchMedia = originalMatchMedia;
  });

  it('says the spell is not available once the master switched it off', async () => {
    const key = 'spell:table-spell:foo';
    let version = 'v1';
    let off = false;
    const watcher = fakeContentWatcher();
    const transport = {
      unary: async (method: { name: string }) => {
        let message: unknown;
        if (method.name === 'ListSpells') {
          message = create(ListSpellsResponseSchema, {
            spells: off ? [] : [create(SpellSchema, { key, namePt: 'Foo', name: 'Foo', level: 1 })],
            total: off ? 0 : 1,
            contentVersion: version,
          });
        } else if (method.name === 'GetSpellDetails') {
          if (off) {
            throw new ConnectError('gone', Code.NotFound);
          }
          message = {
            spell: create(SpellDetailsSchema, {
              spell: { key, namePt: 'Foo', name: 'Foo', level: 1, schoolNamePt: 'Evocação' },
              description: ['Texto completo da magia.'],
            }),
          };
        } else {
          message = { content: { classes: [], spells: [] } };
        }
        return {
          stream: false,
          service: {},
          method,
          header: new Headers(),
          trailer: new Headers(),
          message,
        };
      },
    };
    TestBed.overrideComponent(Spells, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([{ path: 'campaigns/:id/spells', component: Spells }]),
        { provide: CONNECT_TRANSPORT, useValue: transport },
        {
          provide: CampaignsService,
          useValue: {
            getCampaign: async () => ({
              campaign: create(CampaignSchema, {
                id: 'camp-1',
                name: 'Mirathel',
                myRole: Role.PLAYER,
              }),
            }),
          },
        },
        { provide: RosterClient, useValue: { list: async () => [] } },
      ],
    });
    const harness = await RouterTestingHarness.create();
    await harness.navigateByUrl(`/campaigns/camp-1/spells?spell=${key}`, Spells);
    const settle = async () => {
      for (let i = 0; i < 6; i++) {
        harness.detectChanges();
        await harness.fixture.whenStable();
        await new Promise((r) => setTimeout(r, 5));
      }
    };
    await settle();
    const el = harness.routeNativeElement as HTMLElement;
    expect(flat(el)).toContain('Texto completo da magia.');

    // The master switches the spell off: the next answers carry another content version.
    off = true;
    version = 'v2';
    watcher.hint();
    await settle();
    await settle();

    expect(el.querySelectorAll('button.row')).toHaveLength(0);
    expect(flat(el)).toContain('Esta magia não está disponível.');
    expect(flat(el)).not.toContain('Texto completo da magia.');
  });
});
