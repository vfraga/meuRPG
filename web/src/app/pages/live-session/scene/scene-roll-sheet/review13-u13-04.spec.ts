// Finding U13-04 (review/unit-13-web-live-rest.md): a roll refused with SceneBlocked NO_OPEN_SCENE never re-reads the scene, so the stale closed scene (and its "Rolar" buttons) stays.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { SceneBlockedReason, SceneBlockedSchema } from '../../../../../gen/meurpg/play/v1/scene_pb';
import { SceneClient } from '../../../../core/play/scene-client';
import { SceneState } from '../../../../core/play/scene-state';
import { FakeSceneClient, playerScene } from '../../../../core/play/scene-testing';
import { SceneRollSheet, type SceneRollSheetData } from './scene-roll-sheet';

describe('Review13 U13-04: roll refused with NO_OPEN_SCENE leaves the closed scene on screen', () => {
  beforeEach(() => {
    Element.prototype.scrollTo = vi.fn() as never;
  });

  async function run(reason: SceneBlockedReason | null) {
    const api = new FakeSceneClient();
    const state = new SceneState(
      () => api.get(),
      () => false,
    );
    // The player holds the open scene; the master has closed it meanwhile.
    state.apply(playerScene());
    api.scene = null;
    if (reason !== null) {
      // Only the roll is refused; the read of the scene works.
      const refused = new ConnectError('x', Code.FailedPrecondition, undefined, [
        { desc: SceneBlockedSchema, value: create(SceneBlockedSchema, { reason }) },
      ]);
      api.roll = async () => {
        throw refused;
      };
    }
    const data: SceneRollSheetData = {
      campaignId: 'c1',
      action: playerScene().actions[0],
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: SceneClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(SceneRollSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => b.textContent?.includes('Rolar no app'))!
      .click();
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      fixture.detectChanges();
    }
    return { api, state, el };
  }

  it('control: a successful roll re-reads the scene (and the closed scene is cleared)', async () => {
    const { api, state } = await run(null);
    expect(api.calls).toContain('get');
    expect(state.scene()).toBeNull();
  });

  it('re-reads the scene after NO_OPEN_SCENE so the stale scene is cleared', async () => {
    const { api, state, el } = await run(SceneBlockedReason.NO_OPEN_SCENE);
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('fechou a cena');
    expect(api.calls.filter((c) => c === 'get')).toHaveLength(1);
    expect(state.scene()).toBeNull();
  });
});
