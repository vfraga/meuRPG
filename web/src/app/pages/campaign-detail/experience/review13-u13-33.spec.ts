// Finding U13-33 (review/unit-13-web-live-rest.md): XP undo refreshes the store only on Aborted, not on XPBlocked NOTHING_TO_UNDO, though the message says "A tela foi atualizada".
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  XPAwardMode,
  XPAwardSchema,
  XPBlockedReason,
  XPBlockedSchema,
} from '../../../../gen/meurpg/progression/v1/progression_pb';
import { ExperienceStore } from '../../../core/progression/experience-store';
import { ProgressionClient } from '../../../core/progression/progression-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { AwardHistory } from './award-history';

const LAST = create(XPAwardSchema, {
  id: 'a3',
  mode: XPAwardMode.XP_AWARD_MODE_ENEMIES,
  givenByDisplayName: 'Samuel',
  createdAt: timestampFromDate(new Date(2026, 9, 2, 20, 41)),
  reason: 'Combate: Emboscada',
  totalXp: 232,
  canUndo: true,
  shares: [
    { characterId: 'p', characterName: 'Pensantus', xp: 116 },
    { characterId: 't', characterName: 'Toren', xp: 116 },
  ],
});

describe('Review13 U13-33: XP undo does not re-read the history on NOTHING_TO_UNDO', () => {
  const undoLast = vi.fn();

  async function run(rejection: ConnectError) {
    undoLast.mockReset().mockRejectedValue(rejection);
    Element.prototype.scrollIntoView = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        ExperienceStore,
        { provide: ProgressionClient, useValue: { undoLast } },
        { provide: RosterClient, useValue: { list: () => Promise.resolve([]) } },
      ],
    });
    const store = TestBed.inject(ExperienceStore);
    store.awards.set([LAST]);
    store.rows.set([]);
    const refresh = vi.spyOn(store, 'refresh').mockResolvedValue();
    const fixture = TestBed.createComponent(AwardHistory);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('isMaster', true);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const click = async (name: string) => {
      Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
        .find(
          (b) => b.textContent?.trim() === name || b.getAttribute('aria-label')?.startsWith(name),
        )!
        .click();
      await fixture.whenStable();
      fixture.detectChanges();
      await fixture.whenStable();
      fixture.detectChanges();
    };
    await click('Desfazer');
    await click('Desfazer XP');
    return { el, refresh };
  }

  it('control: aborted re-reads the history', async () => {
    const { refresh } = await run(new ConnectError('x', Code.Aborted));
    expect(refresh).toHaveBeenCalled();
  });

  it('re-reads the history when the server says there is nothing to undo, as its message claims', async () => {
    const { el, refresh } = await run(
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: XPBlockedSchema,
          value: { reason: XPBlockedReason.XP_BLOCKED_REASON_NOTHING_TO_UNDO },
        },
      ]),
    );
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('A tela foi atualizada');
    expect(refresh).toHaveBeenCalled();
  });
});
