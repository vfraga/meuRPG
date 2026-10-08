import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { ConditionsDialog } from './conditions-dialog';

describe('ConditionsDialog', () => {
  function open() {
    const before = combatant({ id: 'g', label: 'Goblin', conditions: [] });
    const poisoned = { ...before, conditions: ['condition:poisoned'] };
    const state = new CombatState();
    state.apply(encounter({ id: 'e1', revision: 1, combatants: [before] }));
    const setConditions = vi.fn(async () =>
      encounter({ id: 'e1', revision: 3, combatants: [poisoned] }),
    );
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: { setConditions } },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
        {
          provide: MAT_DIALOG_DATA,
          useValue: { campaignId: 'c1', encounterId: 'e1', combatant: before, state },
        },
      ],
    });
    const fixture = TestBed.createComponent(ConditionsDialog);
    fixture.detectChanges();
    const cmp = fixture.componentInstance as unknown as {
      toggle(k: string): void;
      save(): Promise<void>;
      changed(): boolean;
    };
    return { state, poisoned, setConditions, cmp };
  }

  const sentKeys = (call: unknown[]) => (call[3] as { keys: string[] }).keys;

  it('keeps a condition added while it is open when the master saves another one', async () => {
    const { state, poisoned, setConditions, cmp } = open();
    state.apply(encounter({ id: 'e1', revision: 2, combatants: [poisoned] }));
    cmp.toggle('condition:prone');
    await cmp.save();
    expect(setConditions).toHaveBeenCalledTimes(1);
    expect(sentKeys(setConditions.mock.calls[0] as unknown[]).sort()).toEqual([
      'condition:poisoned',
      'condition:prone',
    ]);
  });

  it('is not changed by a condition added elsewhere', () => {
    const { state, poisoned, cmp } = open();
    state.apply(encounter({ id: 'e1', revision: 2, combatants: [poisoned] }));
    expect(cmp.changed()).toBe(false);
  });

  it('does not bring back a condition the master took off when it is added elsewhere again', async () => {
    const { state, poisoned, setConditions, cmp } = open();
    state.apply(encounter({ id: 'e1', revision: 2, combatants: [poisoned] }));
    cmp.toggle('condition:poisoned');
    await cmp.save();
    expect(sentKeys(setConditions.mock.calls[0] as unknown[])).toEqual([]);
  });

  it('cannot be closed while the change is on its way', async () => {
    const { cmp, setConditions } = open();
    let answer!: (e: unknown) => void;
    setConditions.mockReturnValue(new Promise((resolve) => (answer = resolve)) as never);
    cmp.toggle('condition:prone');
    const saving = cmp.save();
    const closeNow = (cmp as unknown as { close(): void }).close;
    closeNow.call(cmp);
    expect(TestBed.inject(MatDialogRef).close).not.toHaveBeenCalled();
    answer(encounter({ id: 'e1', revision: 3 }));
    await saving;
    expect(TestBed.inject(MatDialogRef).close).toHaveBeenCalledWith(true);
  });

  it('keeps one key for ending the concentration again after a lost answer, and a new one after it worked', async () => {
    const { setConditions, cmp } = open();
    const end = (cmp as unknown as { endConcentration(): Promise<void> }).endConcentration.bind(
      cmp,
    );
    setConditions
      .mockRejectedValueOnce(new Error('lost') as never)
      .mockResolvedValue(encounter({ id: 'e1', revision: 3 }) as never);
    await end();
    await end();
    await end();
    const keys = setConditions.mock.calls.map((c) => (c as unknown[])[4]);
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[1]);
  });
});
