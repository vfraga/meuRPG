// Finding U13-28 (review/unit-13-web-live-rest.md): MilestoneSheet keeps a not-eligible character checked after CHARACTER_NOT_ELIGIBLE, so every retry fails the same way.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  MarkMilestoneResponseSchema,
  XPAwardSchema,
  XPBlockedReason,
  XPBlockedSchema,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import type { ExperienceRow } from '../../core/progression/experience-store';
import { ProgressionClient } from '../../core/progression/progression-client';
import { type MilestoneData, MilestoneSheet } from './milestone-sheet';

const row = (id: string, name: string): ExperienceRow => ({
  id,
  name,
  playerUserId: '',
  sub: '',
  level: 3,
  xp: 0,
  nextLevelXp: 2700,
  canLevelUp: false,
  levelUpReason: 0,
});

describe('Review13 U13-28: milestone sheet drops a not-eligible character', () => {
  const markMilestone = vi.fn();

  function setup() {
    const data: MilestoneData = {
      campaignId: 'camp-1',
      campaignName: 'C',
      rows: [row('A', 'Ana'), row('B', 'Bia')],
    };
    TestBed.configureTestingModule({
      imports: [MilestoneSheet],
      providers: [
        { provide: ProgressionClient, useValue: { markMilestone } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(MilestoneSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const input = el.querySelector<HTMLInputElement>('app-xp-reason input')!;
    input.value = 'Marco';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    const press = async () => {
      el.querySelector<HTMLButtonElement>('app-xp-actions .primary')!.click();
      await fixture.whenStable();
      fixture.detectChanges();
    };
    return { fixture, el, press };
  }

  beforeEach(() => {
    markMilestone
      .mockReset()
      .mockResolvedValue(
        create(MarkMilestoneResponseSchema, { award: create(XPAwardSchema, { id: 'a1' }) }),
      );
  });

  it('control: without a refusal both recipients are sent', async () => {
    const { press } = setup();
    await press();
    expect(markMilestone.mock.calls[0][2]).toEqual(['A', 'B']);
  });

  it('drops B (dead) from the recipients after CHARACTER_NOT_ELIGIBLE so the retry can go', async () => {
    markMilestone.mockRejectedValueOnce(
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: XPBlockedSchema,
          value: {
            reason: XPBlockedReason.XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE,
            characterId: 'B',
          },
        },
      ]),
    );
    const { el, press } = setup();
    await press();
    expect(markMilestone.mock.calls[0][2]).toEqual(['A', 'B']);
    expect(el.querySelector('[role="alert"]')).not.toBeNull();

    await press();
    expect(markMilestone.mock.calls[1][2]).toEqual(['A']);
  });
});
