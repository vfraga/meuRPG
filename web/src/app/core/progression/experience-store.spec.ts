import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterExperienceSchema,
  GetCampaignExperienceResponseSchema,
  ListTreasuresToConvertResponseSchema,
  ListXPAwardsResponseSchema,
  TreasureToConvertSchema,
  XPAwardMode,
  XPAwardSchema,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import { RosterClient } from '../maps/roster-client';
import { ExperienceStore } from './experience-store';
import { ProgressionClient } from './progression-client';

const nbsp = ' ';

function experience(xp: number) {
  return create(GetCampaignExperienceResponseSchema, {
    xpMode: XpMode.ENEMIES,
    characters: [
      create(CharacterExperienceSchema, {
        characterId: 'p1',
        name: 'Pensantus',
        playerDisplayName: 'Vinicius',
        level: 3,
        experiencePoints: xp,
        nextLevelXp: 2700,
        canLevelUp: xp >= 2700,
      }),
      create(CharacterExperienceSchema, {
        characterId: 't1',
        name: 'Toren',
        level: 3,
        experiencePoints: 2250,
        nextLevelXp: 2700,
      }),
    ],
  });
}

function award(id: string) {
  return create(XPAwardSchema, { id, mode: XPAwardMode.XP_AWARD_MODE_MANUAL, reason: id });
}

