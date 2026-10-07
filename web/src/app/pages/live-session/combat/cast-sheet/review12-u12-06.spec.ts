// Review12 finding U12-06: the cast sheet keeps the roll picker's typed text when the slot
// (or the target) changes, so a roll made for one pool is sent for another.
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ActionEconomy, SpellHitPointEffectKind } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { CastSheet, type CastSheetData } from './cast-sheet';

describe('Review12 U12-06: typed dice survive a slot change in the cast sheet', () => {
  async function open() {
    TestBed.resetTestingModule();
    const castSpell = vi.fn().mockRejectedValue(new Error('stop'));
    const sleep = {
      spell: { level: 1 },
      hitPointEffect: {
        kind: SpellHitPointEffectKind.POOL,
        poolDiceCount: 5,
        poolDicePerLevel: 2,
        poolDiceSides: 8,
      },
      healBySlotLevel: {},
      damage: [],
      higherLevel: [],
    };
    const data = {
      campaignId: 'c',
      encounterId: 'enc',
      casterId: 's',
      round: 1,
      spellKey: 'spell:sleep',
      name: 'Sono',
      level: 1,
      concentration: false,
      economy: ActionEconomy.ACTION,
      slots: [
        { level: 1, free: 2, pact: false },
        { level: 2, free: 2, pact: false },
      ],
      usage: [
        { level: 1, total: 2, used: 0 },
        { level: 2, total: 2, used: 0 },
      ],
      pact: null,
      targets: undefined,
      shieldFree: null,
      shieldName: '',
      attackBonus: 0,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state: { encounter: signal(encounter({ combatants: [] })), apply: vi.fn() },
    } as unknown as CastSheetData;
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: { castSpell } },
        { provide: SpellCatalog, useValue: { details: () => Promise.resolve(sleep) } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(CastSheet);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return { fixture, castSpell, el: fixture.nativeElement as HTMLElement };
  }

  it('does not send a 5d8 typed sum as the 7d8 pool of the 2nd-circle slot', async () => {
    const { fixture, castSpell, el } = await open();
    const typeBtn = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Digitar o resultado'),
    )!;
    typeBtn.click();
    fixture.detectChanges();
    const field = el.querySelector('input.type__field') as HTMLInputElement;
    expect(el.textContent).toContain('Role 5d8');
    field.value = '22';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();

    const rows = (fixture.componentInstance as any).rows() as { level: number }[];
    (fixture.componentInstance as any).pickSlot(rows.find((r) => r.level === 2));
    fixture.detectChanges();
    expect(el.textContent).toContain('Role 7d8');

    const field2 = el.querySelector('input.type__field') as HTMLInputElement;
    const confirm = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Confirmar'),
    );
    // The 5-dice roll must not still be confirmable for the 7-dice pool.
    const stillSendable = field2.value === '22' && !!confirm && !confirm.disabled;
    if (stillSendable) {
      confirm!.click();
      await fixture.whenStable();
    }
    const sentStale = castSpell.mock.calls.some((c) => c[6]?.poolSum === 22);
    expect(sentStale).toBe(false);
    expect(field2.value).toBe('');
  });
});
