// Finding U12-13: CombatXp keeps `just` after the award is undone elsewhere (xp_changed),
// so view() stays 'given', and give() reuses the same idempotency key for the same ids+encounter.
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';

import { XpMode } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantState,
  EncounterStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  AwardXPResponseSchema,
  CharacterExperienceSchema,
  GetCampaignExperienceResponseSchema,
  ListXPAwardsResponseSchema,
  XPAwardMode,
  XPAwardSchema,
} from '../../../../../gen/meurpg/progression/v1/progression_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { RosterClient } from '../../../../core/maps/roster-client';
import { ProgressionClient } from '../../../../core/progression/progression-client';
import { XpChanges } from '../../../../core/progression/xp-changes';
import { CombatXp } from './combat-xp';

describe('Review12 U12-13: undo of the combat award from another tab', () => {
  const experience = vi.fn();
  const listAwards = vi.fn();
  const award = vi.fn();

  const shares = [{ characterId: 'cp', characterName: 'Pensantus', xp: 100 }];
  const mk = (undone: boolean) =>
    create(XPAwardSchema, {
      id: 'a1',
      mode: XPAwardMode.XP_AWARD_MODE_ENEMIES,
      encounterId: 'enc-1',
      totalXp: 100,
      shares,
      undone,
    });

  beforeEach(() => {
    experience.mockReset().mockResolvedValue(
      create(GetCampaignExperienceResponseSchema, {
        xpMode: XpMode.ENEMIES,
        characters: [
          create(CharacterExperienceSchema, {
            characterId: 'cp',
            name: 'Pensantus',
            level: 3,
            experiencePoints: 10,
            nextLevelXp: 2700,
          }),
        ],
      }),
    );
    listAwards.mockReset().mockResolvedValue(create(ListXPAwardsResponseSchema, { awards: [] }));
    award
      .mockReset()
      .mockResolvedValue(create(AwardXPResponseSchema, { xpEach: 100, award: mk(false) }));
  });

  async function setup() {
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: ProgressionClient, useValue: { experience, listAwards, award } },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
        { provide: MatDialog, useValue: { open: vi.fn() } },
        { provide: MatBottomSheet, useValue: { open: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(CombatXp);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput(
      'encounter',
      encounter({
        id: 'enc-1',
        status: EncounterStatus.ENDED,
        combatants: [
          combatant({
            id: 'g',
            label: 'Goblin',
            defeated: true,
            state: CombatantState.DEFEATED,
            xpValue: 100,
          }),
          combatant({ id: 'p', label: 'Pensantus', kind: CombatantKind.PLAYER, characterId: 'cp' }),
        ],
      }),
    );
    fixture.detectChanges();
    const ready = async () => {
      for (let i = 0; i < 3; i++) {
        await new Promise((r) => setTimeout(r, 0));
        fixture.detectChanges();
      }
    };
    await ready();
    return { fixture, ready, cmp: fixture.componentInstance as any };
  }

  async function giveThenUndo() {
    const s = await setup();
    await s.cmp.give();
    s.fixture.detectChanges();
    expect(s.cmp.view()).toBe('given');
    // The master undoes it elsewhere: the server now lists it as undone.
    listAwards.mockResolvedValue(create(ListXPAwardsResponseSchema, { awards: [mk(true)] }));
    TestBed.inject(XpChanges).bump();
    s.fixture.detectChanges();
    await s.ready();
    return s;
  }

  it('returns to the give form once the award was undone', async () => {
    const { fixture, cmp } = await giveThenUndo();
    expect(cmp.view()).toBe('open');
    expect((fixture.nativeElement as HTMLElement).querySelector('app-xp-actions')).not.toBeNull();
  });

  it('uses a fresh idempotency key to give again after the undo', async () => {
    const { cmp } = await giveThenUndo();
    await cmp.give();
    expect(award).toHaveBeenCalledTimes(2);
    expect(award.mock.calls[1][4]).not.toBe(award.mock.calls[0][4]);
  });
});
