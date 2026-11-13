import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ActionEconomy } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { CastSheet, type CastSheetData } from './cast-sheet';

describe('CastSheet: a spell cast through a grant (MR-025)', () => {
  const grant = {
    sourceKey: 'feat:graca@mesa',
    sourceNamePt: 'Graça de teste',
    atWill: false,
    resource: 'usos_da_graca',
    usesLeft: 1,
    alsoWithSlot: false,
    ability: 'cha',
    attackBonus: 6,
    saveDc: 14,
  };

  async function open(over: { alsoWithSlot?: boolean; usesCost?: number } = {}) {
    TestBed.resetTestingModule();
    const castSpell = vi.fn().mockRejectedValue(new Error('stop'));
    const details = { spell: { level: 1 }, damage: [], healBySlotLevel: {}, higherLevel: [] };
    const alsoWithSlot = over.alsoWithSlot ?? false;
    const data = {
      campaignId: 'c',
      encounterId: 'enc',
      casterId: 's',
      round: 1,
      spellKey: 'spell:bless',
      name: 'Bênção',
      level: 1,
      concentration: true,
      cantripDice: '',
      economy: ActionEconomy.ACTION,
      grant: {
        ...grant,
        alsoWithSlot,
        usesLeft: over.usesCost ? 3 : 1,
        usesCost: over.usesCost ?? 0,
      },
      slots: alsoWithSlot ? [{ level: 1, free: 2, pact: false }] : [],
      usage: [{ level: 1, total: 2, used: 0 }],
      pact: null,
      targets: undefined,
      shieldFree: null,
      shieldName: '',
      attackBonus: 6,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state: { encounter: signal(encounter({ combatants: [] })), apply: vi.fn() },
    } as unknown as CastSheetData;
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: fakeCombatClient({ castSpell }) },
        { provide: SpellCatalog, useValue: { details: () => Promise.resolve(details) } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(CastSheet);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const button = () =>
      Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Conjurar Bênção'),
      ) as HTMLButtonElement;
    return { fixture, castSpell, el, button };
  }

  it('shows where it comes from, asks for no slot, and sends the grant with no slot', async () => {
    const { fixture, castSpell, el, button } = await open();
    expect(el.querySelector('.grant')?.textContent).toContain('Graça de teste · 1 uso');
    expect(el.querySelector('app-slot-picker')).toBeNull();
    expect(el.querySelector('.missing')?.textContent ?? '').not.toContain('espaço');
    button().click();
    await fixture.whenStable();
    expect(castSpell).toHaveBeenCalledTimes(1);
    const call = castSpell.mock.calls[0];
    expect(call[4]).toBeNull();
    expect(call[12]).toMatchObject({ grantedBy: 'feat:graca@mesa' });
  });

  it('says what a cast costs when it spends more than one use', async () => {
    const { el } = await open({ usesCost: 2 });
    expect(el.querySelector('.grant')?.textContent).toContain('Graça de teste · gasta 2 · 3 usos');
  });

  it('offers a slot only when the grant allows one, and sends the slot without a use of the grant', async () => {
    const { fixture, castSpell, el, button } = await open({ alsoWithSlot: true });
    const free = el.querySelector<HTMLInputElement>('app-check-row input');
    expect(free?.checked).toBe(true);
    expect(el.querySelector('app-slot-picker')).toBeNull();

    free?.click();
    fixture.detectChanges();
    expect(el.querySelector('app-slot-picker')).not.toBeNull();
    button().click();
    await fixture.whenStable();
    expect(castSpell.mock.calls[0][4]).toEqual({ level: 1, pact: false });
  });
});
