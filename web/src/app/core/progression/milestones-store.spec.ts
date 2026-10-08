import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { type Milestone, MilestoneSchema } from '../../../gen/meurpg/progression/v1/progression_pb';
import { MilestonesStore } from './milestones-store';
import { ProgressionClient } from './progression-client';

const milestone = (id: string, text: string): Milestone => create(MilestoneSchema, { id, text });

describe('MilestonesStore', () => {
  let answers: Map<string, { resolve: (m: Milestone[]) => void; reject: (e: unknown) => void }>;
  let store: MilestonesStore;

  beforeEach(() => {
    answers = new Map();
    TestBed.configureTestingModule({
      providers: [
        MilestonesStore,
        {
          provide: ProgressionClient,
          useValue: {
            listMilestones: (campaignId: string) =>
              new Promise((resolve, reject) => {
                answers.set(campaignId, {
                  resolve: (milestones) => resolve({ milestones }),
                  reject,
                });
              }),
          },
        },
      ],
    });
    store = TestBed.inject(MilestonesStore);
  });

  it('shows the milestones of the campaign it read, as ready', async () => {
    const loading = store.load('camp-1');
    answers.get('camp-1')!.resolve([milestone('m1', 'Chegar à torre')]);
    await loading;
    expect(store.list().map((m) => m.text)).toEqual(['Chegar à torre']);
    expect(store.state()).toBe('ready');
  });

  it('shows nothing of the last campaign while another one is read, and says so when that read fails', async () => {
    const first = store.load('camp-1');
    answers.get('camp-1')!.resolve([milestone('m1', 'Chegar à torre')]);
    await first;
    const second = store.load('camp-2');
    expect(store.list()).toEqual([]);
    expect(store.state()).toBe('loading');
    answers.get('camp-2')!.reject(new Error('down'));
    await second;
    expect(store.list()).toEqual([]);
    expect(store.state()).toBe('error');
  });

  it('keeps what is on screen while the same campaign is read again', async () => {
    const first = store.load('camp-1');
    answers.get('camp-1')!.resolve([milestone('m1', 'Chegar à torre')]);
    await first;
    const again = store.refresh();
    expect(store.list()).toHaveLength(1);
    answers.get('camp-1')!.resolve([milestone('m1', 'Chegar à torre'), milestone('m2', 'Sair')]);
    await again;
    expect(store.list()).toHaveLength(2);
  });
});
