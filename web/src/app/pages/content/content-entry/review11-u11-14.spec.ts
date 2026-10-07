// Finding U11-14: `refresh` (run on content_changed) re-reads the context but never the catalog, so a name that became
// visible after the hint still shows as its raw key; and it has no sequence guard, so an older read can land last and win.
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { BehaviorSubject } from 'rxjs';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ContentSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableContentKind,
  TableSubraceSchema,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { TableContentClient } from '../../../core/content/content-client';
import { entry, fakeContentWatcher } from '../../../core/content/content-testing';
import { ContentEntry } from './content-entry';

describe('Review11 U11-14: content-entry refresh keeps a stale catalog and has no sequence guard', () => {
  const RACE = 'race:corujeiro@mesa';
  const sub = () =>
    entry(TableContentKind.SUBRACE, 'Corujeiro Pálido', {
      body: {
        case: 'tableSubrace',
        value: create(TableSubraceSchema, { namePt: 'Corujeiro Pálido', raceKey: RACE }),
      },
    });
  const race = (name: string) =>
    entry(TableContentKind.RACE, name, { key: RACE });

  async function setup(list: ReturnType<typeof vi.fn>) {
    const watcher = fakeContentWatcher();
    const originalMatchMedia = window.matchMedia;
    window.matchMedia = (() => ({
      matches: false,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    })) as never;
    TestBed.resetTestingModule();
    TestBed.overrideComponent(ContentEntry, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: new BehaviorSubject(
              convertToParamMap({ id: 'camp-1', key: sub().key }),
            ),
            snapshot: { queryParamMap: convertToParamMap({}) },
          },
        },
        {
          provide: CampaignsService,
          useValue: {
            getCampaign: vi.fn().mockResolvedValue({
              campaign: { id: 'camp-1', name: 'Mirathel', myRole: Role.PLAYER, awaitingApproval: false },
            }),
          },
        },
        {
          provide: TableContentClient,
          useValue: { list, catalog: vi.fn().mockResolvedValue(create(ContentSchema, {})) },
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
    return { fixture, watcher, settle, restore: () => (window.matchMedia = originalMatchMedia) };
  }
  const text = (el: Element) => (el.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ');

  it('shows the name of a race that became visible after content_changed, not its raw key', async () => {
    const list = vi.fn().mockResolvedValue({ entries: [sub()], tableRevision: 1 });
    const { fixture, watcher, settle, restore } = await setup(list);
    expect(text(fixture.nativeElement)).toContain('Sub-raça de');
    list.mockResolvedValue({ entries: [sub(), race('Corujeiro')], tableRevision: 2 });
    watcher.hint();
    await settle();
    restore();
    const shown = text(fixture.nativeElement);
    expect(shown).not.toContain(RACE);
    expect(shown).toContain('Sub-raça de Corujeiro');
  });

  it('lets the newest read win when two content_changed reads finish out of order', async () => {
    const list = vi.fn().mockResolvedValueOnce({ entries: [sub()], tableRevision: 1 });
    const { fixture, watcher, settle, restore } = await setup(list);
    const deferred = () => {
      let resolve!: (v: unknown) => void;
      const p = new Promise((r) => (resolve = r));
      return { p, resolve };
    };
    const older = deferred();
    const newer = deferred();
    list.mockReturnValueOnce(older.p).mockReturnValueOnce(newer.p);
    watcher.hint();
    watcher.hint();
    await settle();
    newer.resolve({ entries: [sub(), race('Corujeiro Novo')], tableRevision: 3 });
    await settle();
    older.resolve({ entries: [sub(), race('Corujeiro Velho')], tableRevision: 2 });
    await settle();
    restore();
    const shown = text(fixture.nativeElement);
    expect(shown).not.toContain('Corujeiro Velho');
  });
});