describe('ExperienceStore', () => {
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
    api.experience.mockReset().mockResolvedValue(experience(2600));
    api.listAwards.mockReset().mockResolvedValue(
      create(ListXPAwardsResponseSchema, {
        awards: [award('a2'), award('a1')],
        nextPageToken: 'next',
      }),
    );
    roster.list.mockReset().mockResolvedValue([
      {
        id: 'p1',
        name: 'Pensantus',
        kind: 1,
        playerUserId: 'u1',
        classSummary: 'Mago 3',
        raceName: 'Gnomo',
        playerName: null,
      },
    ]);
  });

  it('puts the XP, the class line and the player together', async () => {
    const s = store();
    await s.load('c1', true);

    expect(s.rowsState()).toBe('ready');
    expect(s.xpMode()).toBe(XpMode.ENEMIES);
    expect(s.rows()[0]).toMatchObject({
      id: 'p1',
      name: 'Pensantus',
      xp: 2600,
      nextLevelXp: 2700,
      canLevelUp: false,
    });
    expect(s.rows()[0].sub).toBe(`Mago 3${nbsp}· de Vinicius`);
    // A character the roster does not list still reads, with what the XP call knows.
    expect(s.rows()[1].sub).toBe('');
    expect(s.awards().map((a) => a.id)).toEqual(['a2', 'a1']);
    expect(s.nextPageToken()).toBe('next');
  });

  it('does not ask for the history when the host does not show it', async () => {
    const s = store();
    await s.load('c1', false);
    expect(api.listAwards).not.toHaveBeenCalled();
    await s.refresh();
    expect(api.listAwards).not.toHaveBeenCalled();
  });

  it('keeps the rows without the class line when the roster fails', async () => {
    roster.list.mockRejectedValue(new Error('x'));
    const s = store();
    await s.load('c1', false);
    expect(s.rowsState()).toBe('ready');
    expect(s.rows()[0].sub).toBe('de Vinicius');
  });

  it('says it failed, and keeps what it had on a later failure', async () => {
    api.experience.mockRejectedValue(new Error('x'));
    const s = store();
    await s.load('c1', false);
    expect(s.rowsState()).toBe('error');

    api.experience.mockResolvedValue(experience(2600));
    await s.refresh();
    expect(s.rowsState()).toBe('ready');
    api.experience.mockRejectedValue(new Error('x'));
    await s.refresh();
    expect(s.rowsState()).toBe('ready');
    expect(s.rows()).toHaveLength(2);
  });

  it('drops an answer that a newer read overtook', async () => {
    const s = store();
    let slow!: (v: unknown) => void;
    api.experience.mockReturnValueOnce(new Promise((resolve) => (slow = resolve)));
    const first = s.load('c1', false);
    api.experience.mockResolvedValueOnce(experience(2716));
    await s.refresh();
    expect(s.rows()[0].xp).toBe(2716);

    slow(experience(100)); // the old answer arrives last
    await first;
    expect(s.rows()[0].xp).toBe(2716);
  });

  it('reads the next page of the history and appends it', async () => {
    const s = store();
    await s.load('c1', true);
    api.listAwards.mockResolvedValueOnce(
      create(ListXPAwardsResponseSchema, { awards: [award('a0')], nextPageToken: '' }),
    );
    await s.more();

    expect(api.listAwards).toHaveBeenLastCalledWith('c1', 'next');
    expect(s.awards().map((a) => a.id)).toEqual(['a2', 'a1', 'a0']);
    expect(s.nextPageToken()).toBe('');
    // No token, nothing to read.
    api.listAwards.mockClear();
    await s.more();
    expect(api.listAwards).not.toHaveBeenCalled();
  });

  it('goes back to loading when the host loads another campaign', async () => {
    const s = store();
    await s.load('c1', true);
    const next = s.load('c2', true);
    expect(s.rowsState()).toBe('loading');
    expect(s.awardsState()).toBe('loading');
    await next;
    expect(api.experience).toHaveBeenLastCalledWith('c2');
  });

  it('shows an error, not a spinner, when another campaign fails after one loaded', async () => {
    const s = store();
    await s.load('c1', true);
    expect(s.rowsState()).toBe('ready');

    api.experience.mockRejectedValue(new Error('x'));
    api.listAwards.mockRejectedValue(new Error('x'));
    await s.load('c2', true);

    expect(s.rowsState()).toBe('error');
    expect(s.awardsState()).toBe('error');
  });

  it("does not show the last campaign's rows, awards or page token while another loads", async () => {
    const s = store();
    await s.load('c1', true);

    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    api.experience.mockImplementation(async () => {
      await gate;
      return experience(100);
    });
    const pending = s.load('c2', true);

    expect(s.rowsState()).toBe('loading');
    expect(s.rows()).toEqual([]);
    expect(s.awards()).toEqual([]);
    expect(s.nextPageToken()).toBe('');
    expect(s.xpMode()).toBe(XpMode.UNSPECIFIED);

    release();
    await pending;
    expect(s.rows()[0].xp).toBe(100);
  });

  it('drops the answer of the campaign it left', async () => {
    const s = store();
    let slow!: (v: unknown) => void;
    api.experience.mockReturnValueOnce(new Promise((resolve) => (slow = resolve)));
    const first = s.load('c1', false);
    api.experience.mockResolvedValueOnce(experience(2716));
    await s.load('c2', false);

    slow(experience(100)); // the old campaign answers last
    await first;
    expect(s.rows()[0].xp).toBe(2716);
  });

  describe('the treasures to convert (E9-09, master only)', () => {
    const found = create(TreasureToConvertSchema, {
      pointId: 'c',
      name: 'Baú de moedas',
      valuePo: 250,
    });

    function withTreasures(mode: XpMode) {
      api.experience.mockResolvedValue({ ...experience(2600), xpMode: mode });
      (api as Record<string, unknown>)['listTreasures'] = vi
        .fn()
        .mockResolvedValue(
          create(ListTreasuresToConvertResponseSchema, { treasures: [found], total: 130 }),
        );
      return (api as unknown as { listTreasures: ReturnType<typeof vi.fn> }).listTreasures;
    }

    it('reads them for the master in a campaign that counts XP, with the total the server holds', async () => {
      const listTreasures = withTreasures(XpMode.GOLD);
      const s = store();
      await s.load('camp-1', true, true);
      expect(listTreasures).toHaveBeenCalledWith('camp-1');
      expect(s.treasures().map((t) => t.name)).toEqual(['Baú de moedas']);
      expect(s.treasuresTotal()).toBe(130);
      expect(s.treasuresState()).toBe('ready');
    });

    it('never asks for a player, nor in a campaign by milestones', async () => {
      const listTreasures = withTreasures(XpMode.GOLD);
      await store().load('camp-1', true);
      expect(listTreasures).not.toHaveBeenCalled();
      TestBed.resetTestingModule();
      const miles = withTreasures(XpMode.MILESTONES);
      await store().load('camp-1', true, true);
      expect(miles).not.toHaveBeenCalled();
    });

    it('reads them again on refresh (after an award or an undo)', async () => {
      const listTreasures = withTreasures(XpMode.GOLD);
      const s = store();
      await s.load('camp-1', false, true);
      await s.refresh();
      expect(listTreasures).toHaveBeenCalledTimes(2);
    });

    it("forgets the last campaign's treasures when another one is loaded", async () => {
      withTreasures(XpMode.GOLD);
      const s = store();
      await s.load('camp-1', false, true);
      expect(s.treasures()).toHaveLength(1);
      (
        api as unknown as { listTreasures: ReturnType<typeof vi.fn> }
      ).listTreasures.mockRejectedValue(new Error('down'));
      await s.load('camp-2', false, true);
      expect(s.treasures()).toEqual([]);
      expect(s.treasuresTotal()).toBe(0);
      expect(s.treasuresState()).toBe('error');
    });

    it('leaves "loading" when a later read fails with an old list in hand (it shows the list)', async () => {
      const listTreasures = withTreasures(XpMode.GOLD);
      const s = store();
      await s.load('camp-1', false, true);
      listTreasures.mockRejectedValue(new Error('down'));
      await s.refresh();
      expect(s.treasuresState()).toBe('ready');
      expect(s.treasures()).toHaveLength(1);
    });

    it('can refresh the XP without the treasures', async () => {
      const listTreasures = withTreasures(XpMode.GOLD);
      const s = store();
      await s.load('camp-1', false, true);
      await s.refresh({ treasures: false });
      expect(listTreasures).toHaveBeenCalledTimes(1);
    });

    it('says it could not read them, and keeps what it had when a later read fails', async () => {
      const listTreasures = withTreasures(XpMode.GOLD);
      listTreasures.mockRejectedValueOnce(new Error('down'));
      const s = store();
      await s.load('camp-1', false, true);
      expect(s.treasuresState()).toBe('error');
      await s.loadTreasures();
      expect(s.treasuresState()).toBe('ready');
      listTreasures.mockRejectedValueOnce(new Error('down'));
      await s.loadTreasures();
      expect(s.treasures()).toHaveLength(1);
      expect(s.treasuresState()).toBe('ready');
    });
  });
});
