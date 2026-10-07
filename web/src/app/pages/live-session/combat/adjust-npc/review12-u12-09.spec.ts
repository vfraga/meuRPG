// Review 12, finding U12-09: AdjustNpc freezes hp/max/temp0 from data.combatant, so a stream update while the sheet is open makes "Valor exato" drop a legitimate change.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { AdjustNpc } from './adjust-npc';

describe('Review12 U12-09: AdjustNpc uses a stale hit-point baseline', () => {
  it('sends exact 5 when the goblin went from 5 to 10 HP while the sheet was open', async () => {
    const at5 = combatant({ id: 'g', label: 'Goblin', hitPointsCurrent: 5, hitPointsMax: 12 });
    const at10 = { ...at5, hitPointsCurrent: 10 };
    const state = new CombatState();
    state.apply(encounter({ id: 'e1', revision: 1, combatants: [at5] }));
    const adjustHitPoints = vi.fn(async () => encounter({ id: 'e1', revision: 3, combatants: [at10] }));
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
      'c1', 'e1', 'g', { change: { kind: 'exact', value: 5 }, temporary: undefined }, expect.any(String),
    );
  });
});
