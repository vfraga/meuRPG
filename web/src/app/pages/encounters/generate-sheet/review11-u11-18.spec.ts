// Finding U11-18: confirmSwap has no `version` check, so a swap's evaluate that resolves after a
// setBand()/setType() regeneration overwrites the newer encounter with the stale swapped one.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';

import { flat } from '../../../core/creatures/creatures-testing';
import { EncountersClient } from '../../../core/encounters/encounters-client';
import {
  BANDIT,
  FakeEncountersClient,
  GOBLIN,
  MIRATHEL,
  OGRE,
  evaluation,
  line,
} from '../../../core/encounters/encounters-testing';
import { GenerateSheet, type GenerateData } from './generate-sheet';

describe('Review11 U11-18: swap in flight is not discarded by a newer generate()', () => {
  it('a band change during the swap evaluate wins; "Usar" takes the new band encounter', async () => {
    const api = new FakeEncountersClient();
    const ORC = { ...GOBLIN, key: 'monster:orc', namePt: 'Orc', name: 'Orc', xp: 100 };
    api.table.set(ORC.key, ORC as never);
    api.generated = { lines: [[BANDIT, 4]], seed: 111 };
    api.swapList = [ORC as never];
    const close = vi.fn();
    const data: GenerateData = {
      campaignId: 'camp-1',
      campaignName: 'Mirathel',
      party: [],
      evaluation: evaluation([line(OGRE, 1)], { party: MIRATHEL }),
    };
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: EncountersClient, useValue: api },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close } },
      ],
    });
    const fixture = TestBed.createComponent(GenerateSheet);
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
      }
    };
    await settle();
    const el = fixture.nativeElement as HTMLElement;

    // Pick a swap and confirm; the evaluate RPC is held back.
    let release!: () => void;
    api.gates = [new Promise<void>((r) => (release = r))];
    el.querySelector<HTMLButtonElement>('.line__swap')!.click();
    await settle();
    el.querySelector<HTMLInputElement>('.swap__opt input')!.click();
    await settle();
    el.querySelector<HTMLButtonElement>('.swap .go')!.click();
    await settle();
    expect(api.evaluateCalls.length).toBe(1);

    // Meanwhile the master changes the difficulty: a fresh encounter (seed 222, 6 Ogres) arrives.
    api.generated = { lines: [[OGRE, 6]], seed: 222 };
    const alta = Array.from(el.querySelectorAll<HTMLLabelElement>('.seg__item'))
      .find((l) => flat(l)?.includes('Alta'))!
      .querySelector('input')!;
    alta.click();
    await settle();
    expect(flat(el.querySelector('.res__seed'))).toBe('Semente 222');

    // The old swap answers last.
    release();
    await settle();

    // Correct behaviour: the newer generate() result stays.
    expect(flat(el.querySelector('.res__seed'))).toBe('Semente 222');
    expect(Array.from(el.querySelectorAll('.line__pt')).map((l) => flat(l))).toEqual([
      '6 × Ogro',
    ].map((s) => s));
    expect(flat(el.querySelector('.seg__item--on'))).toBe('Alta');
  });
});
