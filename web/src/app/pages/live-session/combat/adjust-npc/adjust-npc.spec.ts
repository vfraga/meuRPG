// The adjust sheet works from the combatant as the combat has it now, not as it was at open.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { AdjustNpc } from './adjust-npc';

describe('AdjustNpc reads the live hit points', () => {
  it('compares an exact value with the current hit points', async () => {
    const at5 = combatant({ id: 'g', label: 'Goblin', hitPointsCurrent: 5, hitPointsMax: 12 });
    const at10 = { ...at5, hitPointsCurrent: 10 };
    const state = new CombatState();
    state.apply(encounter({ id: 'e1', revision: 1, combatants: [at5] }));
    const adjustHitPoints = vi.fn(async () =>
      encounter({ id: 'e1', revision: 3, combatants: [at10] }),
    );
    const close = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: { adjustHitPoints } },
        { provide: MatDialogRef, useValue: { close } },
        {
          provide: MAT_DIALOG_DATA,
          useValue: { campaignId: 'c1', encounterId: 'e1', combatant: at5, state },
        },
      ],
    });
    const fixture = TestBed.createComponent(AdjustNpc);
    fixture.detectChanges();
    const cmp = fixture.componentInstance as unknown as {
      mode: { set(v: string): void };
      amount: { set(v: number): void };
      save(): Promise<void>;
    };
    // The stream brings the goblin to 10 HP while the sheet is open.
    state.apply(encounter({ id: 'e1', revision: 2, combatants: [at10] }));
    cmp.mode.set('exact');
    cmp.amount.set(5);
    await cmp.save();
    expect(adjustHitPoints).toHaveBeenCalledTimes(1);
    expect(adjustHitPoints).toHaveBeenCalledWith(
      'c1',
      'e1',
      'g',
      { change: { kind: 'exact', value: 5 }, temporary: undefined },
      expect.any(String),
    );
  });

  describe('while a correction is on its way', () => {
    function open(adjustHitPoints: ReturnType<typeof vi.fn>) {
      const goblin = combatant({ id: 'g', label: 'Goblin', hitPointsCurrent: 5, hitPointsMax: 12 });
      const state = new CombatState();
      state.apply(encounter({ id: 'e1', revision: 1, combatants: [goblin] }));
      const close = vi.fn();
      TestBed.configureTestingModule({
        providers: [
          { provide: CombatClient, useValue: { adjustHitPoints } },
          { provide: MatDialogRef, useValue: { close } },
          {
            provide: MAT_DIALOG_DATA,
            useValue: { campaignId: 'c1', encounterId: 'e1', combatant: goblin, state },
          },
        ],
      });
      const fixture = TestBed.createComponent(AdjustNpc);
      fixture.detectChanges();
      const cmp = fixture.componentInstance as unknown as {
        amount: { set(v: number): void };
        save(): Promise<void>;
        close(): void;
      };
      return { cmp, close, state };
    }

    it('cannot be closed, so its answer is not lost', async () => {
      let answer!: (e: unknown) => void;
      const adjustHitPoints = vi.fn(() => new Promise((resolve) => (answer = resolve)));
      const { cmp, close, state } = open(adjustHitPoints);
      cmp.amount.set(3);
      const saving = cmp.save();
      cmp.close();
      expect(close).not.toHaveBeenCalled();
      answer(encounter({ id: 'e1', revision: 2, combatants: [state.encounter()!.combatants[0]] }));
      await saving;
      expect(close).toHaveBeenCalledWith(true);
    });

    it('sends the same key for the same numbers and a new one for other numbers', async () => {
      const adjustHitPoints = vi
        .fn()
        .mockRejectedValue(new Error('lost'))
        .mockRejectedValueOnce(new Error('lost'));
      const { cmp } = open(adjustHitPoints);
      cmp.amount.set(3);
      await cmp.save();
      await cmp.save();
      cmp.amount.set(4);
      await cmp.save();
      const keys = adjustHitPoints.mock.calls.map((c) => c[4] as string);
      expect(keys[1]).toBe(keys[0]);
      expect(keys[2]).not.toBe(keys[0]);
    });
  });
});
