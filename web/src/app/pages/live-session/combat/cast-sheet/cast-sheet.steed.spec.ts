import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ActionEconomy } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { CastSheet, type CastSheetData } from './cast-sheet';

describe('CastSheet: "Estrela também", a self-only spell that reaches the steed the caster rides (#588, board 588-26 card 6)', () => {
  async function open(alsoSteed: boolean, answer?: unknown) {
    TestBed.resetTestingModule();
    const castSpell = answer
      ? vi.fn().mockResolvedValue(answer)
      : vi.fn().mockRejectedValue(new Error('stop'));
    const details = { spell: { level: 1 }, damage: [], healBySlotLevel: {}, higherLevel: [] };
    const lia = combatant({ id: 'lia', label: 'Lia', mountedOnId: 'estrela' });
    const estrela = combatant({ id: 'estrela', label: 'Estrela', riderId: 'lia' });
    const live = signal(encounter({ combatants: [lia, estrela] }));
    const data = {
      campaignId: 'c',
      encounterId: 'enc',
      casterId: 'lia',
      round: 1,
      spellKey: 'spell:shield-of-faith',
      name: 'Escudo da Fé',
      level: 1,
      concentration: true,
      cantripDice: '',
      economy: ActionEconomy.BONUS_ACTION,
      slots: [{ level: 1, free: 2, pact: false }],
      usage: [{ level: 1, total: 4, used: 2 }],
      pact: null,
      targets: undefined,
      shieldFree: null,
      shieldName: '',
      attackBonus: 0,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      alsoSteed,
      state: { encounter: live, apply: vi.fn() },
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
    const cast = async () => {
      el.querySelector<HTMLInputElement>('app-slot-picker input')?.click();
      fixture.detectChanges();
      (
        Array.from(el.querySelectorAll('button')).find((b) =>
          b.textContent?.includes('Conjurar Escudo da Fé'),
        ) as HTMLButtonElement
      ).click();
      await fixture.whenStable();
    };
    const check = () => el.querySelector<HTMLInputElement>('app-steed-also input[type=checkbox]');
    return { fixture, el, castSpell, cast, check, live, lia, estrela };
  }

  it("offers the check with the steed's name and what it does, on by default, and sends `also_steed`", async () => {
    const { el, castSpell, cast, check } = await open(true);
    const row = el.querySelector('app-steed-also')!;
    expect(row.querySelector('.also__t')?.textContent?.trim()).toBe('Estrela também');
    expect(row.querySelector('.also__d')?.textContent?.trim()).toBe(
      'O corcel que você monta. Um efeito que dura cai nos dois.',
    );
    expect(check()?.checked).toBe(true);
    await cast();
    expect(castSpell).toHaveBeenCalledTimes(1);
    expect(castSpell.mock.calls[0][12]).toMatchObject({ alsoSteed: true });
  });

  it('frames the check in the accent, as board 588-26 card 6 draws it', async () => {
    const { el } = await open(true);
    const row = getComputedStyle(el.querySelector('app-steed-also .also')!);
    // The mark is the app's garnet, not the browser's blue (seen in the render).
    expect(getComputedStyle(el.querySelector('app-steed-also input')!).accentColor).toBe(
      'var(--mr-accent)',
    );
    expect(row.borderTopStyle).toBe('solid');
    expect(parseFloat(row.borderTopWidth)).toBe(1);
    expect(parseFloat(row.minHeight)).toBe(44);
  });

  it("says the steed's line in the result, with what the cast did (the live region), not among what it spent", async () => {
    const { fixture, el, cast } = await open(true, {
      encounter: encounter({ combatants: [] }),
      cast: {
        targets: [],
        pendingDamages: [],
        metamagicKeys: [],
        sorceryPointsSpent: 0,
        slot: { level: 1, pact: false },
        concentrating: true,
        concentrationEndedSpellKey: '',
        steedNote: 'O corcel também é alvo da magia.',
      },
      summoned: [],
    });
    await cast();
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    const note = el.querySelector('app-cast-result [role=status] .steed');
    expect(note?.textContent?.trim()).toBe('O corcel também é alvo da magia.');
    expect(el.querySelector('app-cast-slots')?.textContent).not.toContain('O corcel também');
  });

  it('unticked, the cast reaches the caster alone', async () => {
    const { fixture, castSpell, cast, check } = await open(true);
    check()!.click();
    fixture.detectChanges();
    expect(check()?.checked).toBe(false);
    await cast();
    expect(castSpell.mock.calls[0][12]).not.toHaveProperty('alsoSteed');
  });

  it('has no check, and sends nothing of it, when the server does not offer it', async () => {
    const { el, castSpell, cast } = await open(false);
    expect(el.querySelector('app-steed-also')).toBeNull();
    await cast();
    expect(castSpell.mock.calls[0][12]).not.toHaveProperty('alsoSteed');
  });

  it('a dismount while the sheet is open takes the check away and says why; the cast reaches the caster alone', async () => {
    const { fixture, el, castSpell, cast, check, live, lia, estrela } = await open(true);
    const status = el.querySelector('.steed-status');
    expect(status?.getAttribute('role')).toBe('status');
    expect(status?.textContent?.trim()).toBe('');
    live.set(
      encounter({
        combatants: [
          { ...lia, mountedOnId: '' },
          { ...estrela, riderId: '' },
        ],
      }),
    );
    fixture.detectChanges();
    expect(el.querySelector('.steed-status')).toBe(status);
    expect(check()).toBeNull();
    expect(status?.querySelector('[role]')).toBeNull();
    expect(status?.querySelector('.steed-off')?.textContent?.trim()).toBe(
      'Você desmontou: a magia vale só para você.',
    );
    await cast();
    expect(castSpell.mock.calls[0][12]).not.toHaveProperty('alsoSteed');
  });
});
