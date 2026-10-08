import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { encounter } from '../../../../core/combat/combat-testing';
import { FeatureSheet } from './feature-sheet';

describe('FeatureSheet', () => {
  function open(takeAction: ReturnType<typeof vi.fn>) {
    const close = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: { takeAction } },
        { provide: MatDialogRef, useValue: { close } },
        {
          provide: MAT_DIALOG_DATA,
          useValue: {
            campaignId: 'c1',
            encounterId: 'e1',
            combatantId: 'a',
            actionKey: 'feature:second-wind',
            name: 'Retomar o Fôlego',
            cost: 'Ação bônus',
            diceMode: DiceMode.PLAYERS_CHOOSE,
            preference: DicePreference.APP,
            state: new CombatState(),
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(FeatureSheet);
    fixture.detectChanges();
    const cmp = fixture.componentInstance as unknown as {
      use(die: { inApp: true } | { sum: number }): Promise<void>;
      close(): void;
    };
    return { cmp, close };
  }

  it('cannot be closed while the use is on its way, and closes saying it was used', async () => {
    let answer!: (r: unknown) => void;
    const { cmp, close } = open(vi.fn(() => new Promise((resolve) => (answer = resolve))));
    const using = cmp.use({ inApp: true });
    cmp.close();
    expect(close).not.toHaveBeenCalled();
    answer({ encounter: encounter({ id: 'e1' }), roll: undefined, healed: 4 });
    await using;
    cmp.close();
    expect(close).toHaveBeenCalledWith(true);
  });

  it('sends the same key for the same roll after a lost answer, and a new one for another roll', async () => {
    const takeAction = vi.fn().mockRejectedValue(new Error('lost'));
    const { cmp } = open(takeAction);
    await cmp.use({ sum: 6 });
    await cmp.use({ sum: 6 });
    await cmp.use({ sum: 7 });
    const keys = takeAction.mock.calls.map((c) => c[5] as string);
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[0]);
  });
});
