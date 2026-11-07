// A group check held for a question: when the master answers it for the roller, the sheet drops the question and says so.
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CheckOptionSchema,
  GroupCheckMemberViewSchema,
} from '../../../../gen/meurpg/play/v1/contest_types_pb';
import { ContestClient } from '../../../core/combat/contest-client';
import {
  FakeContestClient,
  checkRoll,
  groupCheck,
  textOf,
} from '../../../core/combat/contest-testing';
import { GroupCheckState } from '../../../core/combat/group-check-state';
import { heldLucky } from '../../../core/held-rolls/held-roll-testing';
import { ResourceClient } from '../../../core/resources/resources-client';
import { GroupCheckSheet, type GroupCheckSheetData } from './group-check-sheet';

// A hold of its own: the page remembers which questions this browser answered, across specs.
const lucky = heldLucky([1], 7, { holdId: 'hold-group-master' });

describe('GroupCheckSheet: the master answers the question for the roller', () => {
  function setup() {
    Element.prototype.scrollTo = vi.fn() as never;
    const api = new FakeContestClient();
    api.lucky = lucky;
    const state = new GroupCheckState();
    state.apply(
      groupCheck({
        youRoll: true,
        yourOption: create(CheckOptionSchema, { modifier: 7, known: true }),
        members: [create(GroupCheckMemberViewSchema, { characterId: 'p', name: 'Pipo' })],
      }),
    );
    const heldTick = signal(0);
    const resources = { answerHeldRoll: vi.fn(), listHeldRolls: vi.fn() };
    resources.listHeldRolls.mockResolvedValue({ offers: [lucky] });
    const data: GroupCheckSheetData = {
      campaignId: 'c',
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state,
      heldTick,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: ContestClient, useValue: api.as() },
        { provide: ResourceClient, useValue: resources },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn(), disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(GroupCheckSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    const buttons = () =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).map((b) => textOf(b));
    const roll = async () => {
      Array.from(el.querySelectorAll<HTMLButtonElement>('button'))
        .find((b) => textOf(b).includes('Rolar no app'))!
        .click();
      await settle();
    };
    return { el, buttons, settle, roll, heldTick, resources, state };
  }

  it('drops the question and says the master answered when the list no longer holds it', async () => {
    const { el, buttons, settle, roll, heldTick, resources } = setup();
    await roll();
    expect(buttons()).toContain('Ficar com o 1');
    resources.listHeldRolls.mockResolvedValue({ offers: [] });
    heldTick.set(1);
    await settle();
    expect(textOf(el.querySelector('[data-testid="sheet-held-vanished"]')!)).toContain(
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

  it('says it on the result too, when the check read again already carries the roll the master made', async () => {
    const { el, buttons, settle, roll, heldTick, resources, state } = setup();
    await roll();
    // The master kept the 1: the check is written with the player's roll, and the list no longer holds the question.
    state.apply(
      groupCheck({
        members: [
          create(GroupCheckMemberViewSchema, {
            characterId: 'p',
            name: 'Pipo',
            answered: true,
            roll: checkRoll({
              skill: 0,
              skillKey: 'skill:stealth',
              faces: [1],
              modifier: 7,
              total: 8,
            }),
          }),
        ],
      }),
    );
    resources.listHeldRolls.mockResolvedValue({ offers: [] });
    heldTick.set(1);
    await settle();
    expect(textOf(el.querySelector('[data-testid="sheet-held-vanished"]')!)).toContain(
      'O mestre respondeu por você.',
    );
    expect(textOf(el)).toContain('Esperando o mestre');
    expect(
      buttons().filter((b) => b.startsWith('Rolar de novo') || b.startsWith('Ficar com o')),
    ).toEqual([]);
  });
});
