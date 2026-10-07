// Review 12, finding U12-20: ConditionsDialog keeps a snapshot of the conditions taken at open; a condition added meanwhile (stream update) is silently dropped by "Salvar condições" (SetCombatantConditions swaps the whole set).
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { ConditionsDialog } from './conditions-dialog';

describe('Review12 U12-20: ConditionsDialog drops a condition added while it is open', () => {
  it('keeps the concurrently added Poisoned when saving after toggling Prone', async () => {
    const before = combatant({ id: 'g', label: 'Goblin', conditions: [] });
    const after = { ...before, conditions: ['condition:poisoned'] };
    const state = new CombatState();
    state.apply(encounter({ id: 'e1', revision: 1, combatants: [before] }));
    const setConditions = vi.fn(async () => encounter({ id: 'e1', revision: 3, combatants: [after] }));
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
    };
    // The stream brings Poisoned onto the goblin while the dialog is open.
    state.apply(encounter({ id: 'e1', revision: 2, combatants: [after] }));
    cmp.toggle('condition:prone');
    await cmp.save();
    expect(setConditions).toHaveBeenCalledTimes(1);
    const sent = (setConditions.mock.calls[0] as unknown as unknown[])[3] as { keys: string[] };
    expect(sent.keys).toContain('condition:poisoned');
  });
});
