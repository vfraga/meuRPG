// Finding U16-14 in review/unit-16-web-content-campaigns.md
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { BehaviorSubject } from 'rxjs';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ContentSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableContentRefusalSchema,
  TableContentViolationSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { TableContentClient } from '../../../core/content/content-client';
import {
  classDefaults,
  fakeContentWatcher,
  menuResponse,
  mirathel,
} from '../../../core/content/content-testing';
import { ContentEntry } from './content-entry';

describe('Review16 U16-14: refused unarchive reads as a connection problem', () => {
  it('does not tell the master to check the connection when the server refuses the unarchive', async () => {
    const refusal = create(TableContentRefusalSchema, {
      violations: [
        create(TableContentViolationSchema, {
          field: 'table_class.name_pt',
          reason: 'duplicate_name',
        }),
      ],
    });
    const unarchive = vi
      .fn()
      .mockRejectedValue(
        new ConnectError('x', Code.InvalidArgument, undefined, [
          { desc: TableContentRefusalSchema, value: refusal },
        ]),
      );
    const watcher = fakeContentWatcher();
    TestBed.resetTestingModule();
    TestBed.overrideComponent(ContentEntry, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: new BehaviorSubject(
              convertToParamMap({ id: 'camp-1', key: 'class:bardo-das-cinzas@mesa' }),
            ),
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
                myRole: Role.MASTER,
                awaitingApproval: false,
              },
            }),
          },
        },
        {
          provide: TableContentClient,
          useValue: {
            list: vi.fn().mockResolvedValue({ entries: mirathel(), tableRevision: 7 }),
            catalog: vi.fn().mockResolvedValue(create(ContentSchema, {})),
            effectMenu: vi.fn().mockResolvedValue(menuResponse()),
            classDefaults: vi.fn().mockResolvedValue(classDefaults()),
            unarchive,
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
    const btn = Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      (b.textContent ?? '').includes('Desarquivar'),
    )!;
    btn.click();
    await settle();
    expect(unarchive).toHaveBeenCalled();
    const t = (el.textContent ?? '').replace(/\s+/g, ' ');
    expect(t).toContain('Não foi possível desarquivar');
    expect(t).not.toContain('Confira a conexão');
  });
});
