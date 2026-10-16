import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ActionEconomy, SpellDamageChoice } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { CastSheet, type CastSheetData } from './cast-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

describe('CastSheet: the damage type the caster picks', () => {
  async function open(damageChoice: SpellDamageChoice, types = ['fire', 'radiant']) {
    TestBed.resetTestingModule();
    const castSpell = vi.fn().mockRejectedValue(new Error('stop'));
    const details = {
      spell: { level: 5 },
      damageChoice,
      damage: types.map((t, i) => ({
        damageTypeKey: `damage-type:${t}`,
        damageTypePt: ['fogo', 'radiante'][i],
        bySlotLevel: { 5: '4d6' },
        byCharacterLevel: {},
      })),
      healBySlotLevel: {},
      higherLevel: [],
    };
    const data = {
      campaignId: 'c',
      encounterId: 'enc',
      casterId: 's',
      round: 1,
      spellKey: 'spell:flame-strike',
      name: 'Coluna de Chamas',
      level: 5,
      concentration: false,
      cantripDice: '',
      economy: ActionEconomy.ACTION,
      slots: [{ level: 5, free: 1, pact: false }],
      usage: [{ level: 5, total: 1, used: 0 }],
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
    const radios = () =>
      Array.from(
        el.querySelectorAll<HTMLInputElement>('app-damage-type-picker input[type="radio"]'),
      );
    const cast = async () => {
      const button = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Conjurar Coluna de Chamas'),
      )!;
      button.click();
      await fixture.whenStable();
    };
    return { fixture, castSpell, el, radios, cast };
  }

  it('offers the spell’s types with the first chosen, and says where the higher slot’s dice go', async () => {
    const { el, radios } = await open(SpellDamageChoice.SCALE);
    expect(radios().length).toBe(2);
    expect(radios().map((r) => r.checked)).toEqual([true, false]);
    expect(el.querySelector('app-damage-type-picker')?.textContent).toContain('fogo');
    expect(el.querySelector('app-damage-type-picker')?.textContent).toContain('radiante');
    expect(el.querySelector('app-damage-type-picker')?.textContent).toContain(
      'Os dados a mais de um espaço de magia maior vão para o tipo escolhido.',
    );
  });

  it('says that only the chosen type is dealt when the spell deals just one of them', async () => {
    const { el } = await open(SpellDamageChoice.ALTERNATIVE);
    expect(el.querySelector('app-damage-type-picker')?.textContent).toContain(
      'Só o tipo escolhido causa dano.',
    );
  });

  it('sends the first type when the caster does not pick, and the picked one otherwise', async () => {
    const first = await open(SpellDamageChoice.SCALE);
    await first.cast();
    expect(first.castSpell.mock.calls[0][9]).toBe('damage-type:fire');

    const picked = await open(SpellDamageChoice.SCALE);
    picked.radios()[1].click();
    picked.fixture.detectChanges();
    expect(picked.radios().map((r) => r.checked)).toEqual([false, true]);
    await picked.cast();
    expect(picked.castSpell.mock.calls[0][9]).toBe('damage-type:radiant');
  });

  it('shows no picker for a spell that offers no choice', async () => {
    const { el, castSpell, cast } = await open(SpellDamageChoice.UNSPECIFIED);
    expect(el.querySelector('app-damage-type-picker')).toBeNull();
    await cast();
    expect(castSpell.mock.calls[0][9]).toBe('');
  });
});
