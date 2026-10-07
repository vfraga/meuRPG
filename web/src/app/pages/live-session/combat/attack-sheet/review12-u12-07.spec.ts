// Review 12, finding U12-07: the attack sheet can be closed (X / Cancelar / Esc /
// backdrop) while its roll request is in flight; busy() is not checked in close().
// The sheet calls CombatClient itself (not combat-view's run()), so the page-level
// busy guard does not cover it and a re-opened sheet (new attackKey) can spend a second attack.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CombatantKind, CombatantSide } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { Attack } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { AttackSheet, type AttackSheetData } from './attack-sheet';

const sword = {
  key: 'attack:longsword',
  name: 'Longsword',
  namePt: 'Espada longa',
  attackBonus: 6,
  saveDc: 0,
  rangeFt: 5,
  longRangeFt: 0,
  damage: '1d8 + 3',
  damageTypePt: 'cortante',
  kind: 0,
} as unknown as Attack;

describe('Review12 U12-07: attack sheet closes while the roll request is in flight', () => {
  it('does not close (nor let a second sheet start a second call) while busy', async () => {
    const state = new CombatState();
    state.apply(
      encounter({
        currentCombatantId: 't',
        combatants: [
          combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, side: CombatantSide.PARTY, mine: true }),
          combatant({ id: 'cap', label: 'Capitão Goblin' }),
        ],
      }),
    );
    const calls: unknown[][] = [];
    let release!: (v: unknown) => void;
    const api = {
      rollAttack: (...args: unknown[]) => {
        calls.push(args);
        return new Promise((r) => (release = r));
      },
    };
    const closed: unknown[] = [];
    const data: AttackSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      round: 1,
      attack: sword,
      targets: [],
      diceMode: DiceMode.APP,
      preference: DicePreference.APP,
      state,
      opportunity: { offerId: 'o', targetId: 'cap', targetLabel: 'Capitão Goblin', byMaster: false },
      asReaction: true,
    } as AttackSheetData;
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: (r: unknown) => closed.push(r) } },
        { provide: CombatClient, useValue: api },
      ],
    });
    const fixture = TestBed.createComponent(AttackSheet);
    fixture.detectChanges();
    const cmp = fixture.componentInstance as unknown as {
      rollAttack(d: { face: number }): Promise<void>;
      busy(): boolean;
      close(): void;
    };
    const pending = cmp.rollAttack({ face: 12 });
    expect(cmp.busy()).toBe(true);
    expect(calls.length).toBe(1);

    cmp.close(); // X / Esc / backdrop while the request is still pending

    // The request is still in the air: the sheet must stay open to show its result.
    expect(closed.length).toBe(0);
    release({ encounter: undefined, roll: { outcome: 0 } });
    await pending.catch(() => undefined);
  });
});
