import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ActionEconomy } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { CastSheet, type CastSheetData } from './cast-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

describe('CastSheet: the ability Aprimorar Habilidade is cast for', () => {
  async function open(spellKey: string, name: string) {
    TestBed.resetTestingModule();
    const castSpell = vi.fn().mockRejectedValue(new Error('stop'));
    const details = { spell: { level: 2 }, damage: [], healBySlotLevel: {}, higherLevel: [] };
    const data = {
      campaignId: 'c',
      encounterId: 'enc',
      casterId: 's',
      round: 1,
      spellKey,
      name,
      level: 2,
      concentration: true,
      cantripDice: '',
      economy: ActionEconomy.ACTION,
      slots: [{ level: 2, free: 1, pact: false }],
      usage: [{ level: 2, total: 1, used: 0 }],
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
        b.textContent?.includes(`Conjurar ${name}`),
      ) as HTMLButtonElement;
    const pickSlot = () => {
      el.querySelector<HTMLInputElement>('app-slot-picker input')?.click();
      fixture.detectChanges();
    };
    return { fixture, castSpell, el, button, pickSlot };
  }

  it('holds the cast back until an ability is chosen, then sends it', async () => {
    const { fixture, castSpell, el, button, pickSlot } = await open(
      'spell:enhance-ability',
      'Aprimorar Habilidade',
    );
    pickSlot();
    expect(el.querySelectorAll('app-ability-picker input[type="radio"]')).toHaveLength(6);
    expect(el.querySelector('.missing')?.textContent).toContain('Escolha a habilidade.');
    button().click();
    await fixture.whenStable();
    expect(castSpell).not.toHaveBeenCalled();

    el.querySelectorAll<HTMLInputElement>('app-ability-picker input')[3].click();
    fixture.detectChanges();
    expect(el.querySelector('.missing')?.textContent ?? '').not.toContain('Escolha a habilidade.');
    button().click();
    await fixture.whenStable();
    expect(castSpell).toHaveBeenCalledTimes(1);
    expect(castSpell.mock.calls[0][12]).toMatchObject({ abilityKey: 'int' });
  });

  it('shows no ability picker for another spell', async () => {
    const { el, fixture, castSpell, button, pickSlot } = await open('spell:bless', 'Bênção');
    pickSlot();
    expect(el.querySelector('app-ability-picker')).toBeNull();
    button().click();
    await fixture.whenStable();
    expect(castSpell.mock.calls[0][12].abilityKey).toBeUndefined();
  });
});
