// Finding U11-06: ExperienceStore.load(B) after load(A) on the same instance keeps A's
// rows/awards under B, and if B's read fails the state never leaves 'loading'.
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterExperienceSchema,
  GetCampaignExperienceResponseSchema,
  ListXPAwardsResponseSchema,
  XPAwardMode,
  XPAwardSchema,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import { RosterClient } from '../maps/roster-client';
import { ExperienceStore } from './experience-store';
import { ProgressionClient } from './progression-client';

describe('Review11 U11-06: ExperienceStore reused across campaigns', () => {
  const api = { experience: vi.fn(), listAwards: vi.fn() };
  const roster = { list: vi.fn() };

  function store(): ExperienceStore {
    TestBed.configureTestingModule({
      providers: [
        ExperienceStore,
        { provide: ProgressionClient, useValue: api },
        { provide: RosterClient, useValue: roster },
      ],
    });
    return TestBed.inject(ExperienceStore);
  }

  beforeEach(() => {
    api.experience.mockReset().mockResolvedValue(
      create(GetCampaignExperienceResponseSchema, {
        xpMode: XpMode.ENEMIES,
        characters: [
          create(CharacterExperienceSchema, { characterId: 'a1', name: 'HeroOfA', level: 3 }),
        ],
      }),
    );
    api.listAwards.mockReset().mockResolvedValue(
      create(ListXPAwardsResponseSchema, {
        awards: [create(XPAwardSchema, { id: 'award-a', mode: XPAwardMode.XP_AWARD_MODE_MANUAL })],
        nextPageToken: 'token-a',
      }),
    );
    roster.list.mockReset().mockResolvedValue([]);
  });

  it('shows an error, not a spinner, when campaign B fails after A loaded', async () => {
    const s = store();
    await s.load('A', true);
    expect(s.rowsState()).toBe('ready');

    api.experience.mockRejectedValue(new Error('boom'));
    api.listAwards.mockRejectedValue(new Error('boom'));
    await s.load('B', true);

    expect(s.rowsState()).toBe('error');
    expect(s.awardsState()).toBe('error');
  });

  it("does not expose A's rows, awards or page token as B's while B loads", async () => {
    const s = store();
    await s.load('A', true);

    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    api.experience.mockImplementation(async () => {
      await gate;
      return create(GetCampaignExperienceResponseSchema, { xpMode: XpMode.ENEMIES });
    });
    const pending = s.load('B', true);

    expect(s.rowsState()).toBe('loading');
    expect(s.rows().map((r) => r.name)).toEqual([]);
    expect(s.awards()).toEqual([]);
    expect(s.nextPageToken()).toBe('');

    release();
    await pending;
  });
});
