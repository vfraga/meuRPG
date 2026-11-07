// A scene check held for a question: when the master answers it for the roller, the sheet drops the question and says so.
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import { heldDie, heldLucky } from '../../../../core/held-rolls/held-roll-testing';
import { SceneClient } from '../../../../core/play/scene-client';
import { SceneState } from '../../../../core/play/scene-state';
import { FakeSceneClient, playerScene } from '../../../../core/play/scene-testing';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { SceneRollSheet, type SceneRollSheetData } from './scene-roll-sheet';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

// A hold of its own: the page remembers which questions this browser answered, across specs.
const lucky = heldLucky([1], 6, { holdId: 'hold-scene-master' });

describe('SceneRollSheet: the master answers the question for the roller', () => {
  function setup() {
    Element.prototype.scrollTo = vi.fn() as never;
    const api = new FakeSceneClient();
    api.held = lucky;
    const heldTick = signal(0);
    const resources = { answerHeldRoll: vi.fn(), listHeldRolls: vi.fn() };
    resources.listHeldRolls.mockResolvedValue({ offers: [lucky] });
    const data: SceneRollSheetData = {
      campaignId: 'c1',
      action: playerScene().actions[0],
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state: new SceneState(
        () => api.get(),
        () => false,
      ),
      heldTick,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: SceneClient, useValue: api },
        { provide: ResourceClient, useValue: resources },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(SceneRollSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    const buttons = () =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).map((b) => flat(b));
    const roll = async () => {
      Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
        .find((b) => flat(b).includes('Rolar no app'))!
        .click();
      await settle();
    };
    return { el, buttons, settle, roll, heldTick, resources };
  }

  it('drops the question and says the master answered when the list no longer holds it', async () => {
    const { el, buttons, settle, roll, heldTick, resources } = setup();
    await roll();
    expect(buttons()).toContain('Ficar com o 1');
    // The master answered: the stream says the questions changed, and the list has none.
    resources.listHeldRolls.mockResolvedValue({ offers: [] });
    heldTick.set(1);
    await settle();
    expect(flat(el.querySelector('[data-testid="sheet-held-vanished"]'))).toContain(
      'O mestre respondeu por você.',
    );
    expect(buttons()).not.toContain('Ficar com o 1');
    expect(buttons().filter((b) => b.startsWith('Rolar de novo'))).toEqual([]);
    expect(buttons().filter((b) => b.includes('Rolar no app'))).toEqual([]);
  });

  it('keeps the question while the list still holds it', async () => {
    const { el, buttons, settle, roll, heldTick } = setup();
    await roll();
    heldTick.set(1);
    await settle();
    expect(el.querySelector('[data-testid="sheet-held-vanished"]')).toBeNull();
    expect(buttons()).toContain('Ficar com o 1');
  });

  it('reads the list a bounded number of times when the master answered the first question and the next is open', async () => {
    const { buttons, settle, roll, heldTick, resources } = setup();
    await roll();
    const next = () => heldDie(6, 5, 8, { holdId: 'hold-scene-master' });
    resources.listHeldRolls.mockImplementation(() => Promise.resolve({ offers: [next()] }));
    resources.listHeldRolls.mockClear();
    heldTick.set(1);
    await settle();
    await settle();
    expect(resources.listHeldRolls.mock.calls.length).toBeLessThanOrEqual(3);
    expect(buttons().some((b) => /d8|Não usar/.test(b))).toBe(true);
  });

  it('says the master answered when the player’s click was refused because the master was first', async () => {
    const { el, buttons, settle, roll, resources } = setup();
    await roll();
    // The master answered before the click, and the list already says so; the refused click must not hide it.
    resources.listHeldRolls.mockResolvedValue({ offers: [] });
    resources.answerHeldRoll.mockRejectedValue(
      new ConnectError('answered', Code.FailedPrecondition),
    );
    Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
      .find((b) => flat(b) === 'Ficar com o 1')!
      .click();
    await settle();
    await settle();
    expect(flat(el.querySelector('[data-testid="sheet-held-vanished"]'))).toContain(
      'O mestre respondeu por você.',
    );
    expect(buttons()).not.toContain('Ficar com o 1');
  });
});
