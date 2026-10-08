// A rejected typed damage keeps the form open, with the typed number, so it can be corrected.
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { PendingDamageStatus } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ActionEconomy } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { CastSheet, type CastSheetData } from './cast-sheet';

describe('a failed typed damage keeps the typed form open', () => {
  it('keeps typing() true when the roll is rejected', async () => {
    TestBed.resetTestingModule();
    const rollDamage = vi.fn().mockRejectedValue(new Error('lost'));
    const data = {
      campaignId: 'c',
      encounterId: 'enc',
      casterId: 's',
      round: 1,
      spellKey: 'spell:fire-bolt',
      name: 'Raio de Fogo',
      level: 0,
      concentration: false,
      economy: ActionEconomy.ACTION,
      slots: [],
      usage: [],
      pact: null,
      targets: undefined,
      shieldFree: null,
      shieldName: '',
      attackBonus: 0,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state: {
        encounter: signal(encounter({ combatants: [combatant({ id: 't', label: 'Goblin' })] })),
        apply: vi.fn(),
      },
      resume: [
        {
          id: 'p1',
          targetId: 't',
          status: PendingDamageStatus.AWAITING_ROLL,
          diceCount: 1,
          diceSides: 10,
          bonus: 0,
          criticalMax: 0,
        },
      ],
    } as unknown as CastSheetData;
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: { rollDamage } },
        { provide: SpellCatalog, useValue: { details: () => Promise.resolve(null) } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(CastSheet);
    fixture.detectChanges();
    await fixture.whenStable();
    const sheet = fixture.componentInstance as unknown as {
      typing: { set(v: boolean): void; (): boolean };
      error(): string;
      rollTyped(sum: number): Promise<void>;
    };
    sheet.typing.set(true);
    await sheet.rollTyped(7);
    expect(rollDamage).toHaveBeenCalledTimes(1);
    expect(sheet.error()).not.toBe('');
    expect(sheet.typing()).toBe(true);
  });
});
