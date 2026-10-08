import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';

import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { FakeCreaturesClient, flat, ogre } from '../../../core/creatures/creatures-testing';
import { openSheet } from '../../../shared/sheet/sheet-host';
import { CreateNpcSheet, type CreateNpcData, type CreateNpcResult } from './create-npc-sheet';

describe('CreateNpcSheet while the request runs', () => {
  it('does not let a reopened dialog create a second NPC', async () => {
    const api = new FakeCreaturesClient();
    // The server is slow: the first answer is held until we release it.
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const original = api.createNpc.getMockImplementation()!;
    api.createNpc.mockImplementationOnce(async (...args: Parameters<typeof original>) => {
      await gate;
      return original(...args);
    });
    TestBed.configureTestingModule({ providers: [{ provide: CreaturesClient, useValue: api }] });
    const dialog = TestBed.inject(MatDialog);
    const bottomSheet = TestBed.inject(MatBottomSheet);
    const open = () =>
      openSheet<CreateNpcSheet, CreateNpcData, CreateNpcResult>(
        dialog,
        bottomSheet,
        CreateNpcSheet,
        {
          data: { campaignId: 'camp-1', creature: ogre() },
          ariaLabel: 'Criar NPC',
          labelledBy: 'npc-t',
          width: '540px',
          tall: true,
          focus: 'input[name=name]',
          restoreFocus: false,
        },
      ).subscribe();
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        await new Promise((r) => setTimeout(r, 0));
        TestBed.tick();
      }
    };
    const clickCreate = () =>
      Array.from(document.querySelectorAll<HTMLButtonElement>('app-create-npc-sheet button'))
        .find((b) => flat(b)?.includes('Criar NPC') || flat(b)?.includes('Criando'))!
        .click();

    open();
    await settle();
    clickCreate();
    await settle();
    expect(api.createNpc).toHaveBeenCalledTimes(1);

    // Esc while the request is pending (Cancelar is disabled, Esc is not).
    document.body.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', code: 'Escape', keyCode: 27, bubbles: true }),
    );
    await settle();
    const stillOpen = dialog.openDialogs.length > 0;

    release();
    await settle();

    if (!stillOpen) {
      // The master reopens "Criar NPC" and submits again.
      open();
      await settle();
      clickCreate();
      await settle();
    }
    // One NPC: the dialog stayed open, so nothing could be submitted again under a new key.
    const keys = api.createNpc.mock.calls.map((c) => c[4]);
    expect(
      stillOpen || (keys.length === 2 && keys[0] === keys[1]),
      `the dialog closed while busy and reopening sent ${keys.length} createNpc calls with keys ${JSON.stringify(keys)}`,
    ).toBe(true);
  });
});
