// Finding U16-04 in review/unit-16-web-content-campaigns.md
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { BehaviorSubject } from 'rxjs';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ContentSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { TableContentClient } from '../../../core/content/content-client';
import {
  classDefaults,
  fakeContentWatcher,
  menuResponse,
  mirathel,
} from '../../../core/content/content-testing';
import { ContentEntry } from './content-entry';

describe('Review16 U16-04: refresh keeps the old body for a reading player', () => {
  const KEY = 'race:corujeiro@mesa';

  it('a player who has the entry open reads the new name after content_changed bumps its revision', async () => {
    const list = vi.fn().mockResolvedValue({ entries: mirathel(), tableRevision: 7 });
    const watcher = fakeContentWatcher();
    TestBed.resetTestingModule();
    TestBed.overrideComponent(ContentEntry, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: new BehaviorSubject(convertToParamMap({ id: 'camp-1', key: KEY })),
            snapshot: { queryParamMap: convertToParamMap({}) },
          },
        },
        {
          provide: CampaignsService,
          useValue: {
            getCampaign: vi.fn().mockResolvedValue({
              campaign: {
                id: 'camp-1',
                name: 'Mirathel',
                myRole: Role.PLAYER,
                awaitingApproval: false,
              },
            }),
          },
        },
        {
          provide: TableContentClient,
          useValue: {
            list,
            catalog: vi.fn().mockResolvedValue(create(ContentSchema, {})),
            effectMenu: vi.fn().mockResolvedValue(menuResponse()),
            classDefaults: vi.fn().mockResolvedValue(classDefaults()),
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(ContentEntry);
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        fixture.detectChanges();
        await new Promise((r) => setTimeout(r));
        await fixture.whenStable();
      }
      fixture.detectChanges();
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('h1')!.textContent).toContain('Corujeiro');

    list.mockResolvedValue({
      entries: mirathel().map((e) =>
        e.key === KEY ? ({ ...e, namePt: 'Corujeiro Renomeado', revision: 9 } as typeof e) : e,
      ),
      tableRevision: 9,
    });
    watcher.hint();
    await settle();
    expect(el.querySelector('h1')!.textContent).toContain('Corujeiro Renomeado');
  });
});
